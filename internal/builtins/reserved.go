package builtins

import "github.com/standardbeagle/slop/pkg/slop"

// reservedNames is derived once from a reference runtime that has every SLOP
// builtin plus every builtin this project registers. Membership is checked via
// the runtime's global scope, so the set stays in sync with upstream additions
// and with new internal/builtins/*.go registrations automatically -- unlike a
// hand-maintained list, which drifted out of date (missing crypto_*, jwt_*,
// store_keys, dedent, ... and listing never-registered names).
//
// Built eagerly at package initialization: init runs single-threaded, so the
// construction (which rebinds slop's package-global pipeline caller) races
// nothing, and IsReservedBuiltin never constructs a runtime at call time --
// important because it is called from inside the process-wide exec lock, where
// acquiring that lock again would deadlock.
var reservedNames = buildReservedNames()

// extraReserved covers names that are NOT registered builtins in the global
// scope but must still not be used as arg shorthands: "args" is the full
// custom-tool parameter map this project injects, and "emit" is a SLOP
// statement keyword rather than a builtin value.
var extraReserved = map[string]struct{}{
	"args": {},
	"emit": {},
}

// buildReservedNames constructs a reference runtime with the full builtin set
// and returns a membership tester over its global scope.
func buildReservedNames() *slop.Runtime {
	rt := slop.NewRuntime()
	// print is registered per-runtime by the server; include it here so custom
	// tool arg shorthands cannot shadow it.
	rt.RegisterBuiltin("print", func(_ []slop.Value, _ map[string]slop.Value) (slop.Value, error) {
		return slop.NewNullValue(), nil
	})
	RegisterCrypto(rt)
	RegisterSlopSearch(rt)
	RegisterJWT(rt)
	RegisterTemplate(rt)
	RegisterSession(rt, NewSessionStore())
	RegisterMemory(rt, NewMemoryStore())
	return rt
}

// IsReservedBuiltin reports whether name would collide with a SLOP builtin or a
// slop-mcp-injected binding, and therefore must not be used as a shorthand
// binding for a custom tool arg.
func IsReservedBuiltin(name string) bool {
	if _, ok := extraReserved[name]; ok {
		return true
	}
	return reservedNames.Context().Globals.Has(name)
}
