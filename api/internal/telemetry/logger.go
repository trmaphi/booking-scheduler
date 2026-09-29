package telemetry

import (
	"context"
	"io"
	"log/slog"
	"time"
)

type jsonRecorder struct{ logger *slog.Logger }

func NewJSONRecorder(output io.Writer) Recorder {
	return Safe(&jsonRecorder{logger: slog.New(slog.NewJSONHandler(output, nil))})
}

func (r *jsonRecorder) RequestCompleted(ctx context.Context, event RequestEvent) {
	correlation := CorrelationFromContext(ctx)
	attrs := append(common(correlation, "request.completed"),
		slog.String("method", normalizeMethod(event.Method)), slog.String("route", normalizeRoute(event.Route)),
		slog.Int("status", normalizeStatus(event.Status)), slog.String("result", normalizeResult(event.Result)),
		slog.Int64("duration_ms", nonnegativeDuration(event.Duration).Milliseconds()))
	r.logger.LogAttrs(ctx, slog.LevelInfo, "request completed", attrs...)
}

func (r *jsonRecorder) Availability(ctx context.Context, event AvailabilityEvent) {
	correlation := CorrelationFromContext(ctx)
	attrs := append(common(correlation, "availability.completed"),
		slog.String("dealership_id", safeUUID(event.DealershipID)), slog.String("service_type_id", safeUUID(event.ServiceTypeID)),
		slog.String("result", normalizeResult(event.Result)), slog.Int("count", nonnegative(event.Count)))
	r.logger.LogAttrs(ctx, slog.LevelInfo, "availability completed", attrs...)
}

func (r *jsonRecorder) Confirmation(ctx context.Context, event ConfirmationEvent) {
	correlation := CorrelationFromContext(ctx)
	attrs := append(common(correlation, "confirmation.completed"),
		slog.String("dealership_id", safeUUID(event.DealershipID)), slog.String("service_type_id", safeUUID(event.ServiceTypeID)),
		slog.String("result", normalizeResult(event.Result)), slog.Int("retry_count", nonnegative(event.RetryCount)), slog.Float64("duration_ms", float64(nonnegativeDuration(event.Duration).Microseconds())/1000))
	r.logger.LogAttrs(ctx, slog.LevelInfo, "confirmation completed", attrs...)
}

func (r *jsonRecorder) Database(ctx context.Context, event DatabaseEvent) {
	correlation := CorrelationFromContext(ctx)
	attrs := append(common(correlation, "database.completed"),
		slog.String("operation", normalizeDatabaseOperation(event.Operation)), slog.String("result", normalizeResult(event.Result)),
		slog.String("dealership_id", safeUUID(event.DealershipID)), slog.String("service_type_id", safeUUID(event.ServiceTypeID)),
		slog.Int("retry_count", nonnegative(event.RetryCount)), slog.Float64("duration_ms", float64(nonnegativeDuration(event.Duration).Microseconds())/1000))
	level := slog.LevelInfo
	if event.Result == ResultResourceConflict {
		level = slog.LevelWarn
	}
	r.logger.LogAttrs(ctx, level, "database completed", attrs...)
}

func (r *jsonRecorder) Metric(ctx context.Context, event MetricEvent) {
	c := CorrelationFromContext(ctx)
	attrs := append(common(c, "metric.recorded"),
		slog.String("name", normalizeMetricName(event.Name)), slog.String("unit", normalizeMetricUnit(event.Unit)), slog.Float64("value", nonnegativeFloat(event.Value)),
		slog.String("method", normalizeMethod(event.Method)), slog.String("route", normalizeRoute(event.Route)), slog.Int("status", metricStatus(event.Status)),
		slog.String("result", normalizeResult(event.Result)), slog.String("operation", normalizeDatabaseOperation(event.Operation)),
		slog.String("dealership_id", safeUUID(event.DealershipID)), slog.String("service_type_id", safeUUID(event.ServiceTypeID)),
		slog.String("conflict_class", normalizeConflictClass(event.ConflictClass)), slog.Int("retry_count", nonnegative(event.RetryCount)))
	r.logger.LogAttrs(ctx, slog.LevelInfo, "metric recorded", attrs...)
}

func common(c Correlation, event string) []slog.Attr {
	return []slog.Attr{slog.String("event", event), slog.String("request_id", bound(c.RequestID)), slog.String("trace_id", bound(c.TraceID)), slog.String("span_id", bound(c.SpanID))}
}
func normalizeMetricName(value MetricName) string {
	switch value {
	case MetricHTTPDuration, MetricHTTPErrors, MetricAvailabilityCount, MetricConfirmation, MetricConfirmationDuration, MetricConflicts, MetricAllocationRetries, MetricDatabaseDuration, MetricDatabaseErrors:
		return string(value)
	default:
		return "unknown"
	}
}
func normalizeMetricUnit(value MetricUnit) string {
	switch value {
	case UnitMilliseconds, UnitOne:
		return string(value)
	default:
		return string(UnitOne)
	}
}
func normalizeConflictClass(value ConflictClass) string {
	switch value {
	case ConflictResource, ConflictIdempotency:
		return string(value)
	default:
		return "none"
	}
}
func nonnegativeFloat(value float64) float64 {
	if value < 0 {
		return 0
	}
	return value
}
func metricStatus(value int) int {
	if value == 0 {
		return 0
	}
	return normalizeStatus(value)
}
func normalizeMethod(value Method) string {
	switch value {
	case "GET", "POST", "OPTIONS":
		return string(value)
	default:
		return "OTHER"
	}
}
func normalizeRoute(value Route) string {
	switch value {
	case RouteLive, RouteReady, RouteBookingOptions, RouteAvailability, RouteAppointments, RouteAppointmentByID, RouteUnknown:
		return string(value)
	default:
		return string(RouteUnknown)
	}
}
func normalizeResult(value Result) string {
	switch value {
	case ResultSuccess, ResultCreated, ResultReplay, ResultValidation, ResultNotFound, ResultResourceConflict, ResultIdempotencyConflict, ResultDatabaseError, ResultInternal, ResultCanceled, ResultMethodNotAllowed:
		return string(value)
	default:
		return string(ResultDatabaseError)
	}
}
func normalizeDatabaseOperation(value DatabaseOperation) string {
	switch value {
	case DatabaseBookingOptions, DatabaseAvailability, DatabaseConfirm, DatabaseAppointmentByID:
		return string(value)
	default:
		return "unknown"
	}
}
func normalizeStatus(value int) int {
	if value < 100 || value > 599 {
		return 500
	}
	return value
}
func nonnegative(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
func nonnegativeDuration(value time.Duration) time.Duration {
	if value < 0 {
		return 0
	}
	return value
}
func safeUUID(value string) string {
	if len(value) != 36 {
		return ""
	}
	for i, ch := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if ch != '-' {
				return ""
			}
			continue
		}
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
			return ""
		}
	}
	return value
}
