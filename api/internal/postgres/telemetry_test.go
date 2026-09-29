package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"scheduler/api/internal/application"
	"scheduler/api/internal/telemetry"

	"github.com/jackc/pgx/v5/pgxpool"
)

type databaseEventRecorder struct {
	mu     sync.Mutex
	events []telemetry.DatabaseEvent
}

func (*databaseEventRecorder) RequestCompleted(context.Context, telemetry.RequestEvent)  {}
func (*databaseEventRecorder) Availability(context.Context, telemetry.AvailabilityEvent) {}
func (*databaseEventRecorder) Confirmation(context.Context, telemetry.ConfirmationEvent) {}
func (r *databaseEventRecorder) Database(_ context.Context, event telemetry.DatabaseEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}
func (*databaseEventRecorder) Metric(context.Context, telemetry.MetricEvent) {}

func TestTelemetryDatabaseBoundariesEmitSafeClassifications(t *testing.T) {
	recorder := &databaseEventRecorder{}
	repository := NewRepository(nil, recorder)
	ctx := telemetry.WithCorrelation(context.Background(), "request-safe", "trace-safe")

	_, _ = repository.BookingOptions(ctx)
	_, _ = repository.LoadAvailabilityContext(ctx, application.AvailabilityQuery{DealershipID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", ServiceTypeID: "cccccccc-cccc-cccc-cccc-cccccccccccc"})
	_, _ = repository.Confirm(ctx, application.ConfirmCommand{DealershipID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", ServiceTypeID: "cccccccc-cccc-cccc-cccc-cccccccccccc"})
	_, _ = repository.AppointmentByID(ctx, "PRIVATE-VEHICLE-OR-CUSTOMER-ID")

	if len(recorder.events) != 4 {
		t.Fatalf("database events = %d, want 4", len(recorder.events))
	}
	wantOperations := []telemetry.DatabaseOperation{telemetry.DatabaseBookingOptions, telemetry.DatabaseAvailability, telemetry.DatabaseConfirm, telemetry.DatabaseAppointmentByID}
	for i, event := range recorder.events {
		if event.Operation != wantOperations[i] || event.Result != telemetry.ResultDatabaseError {
			t.Errorf("event %d = %#v", i, event)
		}
		if event.DealershipID != "" || event.ServiceTypeID != "" {
			t.Errorf("failed database operation retained untrusted IDs: %#v", event)
		}
	}
}

func TestTelemetryRecorderPanicDoesNotChangeDatabaseOutcome(t *testing.T) {
	repository := NewRepository(nil, &panicDatabaseRecorder{})
	_, err := repository.Confirm(context.Background(), application.ConfirmCommand{})
	if err != application.ErrPersistence {
		t.Fatalf("error = %v, want persistence error", err)
	}
}

type panicDatabaseRecorder struct{ databaseEventRecorder }

func (*panicDatabaseRecorder) Database(context.Context, telemetry.DatabaseEvent) {
	panic("telemetry unavailable")
}

func TestPrivacyActualJSONForRealDatabaseResourceConflict(t *testing.T) {
	fixture := newBookingFixture(t)
	const customerName = "PRIVATE-CUSTOMER-NAME-DB"
	const customerEmail = "private-db@example.invalid"
	const registration = "PRIVATE-REGISTRATION-DB"
	if _, err := fixture.pool.Exec(context.Background(), `update customers set name=$1,email=$2 where id=$3`, customerName, customerEmail, fixture.customerID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.pool.Exec(context.Background(), `update vehicles set registration=$1 where id=$2`, registration, fixture.vehicleID); err != nil {
		t.Fatal(err)
	}
	fixture.occupyTechnician(t)

	var output bytes.Buffer
	repository := NewRepository(fixture.pool, telemetry.NewJSONRecorder(&output))
	_, err := repository.Confirm(context.Background(), fixture.command())
	if !errors.Is(err, application.ErrResourceConflict) {
		t.Fatalf("error = %v, want resource conflict", err)
	}
	assertDatabaseJSONExcludes(t, output.String(), []string{customerName, customerEmail, registration, fixture.customerID, fixture.vehicleID})
	if !strings.Contains(output.String(), `"level":"WARN"`) || !strings.Contains(output.String(), `"result":"resource_conflict"`) {
		t.Fatalf("resource conflict was not a warning: %s", output.String())
	}
}

func TestPrivacyActualJSONForRealDatabaseOutage(t *testing.T) {
	const databaseURL = "postgresql://PRIVATE_DB_USER:PRIVATE_DB_PASSWORD@127.0.0.1:1/private?connect_timeout=1"
	const rawDatabaseError = "connection refused"
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var output bytes.Buffer
	repository := NewRepository(pool, telemetry.NewJSONRecorder(&output))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _ = repository.BookingOptions(ctx)
	assertDatabaseJSONExcludes(t, output.String(), []string{databaseURL, "PRIVATE_DB_USER", "PRIVATE_DB_PASSWORD", rawDatabaseError})
}

func assertDatabaseJSONExcludes(t *testing.T, output string, sentinels []string) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		t.Fatal("no JSON telemetry emitted")
	}
	for index, line := range lines {
		var value any
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			t.Fatalf("log %d is not JSON: %v", index, err)
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

func TestPrivacyActualJSONForRealSuccessfulBooking(t *testing.T) {
	fixture := newBookingFixture(t)
	const customerName = "PRIVATE-CUSTOMER-NAME-DB-SUCCESS"
	const customerEmail = "private-db-success@example.invalid"
	const registration = "PRIVATE-REGISTRATION-DB-SUCCESS"
	const idempotencyKey = "PRIVATE-IDEMPOTENCY-KEY-DB-SUCCESS"
	if _, err := fixture.pool.Exec(context.Background(), `update customers set name=$1,email=$2 where id=$3`, customerName, customerEmail, fixture.customerID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.pool.Exec(context.Background(), `update vehicles set registration=$1 where id=$2`, registration, fixture.vehicleID); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	repository := NewRepository(fixture.pool, telemetry.NewJSONRecorder(&output))
	command := fixture.command()
	command.IdempotencyKey = idempotencyKey
	result, err := repository.Confirm(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.CustomerName != customerName || result.Registration != registration {
		t.Fatalf("fixture sentinels did not traverse successful persistence result: %#v", result)
	}
	assertDatabaseJSONExcludes(t, output.String(), []string{customerName, customerEmail, registration, fixture.customerID, fixture.vehicleID, idempotencyKey})
	if !strings.Contains(output.String(), `"level":"INFO"`) {
		t.Fatalf("successful database event was not info: %s", output.String())
	}
}
