package apidocs

import _ "embed"

// OpenAPI specification embedded into the binary for validation and discovery.
// The path is relative to this file.
//go:embed ../../docs/api/openapi.yaml
var OpenAPISpec []byte

