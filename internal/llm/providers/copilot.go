package providers

import (
	"context"
	"errors"
	"net/http"

	"github.com/standardbeagle/slop-mcp/internal/llm"
)

// CopilotConfig is the provisional shape; Task 10 fleshes this out.
type CopilotConfig struct {
	BaseURL       string
	DefaultModel  string
	TokenStore    interface{ Token() (string, error) }
	HTTPClient    *http.Client
	EditorVersion string
}

// Copilot is a stub awaiting Task 10's real implementation.
type Copilot struct{}

func NewCopilot(_ CopilotConfig) *Copilot { return &Copilot{} }

func (c *Copilot) Name() string                    { return "copilot" }
func (c *Copilot) Available(_ context.Context) bool { return false }
func (c *Copilot) UnavailableReason() string        { return "copilot: not implemented" }
func (c *Copilot) Sample(_ context.Context, _ *llm.Request, _ func(llm.StreamEvent)) (*llm.Response, error) {
	return nil, errors.New("copilot: not implemented")
}
