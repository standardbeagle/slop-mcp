package server

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/standardbeagle/slop-mcp/internal/registry"
)

// TestHandleRunSlop_ConcurrentPipeline exercises many concurrent run_slop calls
// that each use pipeline builtins (map/reduce). slop v0.3.0 binds the pipeline
// function caller to a package global at construction, so without the
// process-wide construct+execute lock these would race the global and misroute
// callbacks across runtimes. Run with -race to catch regressions.
func TestHandleRunSlop_ConcurrentPipeline(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	const workers = 24
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			// sum of (x*2) for x in [1..n+3] via map + reduce; result is
			// deterministic per-worker so cross-runtime misrouting would corrupt it.
			hi := n + 3
			script := fmt.Sprintf(`reduce(map([%s], (x) -> x * 2), (a, b) -> a + b, 0)`, rangeList(1, hi))
			_, output, err := s.handleRunSlop(ctx, &mcp.CallToolRequest{}, RunSlopInput{Script: script})
			if err != nil {
				errs <- fmt.Errorf("worker %d: %w", n, err)
				return
			}
			want := int64(0)
			for x := 1; x <= hi; x++ {
				want += int64(x) * 2
			}
			got, ok := output.Result.(int64)
			if !ok || got != want {
				errs <- fmt.Errorf("worker %d: got %v (%T), want %d", n, output.Result, output.Result, want)
			}
		}(i)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func rangeList(lo, hi int) string {
	out := ""
	for i := lo; i <= hi; i++ {
		if i > lo {
			out += ", "
		}
		out += fmt.Sprintf("%d", i)
	}
	return out
}
