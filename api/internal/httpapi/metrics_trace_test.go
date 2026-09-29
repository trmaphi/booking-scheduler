package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"scheduler/api/internal/application"
	"scheduler/api/internal/httpapi"
	"scheduler/api/internal/telemetry"
)

type capturedMetric struct {
	correlation telemetry.Correlation
	event       telemetry.MetricEvent
}
type metricRecorder struct{ metrics []capturedMetric }

func (*metricRecorder) RequestCompleted(context.Context, telemetry.RequestEvent)  {}
func (*metricRecorder) Availability(context.Context, telemetry.AvailabilityEvent) {}
func (*metricRecorder) Confirmation(context.Context, telemetry.ConfirmationEvent) {}
func (*metricRecorder) Database(context.Context, telemetry.DatabaseEvent)         {}
func (r *metricRecorder) Metric(ctx context.Context, e telemetry.MetricEvent) {
	r.metrics = append(r.metrics, capturedMetric{telemetry.CorrelationFromContext(ctx), e})
}

type fakeClock struct {
	values []time.Time
	index  int
}

func (c *fakeClock) Now() time.Time { value := c.values[c.index]; c.index++; return value }

func TestMetricHTTPDurationAndErrorUseFakeClock(t *testing.T) {
	recorder := &metricRecorder{}
	clock := &fakeClock{values: []time.Time{time.Unix(100, 0), time.Unix(100, int64(250*time.Millisecond))}}
	router := httpapi.NewRouter(httpapi.Dependencies{Telemetry: recorder, Clock: clock, BookingOptions: func(context.Context) (application.BookingOptions, error) {
		return application.BookingOptions{}, application.ErrPersistence
	}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/booking-options", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != 500 {
		t.Fatalf("status = %d", response.Code)
	}
	if len(recorder.metrics) != 2 || recorder.metrics[0].event.Name != telemetry.MetricHTTPDuration || recorder.metrics[0].event.Value != 250 || recorder.metrics[1].event.Name != telemetry.MetricHTTPErrors {
		t.Fatalf("metrics = %#v", recorder.metrics)
	}
}

func TestTraceParentResponseAndContextCorrelation(t *testing.T) {
	recorder := &metricRecorder{}
	var databaseTrace telemetry.TraceContext
	router := httpapi.NewRouter(httpapi.Dependencies{Telemetry: recorder, BookingOptions: func(ctx context.Context) (application.BookingOptions, error) {
		databaseTrace = telemetry.TraceFromContext(ctx)
		return application.BookingOptions{}, nil
	}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/booking-options", nil)
	req.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if databaseTrace.TraceID != "0123456789abcdef0123456789abcdef" || databaseTrace.SpanID == "0123456789abcdef" {
		t.Fatalf("database trace = %#v", databaseTrace)
	}
	if got := response.Header().Get("traceparent"); got != databaseTrace.Header() {
		t.Fatalf("response traceparent = %q, want %q", got, databaseTrace.Header())
	}
	if len(recorder.metrics) == 0 || recorder.metrics[0].correlation.TraceID != databaseTrace.TraceID || recorder.metrics[0].correlation.SpanID != databaseTrace.SpanID {
		t.Fatalf("metric correlation = %#v", recorder.metrics)
	}
}

func TestInvalidOrMultipleTraceParentIsNeverEchoed(t *testing.T) {
	for _, values := range [][]string{{"PRIVATE-TRACE"}, {"00-0123456789abcdef0123456789abcdef-0123456789abcdef-01", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"}} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
		for _, value := range values {
			req.Header.Add("traceparent", value)
		}
		response := httptest.NewRecorder()
		httpapi.NewRouter(httpapi.Dependencies{}).ServeHTTP(response, req)
		if got := response.Header().Get("traceparent"); got == "" || got == values[0] {
			t.Fatalf("unsafe response traceparent = %q", got)
		}
	}
}

func TestMetricConfirmationDurationUsesFakeClockAndResultDimensions(t *testing.T) {
	recorder := &metricRecorder{}
	base := time.Unix(500, 0)
	clock := &fakeClock{values: []time.Time{base, base.Add(10 * time.Millisecond), base.Add(85 * time.Millisecond), base.Add(100 * time.Millisecond)}}
	router := httpapi.NewRouter(httpapi.Dependencies{Telemetry: recorder, Clock: clock, ConfirmAppointment: func(context.Context, application.ConfirmCommand) (application.ConfirmationResult, error) {
		return application.ConfirmationResult{RetryCount: 2}, application.ErrResourceConflict
	}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments", strings.NewReader(`{"vehicleId":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","dealershipId":"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb","serviceTypeId":"cccccccc-cccc-cccc-cccc-cccccccccccc","startAt":"2031-03-04T09:00:00Z"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "idempotency-key-0001")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	var found *telemetry.MetricEvent
	for i := range recorder.metrics {
		if recorder.metrics[i].event.Name == telemetry.MetricConfirmationDuration {
			event := recorder.metrics[i].event
			found = &event
		}
	}
	if found == nil || found.Value != 75 || found.DealershipID != "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb" || found.ServiceTypeID != "cccccccc-cccc-cccc-cccc-cccccccccccc" || found.Result != telemetry.ResultResourceConflict || found.RetryCount != 2 {
		t.Fatalf("confirmation duration = %#v; all metrics %#v", found, recorder.metrics)
	}
}
