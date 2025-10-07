package server

import (
    "context"
    "net/http"
    "strings"
    "os"
    "path/filepath"

    "github.com/gin-gonic/gin"
    "github.com/getkin/kin-openapi/openapi3"
    "github.com/getkin/kin-openapi/openapi3filter"
    "github.com/getkin/kin-openapi/routers"
    legacyrouter "github.com/getkin/kin-openapi/routers/legacy"
)

// openAPIValidator validates incoming API requests against the embedded OpenAPI spec.
type openAPIValidator struct {
    router routers.Router
}

// newOpenAPIValidator loads the embedded OpenAPI doc and prepares a router for validation.
func newOpenAPIValidator() (*openAPIValidator, error) {
    b, err := loadOpenAPISpec()
    if err != nil { return nil, err }
    loader := openapi3.NewLoader()
    doc, err := loader.LoadFromData(b)
    if err != nil {
        return nil, err
    }
    if err := doc.Validate(loader.Context); err != nil {
        // Keep going even if non-fatal warnings; return error only on hard failures
        // For simplicity, return err and let caller decide whether to continue.
        return nil, err
    }
    r, err := legacyrouter.NewRouter(doc)
    if err != nil {
        return nil, err
    }
    return &openAPIValidator{router: r}, nil
}

// Middleware returns a Gin middleware that validates API requests under /api/ against the spec.
func (v *openAPIValidator) Middleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Only validate API routes
        if !strings.HasPrefix(c.Request.URL.Path, "/api/") {
            c.Next()
            return
        }
        // Find the route in the OpenAPI router
        route, pathParams, err := v.router.FindRoute(c.Request)
        if err != nil {
            // No matching route in spec -> 404/405 at router later; mark as bad request here
            c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Request not in API spec", "detail": err.Error()})
            return
        }
        // Validate request
        input := &openapi3filter.RequestValidationInput{
            Request:    c.Request,
            PathParams: pathParams,
            Route:      route,
        }
        // Install a permissive AuthenticationFunc so that spec-level security
        // does not interfere with the app's own session/CSRF middleware. This
        // keeps the validator useful for shapes/params while auth is enforced
        // elsewhere.
        opts := &openapi3filter.Options{
            AuthenticationFunc: func(ctx context.Context, ai *openapi3filter.AuthenticationInput) error {
                // Accept cookieAuth (or any) here; real auth enforced by Gin middleware.
                return nil
            },
        }
        input.Options = opts
        if err := openapi3filter.ValidateRequest(c.Request.Context(), input); err != nil {
            c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Request failed validation", "detail": err.Error()})
            return
        }
        c.Next()
    }
}

// loadOpenAPISpec tries to load the spec from common locations.
func loadOpenAPISpec() ([]byte, error) {
    if p := os.Getenv("PICCOLO_OPENAPI_PATH"); p != "" {
        if b, err := os.ReadFile(p); err == nil { return b, nil }
    }
    // Relative to server binary working dir (dev tree)
    if b, err := os.ReadFile(filepath.Join("docs", "api", "openapi.yaml")); err == nil {
        return b, nil
    }
    // When running tests from package dir, try up two levels
    if b, err := os.ReadFile(filepath.Join("..", "..", "docs", "api", "openapi.yaml")); err == nil {
        return b, nil
    }
    // Optional: package data path
    return nil, os.ErrNotExist
}
