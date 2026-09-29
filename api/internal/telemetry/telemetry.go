// Package telemetry defines a deliberately narrow, privacy-safe operational
// event boundary. Event types cannot carry request bodies, customer details,
// vehicle identifiers, idempotency keys, commands, or raw errors.
package telemetry

import (
	"context"
	"time"
)

type Method string
type Route string
type Result string
type DatabaseOperation string
type MetricName string
type MetricUnit string
type ConflictClass string

const (
	RouteLive            Route = "/api/v1/health/live"
	RouteReady           Route = "/api/v1/health/ready"
	RouteBookingOptions  Route = "/api/v1/booking-options"
	RouteAvailability    Route = "/api/v1/availability"
	RouteAppointments    Route = "/api/v1/appointments"
	RouteAppointmentByID Route = "/api/v1/appointments/{appointmentId}"
	RouteUnknown         Route = "unknown"
)

const (
	ResultSuccess             Result = "success"
	ResultCreated             Result = "created"
	ResultReplay              Result = "replay"
	ResultValidation          Result = "validation"
	ResultNotFound            Result = "not_found"
	ResultResourceConflict    Result = "resource_conflict"
	ResultIdempotencyConflict Result = "idempotency_conflict"
	ResultDatabaseError       Result = "database_error"
	ResultInternal            Result = "internal"
	ResultCanceled            Result = "canceled"
	ResultMethodNotAllowed    Result = "method_not_allowed"
)

const (
	DatabaseBookingOptions  DatabaseOperation = "booking_options"
	DatabaseAvailability    DatabaseOperation = "availability"
	DatabaseConfirm         DatabaseOperation = "confirm"
	DatabaseAppointmentByID DatabaseOperation = "appointment_by_id"
)

type RequestEvent struct {
	Method   Method
	Route    Route
	Status   int
	Result   Result
	Duration time.Duration
}
type AvailabilityEvent struct {
	DealershipID  string
	ServiceTypeID string
	Result        Result
	Count         int
}
type ConfirmationEvent struct {
	DealershipID  string
	ServiceTypeID string
	Result        Result
	RetryCount    int
	Duration      time.Duration
}
type DatabaseEvent struct {
	Operation     DatabaseOperation
	Result        Result
	DealershipID  string
	ServiceTypeID string
	RetryCount    int
	Duration      time.Duration
}

const (
	MetricHTTPDuration         MetricName    = "http.server.duration"
	MetricHTTPErrors           MetricName    = "http.server.errors"
	MetricAvailabilityCount    MetricName    = "availability.result_count"
	MetricConfirmation         MetricName    = "confirmation.attempts"
	MetricConfirmationDuration MetricName    = "confirmation.duration"
	MetricConflicts            MetricName    = "booking.conflicts"
	MetricAllocationRetries    MetricName    = "allocation.retries"
	MetricDatabaseDuration     MetricName    = "database.duration"
	MetricDatabaseErrors       MetricName    = "database.errors"
	UnitMilliseconds           MetricUnit    = "ms"
	UnitOne                    MetricUnit    = "1"
	ConflictResource           ConflictClass = "resource"
	ConflictIdempotency        ConflictClass = "idempotency"
)

// MetricEvent has a closed set of typed dimensions. It intentionally avoids a
// general attribute map so adapters cannot accidentally receive private data.
type MetricEvent struct {
	Name          MetricName
	Unit          MetricUnit
	Value         float64
	Method        Method
	Route         Route
	Status        int
	Result        Result
	Operation     DatabaseOperation
	DealershipID  string
	ServiceTypeID string
	ConflictClass ConflictClass
	RetryCount    int
}

type Recorder interface {
	RequestCompleted(context.Context, RequestEvent)
	Availability(context.Context, AvailabilityEvent)
	Confirmation(context.Context, ConfirmationEvent)
	Database(context.Context, DatabaseEvent)
	Metric(context.Context, MetricEvent)
}

type Correlation struct {
	RequestID string
	TraceID   string
	SpanID    string
}
type correlationKey struct{}

func WithCorrelation(ctx context.Context, requestID, traceID string) context.Context {
	current := CorrelationFromContext(ctx)
	current.RequestID, current.TraceID = bound(requestID), bound(traceID)
	return context.WithValue(ctx, correlationKey{}, current)
}
func CorrelationFromContext(ctx context.Context) Correlation {
	value, _ := ctx.Value(correlationKey{}).(Correlation)
	value.RequestID = bound(value.RequestID)
	value.TraceID = bound(value.TraceID)
	value.SpanID = bound(value.SpanID)
	return value
}

// Clock is injected at timed boundaries to make duration metrics deterministic.
type Clock interface{ Now() time.Time }
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }
func SystemClock() Clock           { return systemClock{} }

func RecordRequest(r Recorder, ctx context.Context, event RequestEvent) {
	r = Safe(r)
	r.RequestCompleted(ctx, event)
	r.Metric(ctx, MetricEvent{Name: MetricHTTPDuration, Unit: UnitMilliseconds, Value: float64(nonnegativeDuration(event.Duration).Microseconds()) / 1000, Method: event.Method, Route: event.Route, Status: event.Status, Result: event.Result})
	if event.Status >= 400 {
		r.Metric(ctx, MetricEvent{Name: MetricHTTPErrors, Unit: UnitOne, Value: 1, Method: event.Method, Route: event.Route, Status: event.Status, Result: event.Result})
	}
}
func RecordAvailability(r Recorder, ctx context.Context, event AvailabilityEvent) {
	r = Safe(r)
	r.Availability(ctx, event)
	r.Metric(ctx, MetricEvent{Name: MetricAvailabilityCount, Unit: UnitOne, Value: float64(nonnegative(event.Count)), Result: event.Result, DealershipID: event.DealershipID, ServiceTypeID: event.ServiceTypeID})
}
func RecordConfirmation(r Recorder, ctx context.Context, event ConfirmationEvent) {
	r = Safe(r)
	r.Confirmation(ctx, event)
	r.Metric(ctx, MetricEvent{Name: MetricConfirmation, Unit: UnitOne, Value: 1, Result: event.Result, DealershipID: event.DealershipID, ServiceTypeID: event.ServiceTypeID, RetryCount: nonnegative(event.RetryCount)})
	r.Metric(ctx, MetricEvent{Name: MetricConfirmationDuration, Unit: UnitMilliseconds, Value: float64(nonnegativeDuration(event.Duration).Microseconds()) / 1000, Result: event.Result, DealershipID: event.DealershipID, ServiceTypeID: event.ServiceTypeID, RetryCount: nonnegative(event.RetryCount)})
	if event.RetryCount > 0 {
		r.Metric(ctx, MetricEvent{Name: MetricAllocationRetries, Unit: UnitOne, Value: float64(event.RetryCount), Result: event.Result, DealershipID: event.DealershipID, ServiceTypeID: event.ServiceTypeID, RetryCount: event.RetryCount})
	}
	if event.Result == ResultResourceConflict || event.Result == ResultIdempotencyConflict {
		class := ConflictResource
		if event.Result == ResultIdempotencyConflict {
			class = ConflictIdempotency
		}
		r.Metric(ctx, MetricEvent{Name: MetricConflicts, Unit: UnitOne, Value: 1, Result: event.Result, DealershipID: event.DealershipID, ServiceTypeID: event.ServiceTypeID, ConflictClass: class})
	}
}
func RecordDatabase(r Recorder, ctx context.Context, event DatabaseEvent) {
	r = Safe(r)
	r.Database(ctx, event)
	r.Metric(ctx, MetricEvent{Name: MetricDatabaseDuration, Unit: UnitMilliseconds, Value: float64(nonnegativeDuration(event.Duration).Microseconds()) / 1000, Operation: event.Operation, Result: event.Result, DealershipID: event.DealershipID, ServiceTypeID: event.ServiceTypeID, RetryCount: event.RetryCount})
	if event.Result == ResultDatabaseError || event.Result == ResultCanceled || event.Result == ResultInternal {
		r.Metric(ctx, MetricEvent{Name: MetricDatabaseErrors, Unit: UnitOne, Value: 1, Operation: event.Operation, Result: event.Result})
	}
}

type nopRecorder struct{}

func Nop() Recorder                                                 { return nopRecorder{} }
func (nopRecorder) RequestCompleted(context.Context, RequestEvent)  {}
func (nopRecorder) Availability(context.Context, AvailabilityEvent) {}
func (nopRecorder) Confirmation(context.Context, ConfirmationEvent) {}
func (nopRecorder) Database(context.Context, DatabaseEvent)         {}
func (nopRecorder) Metric(context.Context, MetricEvent)             {}

// Safe makes telemetry best-effort. A faulty adapter cannot alter request or
// database behavior.
func Safe(recorder Recorder) Recorder {
	if recorder == nil {
		recorder = Nop()
	}
	return safeRecorder{next: recorder}
}

type safeRecorder struct{ next Recorder }

func swallow(call func()) { defer func() { _ = recover() }(); call() }
func (r safeRecorder) RequestCompleted(ctx context.Context, event RequestEvent) {
	swallow(func() { r.next.RequestCompleted(ctx, event) })
}
func (r safeRecorder) Availability(ctx context.Context, event AvailabilityEvent) {
	swallow(func() { r.next.Availability(ctx, event) })
}
func (r safeRecorder) Confirmation(ctx context.Context, event ConfirmationEvent) {
	swallow(func() { r.next.Confirmation(ctx, event) })
}
func (r safeRecorder) Database(ctx context.Context, event DatabaseEvent) {
	swallow(func() { r.next.Database(ctx, event) })
}
func (r safeRecorder) Metric(ctx context.Context, event MetricEvent) {
	swallow(func() { r.next.Metric(ctx, event) })
}

func bound(value string) string {
	if len(value) > 128 {
		value = value[:128]
	}
	result := make([]byte, 0, len(value))
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.' {
			result = append(result, ch)
		}
	}
	return string(result)
}
