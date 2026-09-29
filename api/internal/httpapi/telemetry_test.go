package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"scheduler/api/internal/application"
	"scheduler/api/internal/domain"
	"scheduler/api/internal/httpapi"
	"scheduler/api/internal/telemetry"
)

type memoryRecorder struct {
	mu            sync.Mutex
	requests      []recordedRequest
	availability  []recordedAvailability
	confirmations []recordedConfirmation
}
type recordedRequest struct {
	correlation telemetry.Correlation
	event       telemetry.RequestEvent
}
type recordedAvailability struct {
	correlation telemetry.Correlation
	event       telemetry.AvailabilityEvent
}
type recordedConfirmation struct {
	correlation telemetry.Correlation
	event       telemetry.ConfirmationEvent
}

func (m *memoryRecorder) RequestCompleted(ctx context.Context, event telemetry.RequestEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, recordedRequest{telemetry.CorrelationFromContext(ctx), event})
}
func (m *memoryRecorder) Availability(ctx context.Context, event telemetry.AvailabilityEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.availability = append(m.availability, recordedAvailability{telemetry.CorrelationFromContext(ctx), event})
}
func (m *memoryRecorder) Confirmation(ctx context.Context, event telemetry.ConfirmationEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.confirmations = append(m.confirmations, recordedConfirmation{telemetry.CorrelationFromContext(ctx), event})
}
func (m *memoryRecorder) Database(context.Context, telemetry.DatabaseEvent) {}
func (m *memoryRecorder) Metric(context.Context, telemetry.MetricEvent)     {}

func TestTelemetryEmitsExactlyOneStructuredCompletionForEveryOutcome(t *testing.T) {
	tests := []struct {
		name, method, target, body string
		configure                  func(*httpapi.Dependencies)
		wantStatus                 int
		wantRoute                  telemetry.Route
		wantResult                 telemetry.Result
	}{
		{name: "success", method: http.MethodGet, target: "/api/v1/booking-options", configure: func(d *httpapi.Dependencies) {
			d.BookingOptions = func(context.Context) (application.BookingOptions, error) { return application.BookingOptions{}, nil }
		}, wantStatus: 200, wantRoute: telemetry.RouteBookingOptions, wantResult: telemetry.ResultSuccess},
		{name: "malformed JSON", method: http.MethodPost, target: "/api/v1/appointments", body: `{"customer":"PRIVATE-RAW-BODY"`, wantStatus: 400, wantRoute: telemetry.RouteAppointments, wantResult: telemetry.ResultValidation},
		{name: "validation", method: http.MethodGet, target: "/api/v1/availability?vehicleId=PRIVATE-VEHICLE-ID", wantStatus: 400, wantRoute: telemetry.RouteAvailability, wantResult: telemetry.ResultValidation},
		{name: "not found", method: http.MethodGet, target: "/PRIVATE-PATH-CREDENTIALS", wantStatus: 404, wantRoute: telemetry.RouteUnknown, wantResult: telemetry.ResultNotFound},
		{name: "database outage", method: http.MethodGet, target: "/api/v1/booking-options", configure: func(d *httpapi.Dependencies) {
			d.BookingOptions = func(context.Context) (application.BookingOptions, error) {
				return application.BookingOptions{}, errors.New("postgresql://private:secret@database.invalid PRIVATE-PGX-ERROR")
			}
		}, wantStatus: 500, wantRoute: telemetry.RouteBookingOptions, wantResult: telemetry.ResultDatabaseError},
		{name: "resource conflict", method: http.MethodPost, target: "/api/v1/appointments", body: validConfirmationBody(), configure: func(d *httpapi.Dependencies) {
			d.ConfirmAppointment = func(context.Context, application.ConfirmCommand) (application.ConfirmationResult, error) {
				return application.ConfirmationResult{}, application.ErrResourceConflict
			}
		}, wantStatus: 409, wantRoute: telemetry.RouteAppointments, wantResult: telemetry.ResultResourceConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &memoryRecorder{}
			deps := httpapi.Dependencies{Telemetry: recorder}
			if tt.configure != nil {
				tt.configure(&deps)
			}
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			req.Header.Set("X-Request-ID", "request-safe")
			if tt.method == http.MethodPost {
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Idempotency-Key", "PRIVATE-IDEMPOTENCY-KEY-123")
			}
			response := httptest.NewRecorder()
			httpapi.NewRouter(deps).ServeHTTP(response, req)
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.wantStatus, response.Body.String())
			}
			if len(recorder.requests) != 1 {
				t.Fatalf("completion events = %d, want 1", len(recorder.requests))
			}
			got := recorder.requests[0]
			if got.event.Method != telemetry.Method(tt.method) || got.event.Route != tt.wantRoute || got.event.Status != tt.wantStatus || got.event.Result != tt.wantResult {
				t.Errorf("completion = %#v", got.event)
			}
			if got.event.Duration < 0 || got.correlation.RequestID != "request-safe" {
				t.Errorf("correlation/duration = %#v, %v", got.correlation, got.event.Duration)
			}
			serialized := fmt.Sprintf("%#v", recorder)
			for _, private := range []string{"PRIVATE-RAW-BODY", "PRIVATE-PATH-CREDENTIALS", "PRIVATE-IDEMPOTENCY-KEY", "PRIVATE-VEHICLE-ID", "PRIVATE-PGX-ERROR", "private:secret"} {
				if strings.Contains(serialized, private) {
					t.Errorf("events leaked %q: %s", private, serialized)
				}
			}
		})
	}
}

func TestStructuredAvailabilityAndConfirmationEventsAreCorrelated(t *testing.T) {
	recorder := &memoryRecorder{}
	start := time.Date(2031, 3, 4, 9, 0, 0, 0, time.UTC)
	interval, err := domain.NewInterval(start, start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	deps := httpapi.Dependencies{Telemetry: recorder, AvailableSlots: func(context.Context, application.AvailabilityQuery) ([]domain.Interval, error) {
		return []domain.Interval{interval}, nil
	}, ConfirmAppointment: func(context.Context, application.ConfirmCommand) (application.ConfirmationResult, error) {
		return application.ConfirmationResult{Replayed: true}, nil
	}}
	availabilityURL := "/api/v1/availability?vehicleId=aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa&dealershipId=bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb&serviceTypeId=cccccccc-cccc-cccc-cccc-cccccccccccc&date=2031-03-04"
	request := httptest.NewRequest(http.MethodGet, availabilityURL, nil)
	request.Header.Set("X-Request-ID", "availability-request")
	httpapi.NewRouter(deps).ServeHTTP(httptest.NewRecorder(), request)
	confirm := httptest.NewRequest(http.MethodPost, "/api/v1/appointments", strings.NewReader(validConfirmationBody()))
	confirm.Header.Set("Content-Type", "application/json")
	confirm.Header.Set("Idempotency-Key", "PRIVATE-IDEMPOTENCY-KEY-123")
	confirm.Header.Set("X-Request-ID", "confirmation-request")
	httpapi.NewRouter(deps).ServeHTTP(httptest.NewRecorder(), confirm)
	if len(recorder.availability) != 1 || recorder.availability[0].event.Count != 1 || recorder.availability[0].event.DealershipID != "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb" || recorder.availability[0].correlation.RequestID != "availability-request" {
		t.Errorf("availability events = %#v", recorder.availability)
	}
	if len(recorder.confirmations) != 1 || recorder.confirmations[0].event.Result != telemetry.ResultReplay || recorder.confirmations[0].event.DealershipID != "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb" || recorder.confirmations[0].correlation.RequestID != "confirmation-request" {
		t.Errorf("confirmation events = %#v", recorder.confirmations)
	}
}

func TestTelemetryCorrelationIsIsolatedAcrossParallelRequests(t *testing.T) {
	recorder := &memoryRecorder{}
	router := httpapi.NewRouter(httpapi.Dependencies{Telemetry: recorder, BookingOptions: func(context.Context) (application.BookingOptions, error) { return application.BookingOptions{}, nil }})
	const requests = 32
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("request-%d", i)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/booking-options", nil)
			req.Header.Set("X-Request-ID", id)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Header().Get("X-Request-ID") != id {
				t.Errorf("response request ID = %q, want %q", response.Header().Get("X-Request-ID"), id)
			}
		}(i)
	}
	wg.Wait()
	if len(recorder.requests) != requests {
		t.Fatalf("events = %d, want %d", len(recorder.requests), requests)
	}
	seen := map[string]bool{}
	for _, item := range recorder.requests {
		if seen[item.correlation.RequestID] {
			t.Errorf("duplicate correlation %q", item.correlation.RequestID)
		}
		seen[item.correlation.RequestID] = true
	}
}

func TestPrivacyDoesNotTrustUUIDsFromFailedRequestsAsSafeDimensions(t *testing.T) {
	var output bytes.Buffer
	recorder := telemetry.NewJSONRecorder(&output)
	customerSentinel := "33333333-3333-3333-3333-333333333333"
	vehicleSentinel := "44444444-4444-4444-4444-444444444444"
	target := "/api/v1/availability?vehicleId=aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa&dealershipId=" + customerSentinel + "&serviceTypeId=" + vehicleSentinel + "&date=2031-03-04"
	request := httptest.NewRequest(http.MethodGet, target, nil)
	router := httpapi.NewRouter(httpapi.Dependencies{Telemetry: recorder, AvailableSlots: func(context.Context, application.AvailabilityQuery) ([]domain.Interval, error) {
		return nil, application.ErrInvalidReference
	}})
	router.ServeHTTP(httptest.NewRecorder(), request)
	for _, sentinel := range []string{customerSentinel, vehicleSentinel} {
		if strings.Contains(output.String(), sentinel) {
			t.Errorf("failed request leaked untrusted identifier %q: %s", sentinel, output.String())
		}
	}
}

func TestTelemetryPanicDoesNotChangeHTTPBehavior(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/booking-options", nil)
	response := httptest.NewRecorder()
	httpapi.NewRouter(httpapi.Dependencies{Telemetry: panicRecorder{}, BookingOptions: func(context.Context) (application.BookingOptions, error) { return application.BookingOptions{}, nil }}).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
}

type panicRecorder struct{}

func (panicRecorder) RequestCompleted(context.Context, telemetry.RequestEvent) {
	panic("telemetry unavailable")
}
func (panicRecorder) Availability(context.Context, telemetry.AvailabilityEvent) {
	panic("telemetry unavailable")
}
func (panicRecorder) Confirmation(context.Context, telemetry.ConfirmationEvent) {
	panic("telemetry unavailable")
}
func (panicRecorder) Database(context.Context, telemetry.DatabaseEvent) {
	panic("telemetry unavailable")
}
func (panicRecorder) Metric(context.Context, telemetry.MetricEvent) { panic("telemetry unavailable") }
func validConfirmationBody() string {
	return `{"vehicleId":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","dealershipId":"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb","serviceTypeId":"cccccccc-cccc-cccc-cccc-cccccccccccc","startAt":"2031-03-04T09:00:00Z"}`
}

func TestTelemetryClassifiesUnstartedPanicAsInternalAndRepanics(t *testing.T) {
	recorder := &memoryRecorder{}
	router := httpapi.NewRouter(httpapi.Dependencies{
		Telemetry:      recorder,
		BookingOptions: func(context.Context) (application.BookingOptions, error) { panic("PRIVATE-PANIC") },
	})
	panicValue := serveAndRecover(router, httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/booking-options", nil))
	if panicValue != "PRIVATE-PANIC" {
		t.Fatalf("panic = %#v, want original value", panicValue)
	}
	if len(recorder.requests) != 1 {
		t.Fatalf("completion events = %d, want exactly 1", len(recorder.requests))
	}
	got := recorder.requests[0].event
	if got.Status != http.StatusInternalServerError || got.Result != telemetry.ResultInternal {
		t.Fatalf("completion = %#v, want internal 500", got)
	}
}

func TestTelemetryRetainsStartedResponseStatusAndRepanics(t *testing.T) {
	recorder := &memoryRecorder{}
	router := httpapi.NewRouter(httpapi.Dependencies{
		Telemetry:      recorder,
		BookingOptions: func(context.Context) (application.BookingOptions, error) { return application.BookingOptions{}, nil },
	})
	writer := &panicOnWriteRecorder{ResponseRecorder: httptest.NewRecorder()}
	panicValue := serveAndRecover(router, writer, httptest.NewRequest(http.MethodGet, "/api/v1/booking-options", nil))
	if panicValue != "write panic" {
		t.Fatalf("panic = %#v, want write panic", panicValue)
	}
	if len(recorder.requests) != 1 {
		t.Fatalf("completion events = %d, want exactly 1", len(recorder.requests))
	}
	got := recorder.requests[0].event
	if got.Status != http.StatusOK || got.Result != telemetry.ResultSuccess {
		t.Fatalf("completion = %#v, want started 200", got)
	}
}

func serveAndRecover(handler http.Handler, writer http.ResponseWriter, request *http.Request) (recovered any) {
	defer func() { recovered = recover() }()
	handler.ServeHTTP(writer, request)
	return nil
}

type panicOnWriteRecorder struct{ *httptest.ResponseRecorder }

func (w *panicOnWriteRecorder) Write(body []byte) (int, error) { panic("write panic") }

func TestPrivacyActualJSONAcrossHTTPBookingOutcomes(t *testing.T) {
	const (
		customerName     = "PRIVATE-CUSTOMER-NAME-HTTP"
		customerEmail    = "private-http@example.invalid"
		registration     = "PRIVATE-REGISTRATION-HTTP"
		customerID       = "33333333-3333-4333-8333-333333333333"
		vehicleID        = "44444444-4444-4444-8444-444444444444"
		idempotencyKey   = "PRIVATE-IDEMPOTENCY-KEY-HTTP"
		rawBody          = "PRIVATE-RAW-BODY-HTTP"
		databaseURL      = "postgresql://private-http:secret@database.invalid/scheduler"
		rawDatabaseError = "PRIVATE-PGX-ERROR-HTTP"
		dealershipID     = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
		serviceTypeID    = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	)
	var output bytes.Buffer
	recorder := telemetry.NewJSONRecorder(&output)
	appointment := application.Appointment{CustomerID: customerID, CustomerName: customerName, VehicleID: vehicleID, Registration: registration, DealershipID: dealershipID, ServiceTypeID: serviceTypeID}
	router := httpapi.NewRouter(httpapi.Dependencies{
		Telemetry: recorder,
		ConfirmAppointment: func(context.Context, application.ConfirmCommand) (application.ConfirmationResult, error) {
			return application.ConfirmationResult{Appointment: appointment}, nil
		},
		AvailableSlots: func(context.Context, application.AvailabilityQuery) ([]domain.Interval, error) {
			return nil, application.ErrInvalidReference
		},
		BookingOptions: func(context.Context) (application.BookingOptions, error) {
			return application.BookingOptions{}, errors.New(databaseURL + " " + rawDatabaseError)
		},
	})

	successBody := fmt.Sprintf(`{"vehicleId":%q,"dealershipId":%q,"serviceTypeId":%q,"startAt":"2031-03-04T09:00:00Z"}`, vehicleID, dealershipID, serviceTypeID)
	serveJSONRequest(router, http.MethodPost, "/api/v1/appointments", successBody, idempotencyKey)
	serveJSONRequest(router, http.MethodPost, "/api/v1/appointments", `{"customerName":"`+customerName+`","email":"`+customerEmail+`","registration":"`+registration+`","raw":"`+rawBody, idempotencyKey)
	availabilityTarget := fmt.Sprintf("/api/v1/availability?vehicleId=%s&dealershipId=%s&serviceTypeId=%s&date=2031-03-04", vehicleID, customerID, serviceTypeID)
	serveJSONRequest(router, http.MethodGet, availabilityTarget, "", "")
	serveJSONRequest(router, http.MethodGet, "/api/v1/booking-options", "", "")
	conflictRouter := httpapi.NewRouter(httpapi.Dependencies{Telemetry: recorder, ConfirmAppointment: func(context.Context, application.ConfirmCommand) (application.ConfirmationResult, error) {
		return application.ConfirmationResult{}, application.ErrResourceConflict
	}})
	serveJSONRequest(conflictRouter, http.MethodPost, "/api/v1/appointments", successBody, idempotencyKey)

	assertJSONTelemetryExcludes(t, output.String(), []string{customerName, customerEmail, registration, customerID, vehicleID, idempotencyKey, rawBody, databaseURL, rawDatabaseError})
}

func serveJSONRequest(handler http.Handler, method, target, body, idempotencyKey string) {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	handler.ServeHTTP(httptest.NewRecorder(), request)
}

func assertJSONTelemetryExcludes(t *testing.T, output string, sentinels []string) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		t.Fatal("no JSON telemetry emitted")
	}
	for index, line := range lines {
		var value any
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			t.Fatalf("log %d is not JSON: %v: %s", index, err, line)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, sentinel := range sentinels {
			if strings.Contains(string(encoded), sentinel) {
				t.Errorf("JSON telemetry leaked sentinel %q in event %d", sentinel, index)
			}
		}
	}
}
