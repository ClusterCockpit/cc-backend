# ClusterCockpit Backend - Agent Guidelines

## Build/Test Commands

- Build: `make` or `go build ./cmd/cc-backend`
- Run all tests: `make test` (runs: `go clean -testcache && go build ./... && go vet ./... && go test ./...`)
- Run single test: `go test -run TestName ./path/to/package`
- Run single test file: `go test ./path/to/package -run TestName`
- Frontend build: `cd web/frontend && npm install && npm run build`
- Generate GraphQL: `make graphql` (uses gqlgen)
- Generate Swagger: `make swagger` (uses swaggo/swag)

## Code Style

- **Formatting**: Use `gofumpt` for all Go files (strict requirement)
- **Copyright header**: All files must include copyright header (see existing files)
- **Package docs**: Document packages with comprehensive package-level comments explaining purpose, usage, configuration
- **Imports**: Standard library first, then external packages, then internal packages (grouped with blank lines)
- **Naming**: Use camelCase for private, PascalCase for exported; descriptive names (e.g., `JobRepository`, `handleError`)
- **Error handling**: Return errors, don't panic; use custom error types where appropriate; log with cclog package
- **Logging**: Use `cclog` package (e.g., `cclog.Errorf()`, `cclog.Warnf()`, `cclog.Debugf()`)
- **Testing**: Use standard `testing` package; use `testify/assert` for assertions; name tests `TestFunctionName`
- **Comments**: Document all exported functions/types with godoc-style comments
- **Structs**: Document fields with inline comments, especially for complex configurations
- **HTTP handlers**: Return proper status codes; use `handleError()` helper for consistent error responses
- **JSON**: Use struct tags for JSON marshaling; `DisallowUnknownFields()` for strict decoding
