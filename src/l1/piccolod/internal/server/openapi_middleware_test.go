package server

import (
    "testing"
    "github.com/getkin/kin-openapi/openapi3"
    "piccolod/internal/apidocs"
)

// Ensures the embedded OpenAPI document is well-formed.
func TestOpenAPISpec_Validates(t *testing.T) {
    loader := openapi3.NewLoader()
    doc, err := loader.LoadFromData(apidocs.OpenAPISpec)
    if err != nil {
        t.Fatalf("failed to load embedded openapi: %v", err)
    }
    if err := doc.Validate(loader.Context); err != nil {
        t.Fatalf("openapi spec validation failed: %v", err)
    }
}

