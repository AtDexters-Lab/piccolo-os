package apidocs

import (
    "context"
    "path/filepath"
    "testing"

    "github.com/getkin/kin-openapi/openapi3"
)

func TestOpenAPISpec_Validates(t *testing.T) {
    specPath := filepath.Join("openapi.yaml")

    loader := openapi3.NewLoader()
    loader.IsExternalRefsAllowed = false

    doc, err := loader.LoadFromFile(specPath)
    if err != nil {
        t.Fatalf("failed to load OpenAPI spec: %v", err)
    }

    if err := doc.Validate(context.Background()); err != nil {
        t.Fatalf("OpenAPI validation failed: %v", err)
    }
}

