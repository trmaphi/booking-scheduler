package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"scheduler/api/internal/application"
	"scheduler/api/internal/httpapi"
)

func TestBookingOptionsReturnsContractShapeAndEmptyArrays(t *testing.T) {
	tests := []struct {
		name string
		data application.BookingOptions
		want map[string]any
	}{
		{
			name: "active options",
			data: application.BookingOptions{
				Vehicles:     []application.VehicleOption{{ID: testVehicleID, CustomerID: testCustomerID, Label: "Blue hatchback", Registration: "DEMO 101"}},
				Dealerships:  []application.DealershipOption{{ID: testDealershipID, Name: "Riverside Service Centre", Address: "100 Riverside Way", TimeZone: "Europe/London"}},
				ServiceTypes: []application.ServiceTypeOption{{ID: testServiceTypeID, Name: "Annual service", Description: "Routine inspection", DurationMinutes: 60}},
			},
			want: map[string]any{
				"vehicles":     []any{map[string]any{"id": testVehicleID, "customerId": testCustomerID, "label": "Blue hatchback", "registration": "DEMO 101"}},
				"dealerships":  []any{map[string]any{"id": testDealershipID, "name": "Riverside Service Centre", "address": "100 Riverside Way", "timezone": "Europe/London"}},
				"serviceTypes": []any{map[string]any{"id": testServiceTypeID, "name": "Annual service", "description": "Routine inspection", "durationMinutes": float64(60)}},
			},
		},
		{name: "empty options", data: application.BookingOptions{}, want: map[string]any{"vehicles": []any{}, "dealerships": []any{}, "serviceTypes": []any{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := httpapi.NewRouter(httpapi.Dependencies{BookingOptions: func(context.Context) (application.BookingOptions, error) { return tt.data, nil }})
			response := serve(router, http.MethodGet, "/api/v1/booking-options", nil)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body %s", response.Code, response.Body.String())
			}
			if got := response.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("content type = %q", got)
			}
			var got map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("body = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestBookingOptionsRequestIDCorsMethodAndErrors(t *testing.T) {
	secret := "postgres://private@10.0.0.1/db"
	tests := []struct {
		name       string
		method     string
		origin     string
		requestID  string
		load       func(context.Context) (application.BookingOptions, error)
		wantStatus int
		wantCode   string
	}{
		{name: "generated request ID", method: http.MethodGet, load: func(context.Context) (application.BookingOptions, error) {
			return application.BookingOptions{}, errors.New(secret)
		}, wantStatus: 500, wantCode: "INTERNAL_ERROR"},
		{name: "accepted caller request ID", method: http.MethodGet, requestID: "caller-123", load: func(context.Context) (application.BookingOptions, error) {
			return application.BookingOptions{}, errors.New(secret)
		}, wantStatus: 500, wantCode: "INTERNAL_ERROR"},
		{name: "wrong method", method: http.MethodPost, wantStatus: 405, wantCode: "METHOD_NOT_ALLOWED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := httpapi.NewRouter(httpapi.Dependencies{AllowedOrigin: "http://localhost:3000", BookingOptions: tt.load})
			req := httptest.NewRequest(tt.method, "/api/v1/booking-options", nil)
			if tt.requestID != "" {
				req.Header.Set("X-Request-ID", tt.requestID)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, body %s", response.Code, response.Body.String())
			}
			var body struct{ Code, RequestID, Message string }
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != tt.wantCode || body.RequestID == "" {
				t.Fatalf("error = %#v", body)
			}
			if tt.requestID != "" && body.RequestID != tt.requestID {
				t.Fatalf("request ID = %q", body.RequestID)
			}
			if body.Message == secret {
				t.Fatal("leaked backend error")
			}
			if got := response.Header().Get("X-Request-ID"); got != body.RequestID {
				t.Fatalf("header request ID = %q, body = %q", got, body.RequestID)
			}
		})
	}

	router := httpapi.NewRouter(httpapi.Dependencies{AllowedOrigin: "http://localhost:3000"})
	allowedRequest := httptest.NewRequest(http.MethodOptions, "/api/v1/booking-options", nil)
	allowedRequest.Header.Set("Origin", "http://localhost:3000")
	allowedRequest.Header.Set("Access-Control-Request-Method", http.MethodGet)
	allowedRequest.Header.Set("X-Request-ID", "preflight-1")
	allowed := httptest.NewRecorder()
	router.ServeHTTP(allowed, allowedRequest)
	if allowed.Code != 204 || allowed.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" || allowed.Header().Get("X-Request-ID") != "preflight-1" {
		t.Fatalf("allowed preflight = %d, headers %#v", allowed.Code, allowed.Header())
	}

	rejectedRequest := httptest.NewRequest(http.MethodOptions, "/api/v1/booking-options", nil)
	rejectedRequest.Header.Set("Origin", "http://localhost:3000.evil.example")
	rejectedRequest.Header.Set("Access-Control-Request-Method", http.MethodGet)
	rejected := httptest.NewRecorder()
	router.ServeHTTP(rejected, rejectedRequest)
	if rejected.Header().Get("Access-Control-Allow-Origin") != "" || rejected.Code == http.StatusNoContent {
		t.Fatalf("untrusted preflight = %d, headers %#v", rejected.Code, rejected.Header())
	}
}

func TestBookingOptionsPassesCanceledRequestContext(t *testing.T) {
	called := false
	router := httpapi.NewRouter(httpapi.Dependencies{BookingOptions: func(ctx context.Context) (application.BookingOptions, error) {
		called = true
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Errorf("dependency context error = %v", ctx.Err())
		}
		return application.BookingOptions{}, ctx.Err()
	}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/booking-options", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if !called {
		t.Fatal("dependency was not called")
	}
	if response.Code != 500 {
		t.Fatalf("status = %d", response.Code)
	}
}

const (
	testCustomerID    = "10000000-0000-0000-0000-000000000011"
	testVehicleID     = "10000000-0000-0000-0000-000000000012"
	testDealershipID  = "20000000-0000-0000-0000-000000000021"
	testServiceTypeID = "30000000-0000-0000-0000-000000000031"
)

func serve(handler http.Handler, method, target string, body any) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}
