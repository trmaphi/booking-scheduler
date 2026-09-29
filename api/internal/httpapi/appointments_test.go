package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"scheduler/api/internal/application"
	"scheduler/api/internal/httpapi"
	"scheduler/api/internal/postgres"
	"scheduler/api/migrations"
)

const (
	testAppointmentID = "40000000-0000-4000-8000-000000000001"
	testTechnicianID  = "40000000-0000-4000-8000-000000000002"
	testBayID         = "40000000-0000-4000-8000-000000000003"
)

func TestConfirmAppointmentReturnsCreatedAndReplayContracts(t *testing.T) {
	appointment := completeAppointment()
	for _, test := range []struct {
		name     string
		replayed bool
		status   int
	}{
		{name: "created", status: http.StatusCreated},
		{name: "replayed", replayed: true, status: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			var got application.ConfirmCommand
			router := httpapi.NewRouter(httpapi.Dependencies{ConfirmAppointment: func(_ context.Context, command application.ConfirmCommand) (application.ConfirmationResult, error) {
				got = command
				return application.ConfirmationResult{Appointment: appointment, Replayed: test.replayed}, nil
			}})
			response := appointmentRequest(router, http.MethodPost, "/api/v1/appointments", validConfirmJSON(), "idempotency-key-0001")
			if response.Code != test.status {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			if got.VehicleID != testVehicleID || got.DealershipID != testDealershipID || got.ServiceTypeID != testServiceTypeID || got.IdempotencyKey != "idempotency-key-0001" || !got.StartAt.Equal(time.Date(2031, 3, 4, 9, 30, 0, 0, time.UTC)) {
				t.Fatalf("command = %#v", got)
			}
			assertAppointmentResponse(t, response, appointment)
		})
	}
}

func TestConfirmAppointmentChecksGridInDeclaredOffset(t *testing.T) {
	called := false
	router := httpapi.NewRouter(httpapi.Dependencies{ConfirmAppointment: func(_ context.Context, command application.ConfirmCommand) (application.ConfirmationResult, error) {
		called = true
		want := time.Date(2031, 3, 4, 3, 45, 0, 0, time.UTC)
		if !command.StartAt.Equal(want) {
			t.Fatalf("start = %s, want %s", command.StartAt, want)
		}
		return application.ConfirmationResult{Appointment: completeAppointment()}, nil
	}})
	body := strings.Replace(validConfirmJSON(), "2031-03-04T09:30:00Z", "2031-03-04T09:30:00+05:45", 1)
	response := appointmentRequest(router, http.MethodPost, "/api/v1/appointments", body, "idempotency-key-0001")
	if response.Code != http.StatusCreated || !called {
		t.Fatalf("response = %d %s, called = %v", response.Code, response.Body.String(), called)
	}
}

func TestConfirmAppointmentRejectsInvalidTransportBeforeGateway(t *testing.T) {
	oversized := `{"vehicleId":"` + strings.Repeat("x", (1<<20)+1) + `"}`
	tests := []struct {
		name, contentType, key, body, field string
	}{
		{name: "missing content type", key: "idempotency-key-0001", body: validConfirmJSON(), field: "contentType"},
		{name: "wrong content type", contentType: "text/plain", key: "idempotency-key-0001", body: validConfirmJSON(), field: "contentType"},
		{name: "missing key", contentType: "application/json", body: validConfirmJSON(), field: "idempotencyKey"},
		{name: "short key", contentType: "application/json", key: "too-short", body: validConfirmJSON(), field: "idempotencyKey"},
		{name: "long key", contentType: "application/json", key: strings.Repeat("a", 129), body: validConfirmJSON(), field: "idempotencyKey"},
		{name: "invalid key", contentType: "application/json", key: "invalid key value", body: validConfirmJSON(), field: "idempotencyKey"},
		{name: "empty body", contentType: "application/json", key: "idempotency-key-0001", field: "body"},
		{name: "oversized body", contentType: "application/json", key: "idempotency-key-0001", body: oversized, field: "body"},
		{name: "malformed json", contentType: "application/json", key: "idempotency-key-0001", body: `{`, field: "body"},
		{name: "unknown field", contentType: "application/json", key: "idempotency-key-0001", body: strings.TrimSuffix(validConfirmJSON(), "}") + `,"customerId":"x"}`, field: "customerId"},
		{name: "case variant field", contentType: "application/json", key: "idempotency-key-0001", body: strings.Replace(validConfirmJSON(), `"vehicleId"`, `"VehicleId"`, 1), field: "VehicleId"},
		{name: "canonical and case variant", contentType: "application/json", key: "idempotency-key-0001", body: strings.TrimSuffix(validConfirmJSON(), "}") + `,"VehicleId":"` + testVehicleID + `"}`, field: "VehicleId"},
		{name: "duplicate top-level field", contentType: "application/json", key: "idempotency-key-0001", body: `{"vehicleId":"` + testVehicleID + `","vehicleId":"` + testVehicleID + `","dealershipId":"` + testDealershipID + `","serviceTypeId":"` + testServiceTypeID + `","startAt":"2031-03-04T09:30:00Z"}`, field: "vehicleId"},
		{name: "duplicate nested field", contentType: "application/json", key: "idempotency-key-0001", body: strings.TrimSuffix(validConfirmJSON(), "}") + `,"extra":{"value":1,"value":2}}`, field: "value"},
		{name: "trailing json", contentType: "application/json", key: "idempotency-key-0001", body: validConfirmJSON() + `{}`, field: "body"},
		{name: "missing vehicle", contentType: "application/json", key: "idempotency-key-0001", body: `{"dealershipId":"` + testDealershipID + `","serviceTypeId":"` + testServiceTypeID + `","startAt":"2031-03-04T09:30:00Z"}`, field: "vehicleId"},
		{name: "invalid vehicle uuid", contentType: "application/json", key: "idempotency-key-0001", body: strings.Replace(validConfirmJSON(), testVehicleID, "not-a-uuid", 1), field: "vehicleId"},
		{name: "invalid dealership uuid", contentType: "application/json", key: "idempotency-key-0001", body: strings.Replace(validConfirmJSON(), testDealershipID, "not-a-uuid", 1), field: "dealershipId"},
		{name: "invalid service uuid", contentType: "application/json", key: "idempotency-key-0001", body: strings.Replace(validConfirmJSON(), testServiceTypeID, "not-a-uuid", 1), field: "serviceTypeId"},
		{name: "missing offset", contentType: "application/json", key: "idempotency-key-0001", body: strings.Replace(validConfirmJSON(), "2031-03-04T09:30:00Z", "2031-03-04T09:30:00", 1), field: "startAt"},
		{name: "misaligned start", contentType: "application/json", key: "idempotency-key-0001", body: strings.Replace(validConfirmJSON(), "09:30:00Z", "09:15:00Z", 1), field: "startAt"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			called := false
			router := httpapi.NewRouter(httpapi.Dependencies{ConfirmAppointment: func(context.Context, application.ConfirmCommand) (application.ConfirmationResult, error) {
				called = true
				return application.ConfirmationResult{}, nil
			}})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments", strings.NewReader(test.body))
			if test.contentType != "" {
				req.Header.Set("Content-Type", test.contentType)
			}
			if test.key != "" {
				req.Header.Set("Idempotency-Key", test.key)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			if called {
				t.Fatal("gateway called for invalid request")
			}
			assertAPIError(t, response, "VALIDATION_ERROR", test.field, true)
		})
	}
}

func TestConfirmAppointmentRequiresUnambiguousHeaders(t *testing.T) {
	tests := []struct {
		name, field string
		addHeaders  func(*http.Request)
	}{
		{name: "multiple content type lines", field: "contentType", addHeaders: func(r *http.Request) { r.Header.Add("Content-Type", "application/json") }},
		{name: "comma joined content types", field: "contentType", addHeaders: func(r *http.Request) { r.Header.Set("Content-Type", "application/json, application/json") }},
		{name: "multiple idempotency lines", field: "idempotencyKey", addHeaders: func(r *http.Request) { r.Header.Add("Idempotency-Key", "idempotency-key-0002") }},
		{name: "comma joined idempotency keys", field: "idempotencyKey", addHeaders: func(r *http.Request) { r.Header.Set("Idempotency-Key", "idempotency-key-0001,idempotency-key-0002") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			called := false
			router := httpapi.NewRouter(httpapi.Dependencies{ConfirmAppointment: func(context.Context, application.ConfirmCommand) (application.ConfirmationResult, error) {
				called = true
				return application.ConfirmationResult{}, nil
			}})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments", strings.NewReader(validConfirmJSON()))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", "idempotency-key-0001")
			test.addHeaders(req)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			if called {
				t.Fatal("gateway called for ambiguous headers")
			}
			assertAPIError(t, response, "VALIDATION_ERROR", test.field, true)
		})
	}
}

func TestConfirmAppointmentMapsApplicationErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "resource conflict", err: application.ErrResourceConflict, status: 409, code: "RESOURCE_CONFLICT"},
		{name: "idempotency conflict", err: application.ErrIdempotencyConflict, status: 409, code: "IDEMPOTENCY_CONFLICT"},
		{name: "invalid reference", err: application.ErrInvalidReference, status: 400, code: "VALIDATION_ERROR"},
		{name: "typed validation", err: &application.ValidationError{Field: "startAt", Reason: "must be future"}, status: 400, code: "VALIDATION_ERROR"},
		{name: "cancellation", err: context.Canceled, status: 500, code: "INTERNAL_ERROR"},
		{name: "safe internal", err: errors.New("postgres://private:secret@10.0.0.1/db"), status: 500, code: "INTERNAL_ERROR"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := httpapi.NewRouter(httpapi.Dependencies{ConfirmAppointment: func(context.Context, application.ConfirmCommand) (application.ConfirmationResult, error) {
				return application.ConfirmationResult{}, test.err
			}})
			response := appointmentRequest(router, http.MethodPost, "/api/v1/appointments", validConfirmJSON(), "idempotency-key-0001")
			if response.Code != test.status {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			assertAPIError(t, response, test.code, map[bool]string{true: "startAt"}[test.name == "typed validation"], test.name == "typed validation")
			if strings.Contains(response.Body.String(), "private") || strings.Contains(response.Body.String(), "10.0.0.1") {
				t.Fatal("backend detail leaked")
			}
		})
	}
}

func TestRetrieveAppointmentReturnsCompletePersistedContract(t *testing.T) {
	want := completeAppointment()
	var gotID string
	router := httpapi.NewRouter(httpapi.Dependencies{AppointmentByID: func(_ context.Context, id string) (application.Appointment, error) { gotID = id; return want, nil }})
	response := appointmentRequest(router, http.MethodGet, "/api/v1/appointments/"+testAppointmentID, "", "")
	if response.Code != 200 {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if gotID != testAppointmentID {
		t.Fatalf("id = %q", gotID)
	}
	assertAppointmentResponse(t, response, want)
}

func TestRetrieveAppointmentValidatesExactPathAndMapsErrors(t *testing.T) {
	tests := []struct {
		name, target string
		method       string
		err          error
		status       int
		code         string
		called       bool
	}{
		{name: "not found", target: "/api/v1/appointments/" + testAppointmentID, method: http.MethodGet, err: application.ErrNotFound, status: 404, code: "NOT_FOUND", called: true},
		{name: "malformed id", target: "/api/v1/appointments/not-a-uuid", method: http.MethodGet, status: 400, code: "VALIDATION_ERROR"},
		{name: "trailing path", target: "/api/v1/appointments/" + testAppointmentID + "/extra", method: http.MethodGet, status: 404, code: "NOT_FOUND"},
		{name: "wrong method", target: "/api/v1/appointments/" + testAppointmentID, method: http.MethodDelete, status: 405, code: "METHOD_NOT_ALLOWED"},
		{name: "cancellation", target: "/api/v1/appointments/" + testAppointmentID, method: http.MethodGet, err: context.Canceled, status: 500, code: "INTERNAL_ERROR", called: true},
		{name: "safe internal", target: "/api/v1/appointments/" + testAppointmentID, method: http.MethodGet, err: errors.New("password=secret"), status: 500, code: "INTERNAL_ERROR", called: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			called := false
			router := httpapi.NewRouter(httpapi.Dependencies{AppointmentByID: func(context.Context, string) (application.Appointment, error) {
				called = true
				return application.Appointment{}, test.err
			}})
			response := appointmentRequest(router, test.method, test.target, "", "")
			if response.Code != test.status {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			if called != test.called {
				t.Fatalf("called = %v, want %v", called, test.called)
			}
			assertAPIError(t, response, test.code, map[bool]string{true: "appointmentId"}[test.name == "malformed id"], test.name == "malformed id")
			if strings.Contains(response.Body.String(), "secret") {
				t.Fatal("backend detail leaked")
			}
		})
	}
}

func TestAppointmentRoutesRejectUncleanPathsWithoutRedirect(t *testing.T) {
	tests := []string{
		"/api/v1/appointments//" + testAppointmentID,
		"/api/v1/appointments/./" + testAppointmentID,
		"/api/v1/appointments/../appointments/" + testAppointmentID,
		"/api/v1/appointments/%2e/" + testAppointmentID,
		"/api/v1/appointments/%2e%2e/appointments/" + testAppointmentID,
	}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			called := false
			router := httpapi.NewRouter(httpapi.Dependencies{AppointmentByID: func(context.Context, string) (application.Appointment, error) {
				called = true
				return application.Appointment{}, nil
			}})
			req := httptest.NewRequest(http.MethodGet, target, nil)
			req.Header.Set("X-Request-ID", "raw-path-test")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			if called {
				t.Fatal("repository called for malformed path")
			}
			if response.Header().Get("Location") != "" {
				t.Fatalf("redirect location = %q", response.Header().Get("Location"))
			}
			assertAPIError(t, response, "NOT_FOUND", "", false)
			if response.Header().Get("X-Request-ID") != "raw-path-test" {
				t.Fatalf("request ID = %q", response.Header().Get("X-Request-ID"))
			}
		})
	}
}

func TestAppointmentHTTPContractWithPostgreSQL(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := postgres.Migrate(context.Background(), pool, migrations.Files); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, err = pool.Exec(ctx, `update service_bays set active=false where id='20000000-0000-0000-0000-000000000062'`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `update service_bays set active=true where id='20000000-0000-0000-0000-000000000062'`)
	})
	start := "2041-03-04T09:30:00Z"
	_, _ = pool.Exec(ctx, `delete from idempotency_records where idempotency_key like 'http-contract-key-%'`)
	_, _ = pool.Exec(ctx, `delete from appointments where vehicle_id='20000000-0000-0000-0000-000000000011' and start_at::date='2041-03-04'`)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `delete from idempotency_records where idempotency_key like 'http-contract-key-%'`)
		_, _ = pool.Exec(context.Background(), `delete from appointments where vehicle_id='20000000-0000-0000-0000-000000000011' and start_at::date='2041-03-04'`)
	})
	repository := postgres.NewRepository(pool)
	router := httpapi.NewRouter(httpapi.Dependencies{ConfirmAppointment: repository.Confirm, AppointmentByID: repository.AppointmentByID})
	body := `{"vehicleId":"20000000-0000-0000-0000-000000000011","dealershipId":"20000000-0000-0000-0000-000000000021","serviceTypeId":"20000000-0000-0000-0000-000000000042","startAt":"` + start + `"}`
	created := appointmentRequest(router, http.MethodPost, "/api/v1/appointments", body, "http-contract-key-0001")
	if created.Code != 201 {
		t.Fatalf("create = %d: %s", created.Code, created.Body.String())
	}
	var appointment struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &appointment); err != nil {
		t.Fatal(err)
	}
	replayed := appointmentRequest(router, http.MethodPost, "/api/v1/appointments", body, "http-contract-key-0001")
	if replayed.Code != 200 || replayed.Body.String() != created.Body.String() {
		t.Fatalf("replay = %d: %s", replayed.Code, replayed.Body.String())
	}
	newPool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer newPool.Close()
	readRouter := httpapi.NewRouter(httpapi.Dependencies{AppointmentByID: postgres.NewRepository(newPool).AppointmentByID})
	retrieved := appointmentRequest(readRouter, http.MethodGet, "/api/v1/appointments/"+appointment.ID, "", "")
	if retrieved.Code != 200 || retrieved.Body.String() != created.Body.String() {
		t.Fatalf("retrieve = %d: %s", retrieved.Code, retrieved.Body.String())
	}
	conflict := appointmentRequest(router, http.MethodPost, "/api/v1/appointments", body, "http-contract-key-0002")
	if conflict.Code != 409 {
		t.Fatalf("conflict = %d: %s", conflict.Code, conflict.Body.String())
	}
	assertAPIError(t, conflict, "RESOURCE_CONFLICT", "", false)
	for index, duplicateHeader := range []string{"Content-Type", "Idempotency-Key"} {
		ambiguousBody := strings.Replace(body, start, []string{"2041-03-04T11:00:00Z", "2041-03-04T13:00:00Z"}[index], 1)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments", strings.NewReader(ambiguousBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "http-contract-key-0003")
		req.Header.Add(duplicateHeader, map[string]string{"Content-Type": "application/json", "Idempotency-Key": "http-contract-key-0004"}[duplicateHeader])
		ambiguous := httptest.NewRecorder()
		router.ServeHTTP(ambiguous, req)
		if ambiguous.Code != http.StatusBadRequest {
			t.Fatalf("ambiguous %s = %d: %s", duplicateHeader, ambiguous.Code, ambiguous.Body.String())
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `select count(*) from appointments where vehicle_id='20000000-0000-0000-0000-000000000011' and start_at::date='2041-03-04'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("appointment rows = %d", count)
	}
}

func validConfirmJSON() string {
	return `{"vehicleId":"` + testVehicleID + `","dealershipId":"` + testDealershipID + `","serviceTypeId":"` + testServiceTypeID + `","startAt":"2031-03-04T09:30:00Z"}`
}

func completeAppointment() application.Appointment {
	return application.Appointment{ID: testAppointmentID, CustomerID: testCustomerID, VehicleID: testVehicleID, DealershipID: testDealershipID, ServiceTypeID: testServiceTypeID, TechnicianID: testTechnicianID, ServiceBayID: testBayID, Status: "CONFIRMED", StartAt: time.Date(2031, 3, 4, 9, 30, 0, 0, time.FixedZone("offset", 3600)), EndAt: time.Date(2031, 3, 4, 10, 30, 0, 0, time.FixedZone("offset", 3600)), CustomerName: "Customer", VehicleLabel: "Blue hatchback", Registration: "DEMO 101", DealershipName: "Riverside Service Centre", DealershipAddress: "100 Riverside Way", DealershipTimeZone: "Europe/London", ServiceTypeName: "Annual service", ServiceTypeDescription: "Routine inspection", ServiceDurationMinutes: 60, TechnicianName: "Taylor", ServiceBayName: "Bay A"}
}

func appointmentRequest(handler http.Handler, method, target, body, key string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func assertAppointmentResponse(t *testing.T, response *httptest.ResponseRecorder, want application.Appointment) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	wantJSON := map[string]any{
		"id": want.ID, "status": "CONFIRMED", "startAt": "2031-03-04T08:30:00Z", "endAt": "2031-03-04T09:30:00Z",
		"vehicle":     map[string]any{"id": want.VehicleID, "customerId": want.CustomerID, "label": want.VehicleLabel, "registration": want.Registration},
		"dealership":  map[string]any{"id": want.DealershipID, "name": want.DealershipName, "address": want.DealershipAddress, "timezone": want.DealershipTimeZone},
		"serviceType": map[string]any{"id": want.ServiceTypeID, "name": want.ServiceTypeName, "description": want.ServiceTypeDescription, "durationMinutes": float64(want.ServiceDurationMinutes)},
		"technician":  map[string]any{"id": want.TechnicianID, "name": want.TechnicianName},
		"serviceBay":  map[string]any{"id": want.ServiceBayID, "name": want.ServiceBayName},
	}
	if stringMustJSON(got) != stringMustJSON(wantJSON) {
		t.Fatalf("body = %#v, want %#v", got, wantJSON)
	}
}

func stringMustJSON(value any) string { encoded, _ := json.Marshal(value); return string(encoded) }
