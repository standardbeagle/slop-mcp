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

// Available probes spawnability. On first call it tries to start the
// subprocess; subsequent calls return the cached connected state. If
// Command is empty or spawn fails, returns false.
func (a *ACP) Available(_ context.Context) bool {
	if a.connected.Load() {
		return true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.proc != nil {
		return true
	}
	if len(a.cfg.Command) == 0 {
		return false
	}
	if err := a.spawnLocked(); err != nil {
		return false
	}
	return true
}

func (a *ACP) UnavailableReason() string {
	if a.connected.Load() {
		return ""
	}
	if len(a.cfg.Command) == 0 {
		return "acp: no command configured"
	}
	return "acp: subprocess not yet spawned"
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
// Note: the request schema here mirrors the protocol shape per the
// April 2026 draft. The ACP wire format is evolving; field names may
// require adjustment when validated against a current ACP agent.
func (a *ACP) Sample(ctx context.Context, req *llm.Request, _ func(llm.StreamEvent)) (*llm.Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.proc == nil {
		if err := a.spawnLocked(); err != nil {
			return nil, fmt.Errorf("acp spawn: %w", err)
		}
	}

	id := a.nextID.Add(1)
	rpc := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "prompt",
		"params": map[string]any{
			"messages": req.Messages,
			"system":   req.System,
			"model":    req.Model,
			"tools":    req.Tools,
		},
	}
	if err := writeFrame(a.stdin, rpc); err != nil {
		return nil, fmt.Errorf("acp write: %w", err)
	}

	frame, err := readFrame(a.stdout)
	if err != nil {
		return nil, fmt.Errorf("acp read: %w", err)
	}

	var raw struct {
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
	if err := json.Unmarshal(frame, &raw); err != nil {
		return nil, fmt.Errorf("acp decode: %w", err)
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
