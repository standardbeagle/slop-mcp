package builtins

import (
	"testing"

	"github.com/standardbeagle/slop/pkg/slop"
)

func TestDedent_MixedAndWhitespaceOnlyLines(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "common space indent stripped",
			in:   "    a\n    b\n      c",
			want: "a\nb\n  c",
		},
		{
			name: "whitespace-only line normalized to empty",
			in:   "    a\n    \n    b",
			want: "a\n\nb",
		},
		{
			name: "tab vs space are not conflated",
			in:   "\ta\n b", // no common prefix (tab vs space)
			want: "\ta\n b",
		},
		{
			name: "common tab prefix stripped",
			in:   "\t\ta\n\t\tb",
			want: "a\nb",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := builtinDedent([]slop.Value{slop.NewStringValue(tc.in)}, nil)
			if err != nil {
				t.Fatalf("dedent: %v", err)
			}
			got := out.(*slop.StringValue).Value
			if got != tc.want {
				t.Errorf("dedent(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestTmplDiv_WholeResultCollapsesToInt(t *testing.T) {
	got, err := tmplDiv(int64(6), int64(2))
	if err != nil {
		t.Fatalf("div: %v", err)
	}
	if v, ok := got.(int64); !ok || v != 3 {
		t.Errorf("div(6,2) = %v (%T), want int64(3)", got, got)
	}

	got, err = tmplDiv(int64(7), int64(2))
	if err != nil {
		t.Fatalf("div: %v", err)
	}
	if v, ok := got.(float64); !ok || v != 3.5 {
		t.Errorf("div(7,2) = %v (%T), want float64(3.5)", got, got)
	}

	if _, err := tmplDiv(int64(1), int64(0)); err == nil {
		t.Error("div by zero should error")
	}
}
