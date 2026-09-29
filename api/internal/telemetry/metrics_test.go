package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"scheduler/api/internal/telemetry"
)

func TestMetricEventsAreDeterministicTypedAndBounded(t *testing.T) {
	var output bytes.Buffer
	recorder := telemetry.NewJSONRecorder(&output)
	ctx := telemetry.WithCorrelation(context.Background(), "request-1", "0123456789abcdef0123456789abcdef")

	telemetry.RecordRequest(recorder, ctx, telemetry.RequestEvent{Method: "POST", Route: telemetry.RouteAppointments, Status: 409, Result: telemetry.ResultResourceConflict, Duration: 1250 * time.Millisecond})
	telemetry.RecordAvailability(recorder, ctx, telemetry.AvailabilityEvent{DealershipID: "11111111-1111-1111-1111-111111111111", ServiceTypeID: "22222222-2222-2222-2222-222222222222", Result: telemetry.ResultSuccess, Count: 3})
	telemetry.RecordConfirmation(recorder, ctx, telemetry.ConfirmationEvent{DealershipID: "11111111-1111-1111-1111-111111111111", ServiceTypeID: "22222222-2222-2222-2222-222222222222", Result: telemetry.ResultIdempotencyConflict, RetryCount: 2})
	telemetry.RecordDatabase(recorder, ctx, telemetry.DatabaseEvent{Operation: telemetry.DatabaseConfirm, Result: telemetry.ResultCanceled, Duration: 45 * time.Millisecond})

	var metrics []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event["event"] == "metric.recorded" {
			metrics = append(metrics, event)
		}
	}
	want := []struct {
		name, unit string
		value      float64
	}{
		{"http.server.duration", "ms", 1250}, {"http.server.errors", "1", 1},
		{"availability.result_count", "1", 3}, {"confirmation.attempts", "1", 1},
		{"confirmation.duration", "ms", 0}, {"allocation.retries", "1", 2}, {"booking.conflicts", "1", 1},
		{"database.duration", "ms", 45}, {"database.errors", "1", 1},
	}
	if len(metrics) != len(want) {
		t.Fatalf("metric count = %d, want %d: %s", len(metrics), len(want), output.String())
	}
	for i, expected := range want {
		if metrics[i]["name"] != expected.name || metrics[i]["unit"] != expected.unit || metrics[i]["value"] != expected.value {
			t.Errorf("metric %d = %#v, want %s %s %v", i, metrics[i], expected.name, expected.unit, expected.value)
		}
		for key := range metrics[i] {
			switch key {
			case "time", "level", "msg", "event", "request_id", "trace_id", "span_id", "name", "unit", "value", "method", "route", "status", "result", "operation", "dealership_id", "service_type_id", "conflict_class", "retry_count":
			default:
				t.Errorf("unexpected metric attribute %q", key)
			}
		}
	}
}

func TestMetricNormalizationRejectsUnboundedValues(t *testing.T) {
	var output bytes.Buffer
	recorder := telemetry.NewJSONRecorder(&output)
	recorder.Metric(context.Background(), telemetry.MetricEvent{Name: telemetry.MetricName("private.metric"), Unit: telemetry.MetricUnit("bytes"), Value: -1, Route: telemetry.Route("/private/path"), Result: telemetry.Result("private-result"), ConflictClass: telemetry.ConflictClass("private-conflict")})
	if strings.Contains(output.String(), "private") || strings.Contains(output.String(), "bytes") {
		t.Fatalf("unbounded metric value leaked: %s", output.String())
	}
}

type memoryMetricRecorder struct{ events []telemetry.MetricEvent }

func (*memoryMetricRecorder) RequestCompleted(context.Context, telemetry.RequestEvent)  {}
func (*memoryMetricRecorder) Availability(context.Context, telemetry.AvailabilityEvent) {}
func (*memoryMetricRecorder) Confirmation(context.Context, telemetry.ConfirmationEvent) {}
func (*memoryMetricRecorder) Database(context.Context, telemetry.DatabaseEvent)         {}
func (r *memoryMetricRecorder) Metric(_ context.Context, event telemetry.MetricEvent) {
	r.events = append(r.events, event)
}

func TestMetricEventsCoverRequiredBookingOutcomes(t *testing.T) {
	tests := []struct {
		name   string
		record func(telemetry.Recorder)
		want   []telemetry.MetricName
	}{
		{"http success", func(r telemetry.Recorder) {
			telemetry.RecordRequest(r, context.Background(), telemetry.RequestEvent{Status: 200, Result: telemetry.ResultSuccess})
		}, []telemetry.MetricName{telemetry.MetricHTTPDuration}},
		{"http validation", func(r telemetry.Recorder) {
			telemetry.RecordRequest(r, context.Background(), telemetry.RequestEvent{Status: 400, Result: telemetry.ResultValidation})
		}, []telemetry.MetricName{telemetry.MetricHTTPDuration, telemetry.MetricHTTPErrors}},
		{"http not found", func(r telemetry.Recorder) {
			telemetry.RecordRequest(r, context.Background(), telemetry.RequestEvent{Status: 404, Result: telemetry.ResultNotFound})
		}, []telemetry.MetricName{telemetry.MetricHTTPDuration, telemetry.MetricHTTPErrors}},
		{"no availability", func(r telemetry.Recorder) {
			telemetry.RecordAvailability(r, context.Background(), telemetry.AvailabilityEvent{Result: telemetry.ResultSuccess, Count: 0})
		}, []telemetry.MetricName{telemetry.MetricAvailabilityCount}},
		{"created", func(r telemetry.Recorder) {
			telemetry.RecordConfirmation(r, context.Background(), telemetry.ConfirmationEvent{Result: telemetry.ResultCreated})
		}, []telemetry.MetricName{telemetry.MetricConfirmation, telemetry.MetricConfirmationDuration}},
		{"replay", func(r telemetry.Recorder) {
			telemetry.RecordConfirmation(r, context.Background(), telemetry.ConfirmationEvent{Result: telemetry.ResultReplay})
		}, []telemetry.MetricName{telemetry.MetricConfirmation, telemetry.MetricConfirmationDuration}},
		{"resource conflict retry", func(r telemetry.Recorder) {
			telemetry.RecordConfirmation(r, context.Background(), telemetry.ConfirmationEvent{Result: telemetry.ResultResourceConflict, RetryCount: 1})
		}, []telemetry.MetricName{telemetry.MetricConfirmation, telemetry.MetricConfirmationDuration, telemetry.MetricAllocationRetries, telemetry.MetricConflicts}},
		{"idempotency conflict", func(r telemetry.Recorder) {
			telemetry.RecordConfirmation(r, context.Background(), telemetry.ConfirmationEvent{Result: telemetry.ResultIdempotencyConflict})
		}, []telemetry.MetricName{telemetry.MetricConfirmation, telemetry.MetricConfirmationDuration, telemetry.MetricConflicts}},
		{"database cancellation", func(r telemetry.Recorder) {
			telemetry.RecordDatabase(r, context.Background(), telemetry.DatabaseEvent{Result: telemetry.ResultCanceled})
		}, []telemetry.MetricName{telemetry.MetricDatabaseDuration, telemetry.MetricDatabaseErrors}},
		{"database error", func(r telemetry.Recorder) {
			telemetry.RecordDatabase(r, context.Background(), telemetry.DatabaseEvent{Result: telemetry.ResultDatabaseError})
		}, []telemetry.MetricName{telemetry.MetricDatabaseDuration, telemetry.MetricDatabaseErrors}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &memoryMetricRecorder{}
			tt.record(recorder)
			if len(recorder.events) != len(tt.want) {
				t.Fatalf("events = %#v", recorder.events)
			}
			for i, name := range tt.want {
				if recorder.events[i].Name != name {
					t.Errorf("event %d = %q, want %q", i, recorder.events[i].Name, name)
				}
			}
		})
	}
}
