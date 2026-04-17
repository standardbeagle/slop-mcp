package builtins

import "testing"

func TestReservedNames_KnownBuiltins(t *testing.T) {
	cases := []string{"mem_save", "mem_load", "store_set", "store_get", "emit"}
	for _, c := range cases {
		if !IsReservedBuiltin(c) {
			t.Errorf("%q should be reserved", c)
		}
	}
	if IsReservedBuiltin("my_custom_arg") {
		t.Error("non-builtin must not be reserved")
	}
}
