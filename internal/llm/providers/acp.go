package providers

import (
	"context"
	"errors"

	"github.com/standardbeagle/slop-mcp/internal/llm"
)

// ACPConfig is the provisional shape; Task 11 fleshes this out.
type ACPConfig struct {
	Command []string
	URL     string
}

// ACP is a stub awaiting Task 11's real implementation.
type ACP struct{}

func NewACP(_ ACPConfig) *ACP { return &ACP{} }

func (a *ACP) Name() string                    { return "acp" }
func (a *ACP) Available(_ context.Context) bool { return false }
func (a *ACP) UnavailableReason() string        { return "acp: not implemented" }
func (a *ACP) Sample(_ context.Context, _ *llm.Request, _ func(llm.StreamEvent)) (*llm.Response, error) {
	return nil, errors.New("acp: not implemented")
}
