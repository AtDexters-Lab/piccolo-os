package server

import (
    "testing"
    "github.com/getkin/kin-openapi/openapi3"
)

// Ensures the embedded OpenAPI document is well-formed.
func TestOpenAPISpec_Validates(t *testing.T) {
    b, err := loadOpenAPISpec()
    if err != nil {
        t.Fatalf("failed to find openapi spec: %v", err)
    }
    loader := openapi3.NewLoader()
    doc, err := loader.LoadFromData(b)
    if err != nil {
        t.Fatalf("failed to load embedded openapi: %v", err)
    }
    if err := doc.Validate(loader.Context); err != nil {
        t.Fatalf("openapi spec validation failed: %v", err)
    }
}
