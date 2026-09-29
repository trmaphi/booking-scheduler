package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"scheduler/api/internal/httpapi"
)

func TestLivenessDoesNotCallDatabase(t *testing.T) {
	called := false
	router := httpapi.NewRouter(httpapi.Dependencies{Readiness: func(context.Context) error {
		called = true
		return errors.New("database unavailable")
	}})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil))
	if response.Code != http.StatusOK || response.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Errorf("liveness = %d %q, want 200 status ok", response.Code, response.Body.String())
	}
	if called {
		t.Error("liveness called the readiness dependency")
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

func TestReadinessReturnsOKWhenDatabaseResponds(t *testing.T) {
	router := httpapi.NewRouter(httpapi.Dependencies{Readiness: func(context.Context) error { return nil }})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil))
	if response.Code != http.StatusOK || response.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Errorf("readiness = %d %q, want 200 status ok", response.Code, response.Body.String())
	}
}

func TestReadinessReturnsUnavailableWithoutDetails(t *testing.T) {
	router := httpapi.NewRouter(httpapi.Dependencies{Readiness: func(context.Context) error {
		return errors.New("postgres password=private-secret dial 10.0.0.1:5432")
	}})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Errorf("readiness status = %d, want 503", response.Code)
	}
	for _, secret := range []string{"private-secret", "10.0.0.1", "password="} {
		if strings.Contains(response.Body.String(), secret) {
			t.Errorf("readiness body %q leaked dependency detail", response.Body.String())
		}
	}
	if !strings.Contains(response.Body.String(), "unavailable") {
		t.Errorf("readiness body = %q, want a safe unavailable message", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "\"code\":\"NOT_READY\"") || !strings.Contains(response.Body.String(), "\"requestId\":") {
		t.Errorf("readiness body = %q, want NOT_READY with request ID", response.Body.String())
	}
}

func TestCORSAllowsConfiguredFrontend(t *testing.T) {
	router := httpapi.NewRouter(httpapi.Dependencies{AllowedOrigin: "http://localhost:3000"})
	allowed := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/health/live", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Access-Control-Request-Method", "GET")
	router.ServeHTTP(allowed, request)
	if allowed.Code != http.StatusNoContent || allowed.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Errorf("allowed preflight = %d, origin %q", allowed.Code, allowed.Header().Get("Access-Control-Allow-Origin"))
	}
	if !strings.Contains(allowed.Header().Get("Access-Control-Allow-Methods"), "GET") {
		t.Errorf("allowed preflight methods = %q, want GET", allowed.Header().Get("Access-Control-Allow-Methods"))
	}

	rejected := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	request.Header.Set("Origin", "http://attacker.example")
	router.ServeHTTP(rejected, request)
	if rejected.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("untrusted origin was allowed: %q", rejected.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestUnknownRouteReturnsJSONNotFound(t *testing.T) {
	router := httpapi.NewRouter(httpapi.Dependencies{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/does-not-exist", nil))
	if response.Code != http.StatusNotFound || response.Header().Get("Content-Type") != "application/json" {
		t.Errorf("unknown route = %d, content type %q", response.Code, response.Header().Get("Content-Type"))
	}
	if !strings.Contains(response.Body.String(), "\"code\":\"NOT_FOUND\"") || !strings.Contains(response.Body.String(), "\"requestId\":") {
		t.Errorf("unknown route body = %q, want JSON error with request ID", response.Body.String())
	}
}
