package builtins

import (
	"strings"
	"testing"

	"github.com/standardbeagle/slop/pkg/slop"
)

func newTemplateTestRuntime(t *testing.T) *slop.Runtime {
	t.Helper()
	rt := NewRuntime()
	t.Cleanup(func() { _ = rt.Close() })
	return rt
}

func TestTemplateNumericConversions(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		want string
	}{
		{name: "int from string", tmpl: `{{ toInt "42" }}`, want: "42"},
		{name: "int trims spaces", tmpl: `{{ toInt " 42 " }}`, want: "42"},
		{name: "float from string", tmpl: `{{ toFloat "3.5" }}`, want: "3.5"},
		{name: "float trims spaces", tmpl: `{{ toFloat " 3.5 " }}`, want: "3.5"},
		{name: "add numeric strings", tmpl: `{{ add "2" "3" }}`, want: "5"},
		{name: "mod numeric strings", tmpl: `{{ mod "5" "2" }}`, want: "1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderTemplate(newTemplateTestRuntime(t), tt.tmpl, nil)
			if err != nil {
				t.Fatalf("renderTemplate: %v", err)
			}
			if got != tt.want {
				t.Fatalf("renderTemplate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTemplateNumericConversionsRejectInvalidStrings(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    string
		wantErr string
	}{
		{name: "invalid int", tmpl: `{{ toInt "abc" }}`, wantErr: `toInt: invalid integer "abc"`},
		{name: "partial int", tmpl: `{{ toInt "12abc" }}`, wantErr: `toInt: invalid integer "12abc"`},
		{name: "invalid float", tmpl: `{{ toFloat "abc" }}`, wantErr: `toFloat: invalid float "abc"`},
		{name: "partial float", tmpl: `{{ toFloat "1.2abc" }}`, wantErr: `toFloat: invalid float "1.2abc"`},
		{name: "invalid math operand", tmpl: `{{ add "1x" "2" }}`, wantErr: `toFloat: invalid float "1x"`},
		{name: "divide by zero", tmpl: `{{ div "1" "0" }}`, wantErr: `div: division by zero`},
		{name: "mod by zero", tmpl: `{{ mod "1" "0" }}`, wantErr: `mod: division by zero`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderTemplate(newTemplateTestRuntime(t), tt.tmpl, nil)
			if err == nil {
				t.Fatalf("expected error, got %q", got)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestTemplateJSONHelpers(t *testing.T) {
	got, err := renderTemplate(newTemplateTestRuntime(t), `{{ index (fromJson "{\"name\":\"slop\"}") "name" }}`, nil)
	if err != nil {
		t.Fatalf("renderTemplate fromJson: %v", err)
	}
	if got != "slop" {
		t.Fatalf("renderTemplate fromJson = %q, want slop", got)
	}

	got, err = renderTemplate(newTemplateTestRuntime(t), `{{ toJson . }}`, map[string]any{"ok": true})
	if err != nil {
		t.Fatalf("renderTemplate toJson: %v", err)
	}
	if got != `{"ok":true}` {
		t.Fatalf("renderTemplate toJson = %q, want {\"ok\":true}", got)
	}
}

func TestTemplateZeroBoolAndEmpty(t *testing.T) {
	rt := newTemplateTestRuntime(t)
	tests := []struct {
		name string
		tmpl string
		data any
		want string
	}{
		{name: "int64 zero false", tmpl: `{{ toBool . }}`, data: int64(0), want: "false"},
		{name: "float zero false", tmpl: `{{ toBool . }}`, data: float64(0), want: "false"},
		{name: "int64 zero empty", tmpl: `{{ empty . }}`, data: int64(0), want: "true"},
		{name: "float zero empty", tmpl: `{{ empty . }}`, data: float64(0), want: "true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderTemplate(rt, tt.tmpl, tt.data)
			if err != nil {
				t.Fatalf("renderTemplate: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTemplateSlopUsesRegisteredRuntime(t *testing.T) {
	rt := newTemplateTestRuntime(t)
	rt.Context().Globals.Set("name", slop.NewStringValue("alice"))

	got, err := renderTemplate(rt, `{{ slop "name" }}`, nil)
	if err != nil {
		t.Fatalf("renderTemplate slop: %v", err)
	}
	if got != "alice" {
		t.Fatalf("got %q, want alice", got)
	}
}

func TestTemplateJSONHelpersRejectInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    string
		data    any
		wantErr string
	}{
		{
			name:    "fromJson malformed",
			tmpl:    `{{ fromJson "{" }}`,
			wantErr: "fromJson: invalid JSON",
		},
		{
			name:    "toJson unmarshalable",
			tmpl:    `{{ toJson . }}`,
			data:    map[string]any{"bad": make(chan int)},
			wantErr: "toJson: json: unsupported type: chan int",
		},
		{
			name:    "toPrettyJson unmarshalable",
			tmpl:    `{{ toPrettyJson . }}`,
			data:    map[string]any{"bad": make(chan int)},
			wantErr: "toPrettyJson: json: unsupported type: chan int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderTemplate(newTemplateTestRuntime(t), tt.tmpl, tt.data)
			if err == nil {
				t.Fatalf("expected error, got %q", got)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
