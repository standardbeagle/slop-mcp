package builtins

// reserved lists SLOP builtin names that must not shadow custom tool arg
// shorthand bindings. Keep in sync with the slop/pkg/slop builtin registry
// and with any builtins this project adds in internal/builtins/*.go.
var reserved = map[string]struct{}{
	// memory (this package)
	"mem_save":   {},
	"mem_load":   {},
	"mem_list":   {},
	"mem_search": {},
	"mem_info":   {},
	"mem_delete": {},
	"mem_clear":  {},
	// session (this package)
	"store_set":   {},
	"store_get":   {},
	"store_list":  {},
	"store_clear": {},
	// execute_tool bridge
	"execute_tool": {},
	// standard slop builtins
	"emit":            {},
	"map":             {},
	"filter":          {},
	"reduce":          {},
	"len":             {},
	"json_parse":      {},
	"json_stringify":  {},
	// optional / may-be-present depending on build
	"http_get":  {},
	"http_post": {},
	// reserved arg binding name (the full args map is bound as "args")
	"args": {},
}

// IsReservedBuiltin reports whether name would collide with a SLOP builtin
// or a slop-mcp-injected binding, and therefore must not be used as a
// shorthand binding for a custom tool arg.
func IsReservedBuiltin(name string) bool {
	_, ok := reserved[name]
	return ok
}
