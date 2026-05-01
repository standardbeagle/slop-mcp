package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/standardbeagle/slop-mcp/internal/llm"
)

// ACPConfig configures the Agent Client Protocol provider.
// V1 only supports stdio transport (Command). URL is a placeholder for
// future socket-based transports.
type ACPConfig struct {
	Command []string
	URL     string
}

// ACP is a stdio JSON-RPC client for an Agent Client Protocol agent.
type ACP struct {
	cfg       ACPConfig
	mu        sync.Mutex
	proc      *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	nextID    atomic.Int64
	connected atomic.Bool
}

func NewACP(cfg ACPConfig) *ACP { return &ACP{cfg: cfg} }

func (a *ACP) Name() string { return "acp" }

// Available reports whether the provider is configured and either
// already running or able to be spawned. It does NOT spawn — that
// happens lazily on the first Sample call so the chain's per-request
// probe stays cheap and side-effect-free (matching OpenAI/Copilot
// where Available is a pure capability check).
func (a *ACP) Available(_ context.Context) bool {
	if a.connected.Load() {
		return true
	}
	return len(a.cfg.Command) > 0
}

func (a *ACP) UnavailableReason() string {
	if a.connected.Load() {
		return ""
	}
	if len(a.cfg.Command) == 0 {
		return "acp: no command configured"
	}
	return ""
}

func (a *ACP) spawnLocked() error {
	cmd := exec.Command(a.cfg.Command[0], a.cfg.Command[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	a.proc = cmd
	a.stdin = stdin
	a.stdout = bufio.NewReader(stdout)
	a.connected.Store(true)
	return nil
}

// Sample sends one ACP `prompt` request and reads one response frame.
//
// The request schema mirrors the April 2026 ACP draft. The wire format
// is evolving; field names may require adjustment when validated
// against a current ACP agent. Optional sampling parameters
// (temperature, max_tokens, stop) are forwarded for forward
// compatibility — agents that do not yet honour them will ignore
// unknown keys.
//
// Streaming and ctx cancellation are not yet implemented; onEvent is
// unused. Subprocess stderr is inherited from the parent process
// intentionally so its diagnostic output flows through slop-mcp's
// existing stderr channel.
//
// On any I/O error (read or write), the stored subprocess handle is
// cleared so the next Sample call respawns the agent. This trades the
// agent's in-flight conversation state (already lost when the process
// crashes) for automatic recovery.
func (a *ACP) Sample(ctx context.Context, req *llm.Request, _ func(llm.StreamEvent)) (resp *llm.Response, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	defer func() {
		if err != nil {
			// Either the subprocess died or the wire became corrupt.
			// Drop our handle so the next call respawns.
			a.connected.Store(false)
			a.proc = nil
			a.stdin = nil
			a.stdout = nil
		}
	}()

	if a.proc == nil {
		if spawnErr := a.spawnLocked(); spawnErr != nil {
			return nil, fmt.Errorf("acp spawn: %w", spawnErr)
		}
	}

	id := a.nextID.Add(1)
	params := map[string]any{
		"messages": req.Messages,
		"system":   req.System,
		"model":    req.Model,
		"tools":    req.Tools,
	}
	if req.Temperature != nil {
		params["temperature"] = *req.Temperature
	}
	if req.MaxTokens != nil {
		params["max_tokens"] = *req.MaxTokens
	}
	if len(req.Stop) > 0 {
		params["stop"] = req.Stop
	}
	rpc := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "prompt",
		"params":  params,
	}
	if writeErr := writeFrame(a.stdin, rpc); writeErr != nil {
		return nil, fmt.Errorf("acp write: %w", writeErr)
	}

	frame, readErr := readFrame(a.stdout)
	if readErr != nil {
		return nil, fmt.Errorf("acp read: %w", readErr)
	}

	var raw struct {
		ID     int64 `json:"id"`
		Result *struct {
			Content    string         `json:"content"`
			ToolCalls  []llm.ToolCall `json:"tool_calls"`
			StopReason string         `json:"stop_reason"`
			Model      string         `json:"model"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if decodeErr := json.Unmarshal(frame, &raw); decodeErr != nil {
		return nil, fmt.Errorf("acp decode: %w", decodeErr)
	}
	if raw.ID != id {
		return nil, fmt.Errorf("acp: response id %d != request id %d", raw.ID, id)
	}
	if raw.Error != nil {
		return nil, fmt.Errorf("acp error %d: %s", raw.Error.Code, raw.Error.Message)
	}
	if raw.Result == nil {
		return nil, errors.New("acp response missing result")
	}
	return &llm.Response{
		Content:    raw.Result.Content,
		ToolCalls:  raw.Result.ToolCalls,
		StopReason: llm.StopReason(raw.Result.StopReason),
		Model:      raw.Result.Model,
	}, nil
}

// writeFrame emits a single JSON object followed by a newline.
func writeFrame(w io.Writer, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := w.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// readFrame reads one newline-terminated JSON message.
func readFrame(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	return line, nil
}
