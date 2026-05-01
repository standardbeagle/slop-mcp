package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"testing"
)

func TestACP_AvailableFalseWithoutCommand(t *testing.T) {
	p := NewACP(ACPConfig{})
	if p.Available(context.Background()) {
		t.Fatal("must be unavailable when Command is empty")
	}
	if reason := p.UnavailableReason(); reason == "" {
		t.Fatal("must give a reason when unavailable")
	}
}

func TestACP_BadCommandSurfacesOnSample(t *testing.T) {
	// Available is a pure capability check (configured + alive); it
	// returns true when Command is non-empty even if the binary does
	// not exist. The spawn failure is surfaced when Sample is called.
	p := NewACP(ACPConfig{Command: []string{"/nonexistent/binary"}})
	if !p.Available(context.Background()) {
		t.Fatal("Available should return true for a configured command")
	}
	_, err := p.Sample(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("Sample must error when subprocess cannot spawn")
	}
}

func TestACP_NameIsACP(t *testing.T) {
	p := NewACP(ACPConfig{})
	if p.Name() != "acp" {
		t.Fatalf("want acp, got %q", p.Name())
	}
}

func TestACP_FrameRoundTrip(t *testing.T) {
	r, w := io.Pipe()
	go func() {
		_ = writeFrame(w, map[string]any{"hello": "world"})
		_ = w.Close()
	}()
	got, err := readFrame(bufio.NewReader(r))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatal(err)
	}
	if m["hello"] != "world" {
		t.Fatalf("frame mismatch: %v", m)
	}
}
