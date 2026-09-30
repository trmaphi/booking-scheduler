package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"scheduler/api/internal/httpapi"
)

func TestRouterServesScalarAPIReference(t *testing.T) {
	router := httpapi.NewRouter(httpapi.Dependencies{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/docs", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("GET /api/docs = %d: %s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Fatalf("GET /api/docs Content-Type = %q", contentType)
	}
	for _, expected := range []string{
		"@scalar/api-reference@1.72.1",
		"Scalar.createApiReference",
		"url: '/api/openapi.yaml'",
	} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Errorf("GET /api/docs body does not contain %q", expected)
		}
	}
}

func TestRouterServesOpenAPIContract(t *testing.T) {
	router := httpapi.NewRouter(httpapi.Dependencies{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/openapi.yaml", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("GET /api/openapi.yaml = %d: %s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/yaml" {
		t.Fatalf("GET /api/openapi.yaml Content-Type = %q", contentType)
	}
	if !strings.Contains(response.Body.String(), "openapi: 3.1.0") {
		t.Fatal("GET /api/openapi.yaml did not return the booking API contract")
	}
}

func TestDocumentationRoutesRejectUnsupportedMethods(t *testing.T) {
	router := httpapi.NewRouter(httpapi.Dependencies{})
	for _, path := range []string{"/api/docs", "/api/openapi.yaml"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
			if response.Code != http.StatusMethodNotAllowed {
				t.Fatalf("POST %s = %d: %s", path, response.Code, response.Body.String())
			}
			if allow := response.Header().Get("Allow"); allow != http.MethodGet {
				t.Fatalf("POST %s Allow = %q", path, allow)
			}
		})
	}
}
