module github.com/anthropics/slop-mcp

go 1.23.0

toolchain go1.24.2

replace github.com/anthropics/slop => ../slop

require (
	github.com/anthropics/slop v0.0.0-00010101000000-000000000000
	github.com/modelcontextprotocol/go-sdk v1.2.0
	github.com/sblinch/kdl-go v0.0.0-20251203232544-981d4ecc17c3
)

require (
	github.com/google/jsonschema-go v0.3.0 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/oauth2 v0.30.0 // indirect
)
