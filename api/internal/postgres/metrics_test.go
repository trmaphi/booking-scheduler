package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"scheduler/api/internal/application"
	"scheduler/api/internal/telemetry"
)

type databaseMetricRecorder struct {
	databaseEventRecorder
	metrics      []telemetry.MetricEvent
	correlations []telemetry.Correlation
}

func TestMetricAllocationRetryCountComesFromDatabaseAttempts(t *testing.T) {
	t.Run("successful alternative pair", func(t *testing.T) {
		fixture := newBookingFixture(t)
		recorder := &databaseMetricRecorder{}
		fixture.gateway = NewRepository(fixture.pool, recorder)
		secondTechnician := fixture.addQualifiedTechnician(t, randomUUID(t))
		rejectedTechnician, expectedTechnician := fixture.qualifiedTechnicianID, secondTechnician
		if secondTechnician < fixture.qualifiedTechnicianID {
			rejectedTechnician, expectedTechnician = secondTechnician, fixture.qualifiedTechnicianID
		}
		installAllocationRejection(t, fixture, "new.technician_id = '"+rejectedTechnician+"'::uuid")
		result, err := fixture.gateway.Confirm(context.Background(), fixture.command())
		if err != nil {
			t.Fatal(err)
		}
		if result.TechnicianID != expectedTechnician || result.RetryCount != 1 {
			t.Fatalf("result = %#v", result)
		}
		if len(recorder.events) != 1 || recorder.events[0].RetryCount != 1 || recorder.events[0].Result != telemetry.ResultSuccess {
			t.Fatalf("database events = %#v", recorder.events)
		}
	})
	t.Run("exhausted candidates", func(t *testing.T) {
		fixture := newBookingFixture(t)
		recorder := &databaseMetricRecorder{}
		fixture.gateway = NewRepository(fixture.pool, recorder)
		fixture.addQualifiedTechnician(t, randomUUID(t))
		installAllocationRejection(t, fixture, "new.dealership_id = '"+fixture.dealershipID+"'::uuid")
		result, err := fixture.gateway.Confirm(context.Background(), fixture.command())
		if !errors.Is(err, application.ErrResourceConflict) || result.RetryCount != 2 {
			t.Fatalf("result/error = %#v / %v", result, err)
		}
		if len(recorder.events) != 1 || recorder.events[0].RetryCount != 2 || recorder.events[0].Result != telemetry.ResultResourceConflict {
			t.Fatalf("database events = %#v", recorder.events)
		}
	})
}

func installAllocationRejection(t *testing.T, fixture *bookingFixture, predicate string) {
	t.Helper()
	name := "reject_allocation_" + strings.ReplaceAll(randomUUID(t), "-", "")
	function := name + "_fn"
	sql := fmt.Sprintf(`create function %s() returns trigger language plpgsql as $$ begin if %s then raise exclusion_violation using constraint='appointments_technician_no_overlap'; end if; return new; end $$; create trigger %s before insert on appointments for each row execute function %s()`, function, predicate, name, function)
	if _, err := fixture.pool.Exec(context.Background(), sql); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = fixture.pool.Exec(context.Background(), fmt.Sprintf("drop trigger if exists %s on appointments; drop function if exists %s()", name, function))
	})
}

func (r *databaseMetricRecorder) Metric(ctx context.Context, event telemetry.MetricEvent) {
	r.metrics = append(r.metrics, event)
	r.correlations = append(r.correlations, telemetry.CorrelationFromContext(ctx))
}

type databaseFakeClock struct {
	values []time.Time
	index  int
}

func (c *databaseFakeClock) Now() time.Time {
	value := c.values[c.index]
	c.index++
	return value
}

func TestMetricDatabaseDurationErrorAndTraceUseFakeClock(t *testing.T) {
	recorder := &databaseMetricRecorder{}
	clock := &databaseFakeClock{values: []time.Time{time.Unix(20, 0), time.Unix(20, int64(75*time.Millisecond))}}
	repository := NewRepositoryWithClock(nil, recorder, clock)
	trace := telemetry.TraceContext{TraceID: "0123456789abcdef0123456789abcdef", SpanID: "1111111111111111"}
	ctx := telemetry.WithTrace(telemetry.WithCorrelation(context.Background(), "request", trace.TraceID), trace)
	_, _ = repository.BookingOptions(ctx)
	if len(recorder.metrics) != 2 || recorder.metrics[0].Name != telemetry.MetricDatabaseDuration || recorder.metrics[0].Value != 75 || recorder.metrics[1].Name != telemetry.MetricDatabaseErrors {
		t.Fatalf("metrics = %#v", recorder.metrics)
	}
	if recorder.correlations[0].TraceID != trace.TraceID || recorder.correlations[0].SpanID != trace.SpanID {
		t.Fatalf("correlation = %#v", recorder.correlations[0])
	}
}
