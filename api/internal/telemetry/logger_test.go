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

func TestTelemetryStructuredJSONContainsOnlyTypedSafeFields(t *testing.T) {
	var output bytes.Buffer
	recorder := telemetry.NewJSONRecorder(&output)
	ctx := telemetry.WithCorrelation(context.Background(), "request-123", "trace-456")

	recorder.RequestCompleted(ctx, telemetry.RequestEvent{Method: telemetry.Method("POST"), Route: telemetry.RouteAppointments, Status: 201, Result: telemetry.ResultCreated, Duration: 5 * time.Millisecond})
	recorder.Availability(ctx, telemetry.AvailabilityEvent{DealershipID: "11111111-1111-1111-1111-111111111111", ServiceTypeID: "22222222-2222-2222-2222-222222222222", Result: telemetry.ResultSuccess, Count: 3})
	recorder.Confirmation(ctx, telemetry.ConfirmationEvent{DealershipID: "11111111-1111-1111-1111-111111111111", ServiceTypeID: "22222222-2222-2222-2222-222222222222", Result: telemetry.ResultCreated, RetryCount: 0})
	recorder.Database(ctx, telemetry.DatabaseEvent{Operation: telemetry.DatabaseConfirm, Result: telemetry.ResultSuccess, DealershipID: "11111111-1111-1111-1111-111111111111", ServiceTypeID: "22222222-2222-2222-2222-222222222222"})

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d events, want 4: %s", len(lines), output.String())
	}
	wantNames := []string{"request.completed", "availability.completed", "confirmation.completed", "database.completed"}
	for i, line := range lines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("event %d is not JSON: %v", i, err)
		}
		if event["event"] != wantNames[i] {
			t.Errorf("event %d name = %v, want %s", i, event["event"], wantNames[i])
		}
		if event["request_id"] != "request-123" || event["trace_id"] != "trace-456" {
			t.Errorf("event %d correlation = (%v, %v)", i, event["request_id"], event["trace_id"])
		}
	}
}

func TestPrivacySentinelsNeverAppearInTelemetryKeysOrValues(t *testing.T) {
	sentinels := []string{"PRIVATE-CUSTOMER-NAME", "private@example.invalid", "PRIVATE-REGISTRATION", "33333333-3333-3333-3333-333333333333", "44444444-4444-4444-4444-444444444444", "PRIVATE-IDEMPOTENCY-KEY", `{"customer":"PRIVATE-RAW-BODY"}`, "postgresql://private:secret@database.invalid/scheduler", "PRIVATE-PGX-ERROR"}
	var output bytes.Buffer
	recorder := telemetry.NewJSONRecorder(&output)
	ctx := telemetry.WithCorrelation(context.Background(), "safe-request", "safe-trace")
	recorder.Confirmation(ctx, telemetry.ConfirmationEvent{DealershipID: "11111111-1111-1111-1111-111111111111", ServiceTypeID: "22222222-2222-2222-2222-222222222222", Result: telemetry.ResultDatabaseError})
	recorder.Database(ctx, telemetry.DatabaseEvent{Operation: telemetry.DatabaseConfirm, Result: telemetry.ResultDatabaseError})
	got := output.String()
	for _, sentinel := range sentinels {
		if strings.Contains(got, sentinel) {
			t.Errorf("telemetry leaked sentinel %q", sentinel)
		}
	}
}

func TestTelemetryBoundsAttackerControlledCorrelationAndSafeIDs(t *testing.T) {
	var output bytes.Buffer
	recorder := telemetry.NewJSONRecorder(&output)
	long := strings.Repeat("x", 500)
	ctx := telemetry.WithCorrelation(context.Background(), long+"\nsecret", long)
	recorder.Availability(ctx, telemetry.AvailabilityEvent{DealershipID: long, ServiceTypeID: long, Result: telemetry.ResultSuccess, Count: -8})
	var event map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &event); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"request_id", "trace_id", "dealership_id", "service_type_id"} {
		value, _ := event[key].(string)
		if len(value) > 128 {
			t.Errorf("%s length = %d, want <= 128", key, len(value))
		}
	}
	if event["count"] != float64(0) {
		t.Errorf("negative count was not normalized: %v", event["count"])
	}
}

func TestDatabaseResourceConflictUsesWarningLevelWithoutLeakingDetails(t *testing.T) {
	var output bytes.Buffer
	recorder := telemetry.NewJSONRecorder(&output)
	recorder.Database(context.Background(), telemetry.DatabaseEvent{Operation: telemetry.DatabaseConfirm, Result: telemetry.ResultResourceConflict, DealershipID: "11111111-1111-1111-1111-111111111111", ServiceTypeID: "22222222-2222-2222-2222-222222222222", RetryCount: 2})
	var event map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &event); err != nil {
		t.Fatal(err)
	}
	if event["level"] != "WARN" || event["result"] != "resource_conflict" || event["retry_count"] != float64(2) {
		t.Fatalf("event = %#v", event)
	}
	if strings.Contains(output.String(), "PRIVATE-PGX-DETAIL") {
		t.Fatalf("raw database detail leaked: %s", output.String())
	}
}

func TestDatabaseNonConflictUsesInfoLevel(t *testing.T) {
	var output bytes.Buffer
	telemetry.NewJSONRecorder(&output).Database(context.Background(), telemetry.DatabaseEvent{Operation: telemetry.DatabaseConfirm, Result: telemetry.ResultSuccess})
	var event map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &event); err != nil {
		t.Fatal(err)
	}
	if event["level"] != "INFO" {
		t.Fatalf("event = %#v", event)
	}
}
