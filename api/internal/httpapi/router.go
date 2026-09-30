package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"scheduler/api/internal/application"
	"scheduler/api/internal/domain"
	"scheduler/api/internal/telemetry"
)

type Dependencies struct {
	Readiness          func(context.Context) error
	BookingOptions     func(context.Context) (application.BookingOptions, error)
	AvailableSlots     func(context.Context, application.AvailabilityQuery) ([]domain.Interval, error)
	ConfirmAppointment func(context.Context, application.ConfirmCommand) (application.ConfirmationResult, error)
	AppointmentByID    func(context.Context, string) (application.Appointment, error)
	AllowedOrigin      string
	Telemetry          telemetry.Recorder
	Clock              telemetry.Clock
}

type requestIDKey struct{}

func NewRouter(deps Dependencies) http.Handler {
	recorder := telemetry.Safe(deps.Telemetry)
	clock := deps.Clock
	if clock == nil {
		clock = telemetry.SystemClock()
	}
	mux := http.NewServeMux()
	mux.Handle("/docs", requireMethod(http.MethodGet, http.HandlerFunc(scalarReference)))
	mux.Handle("/openapi.yaml", requireMethod(http.MethodGet, http.HandlerFunc(openAPIContract)))
	mux.Handle("/api/docs", requireMethod(http.MethodGet, http.HandlerFunc(scalarReference)))
	mux.Handle("/api/openapi.yaml", requireMethod(http.MethodGet, http.HandlerFunc(openAPIContract)))
	mux.Handle("/api/v1/health/live", requireMethod(http.MethodGet, http.HandlerFunc(live)))
	mux.Handle("/api/v1/health/ready", requireMethod(http.MethodGet, ready(deps.Readiness)))
	mux.Handle("/api/v1/booking-options", requireMethod(http.MethodGet, bookingOptions(deps.BookingOptions)))
	mux.Handle("/api/v1/availability", requireMethod(http.MethodGet, availability(deps.AvailableSlots, recorder)))
	mux.Handle("/api/v1/appointments", requireMethod(http.MethodPost, confirmAppointment(deps.ConfirmAppointment, recorder, clock)))
	mux.Handle("/api/v1/appointments/", requireMethod(http.MethodGet, retrieveAppointment(deps.AppointmentByID)))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(w, r, http.StatusNotFound, "NOT_FOUND", "resource not found", nil)
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := clock.Now()
		requestID := r.Header.Get("X-Request-ID")
		if !validRequestID(requestID) {
			requestID = newRequestID()
		}
		observed := &observedResponseWriter{ResponseWriter: w}
		w = observed
		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
		trace := telemetry.StartTrace(r.Header.Values("traceparent"), nil)
		ctx = telemetry.WithCorrelation(ctx, requestID, trace.TraceID)
		ctx = telemetry.WithTrace(ctx, trace)
		r = r.WithContext(ctx)
		w.Header().Set("traceparent", trace.Header())
		defer func() {
			panicValue := recover()
			status := observed.status
			result := observed.result
			if panicValue != nil && status == 0 {
				status = http.StatusInternalServerError
				result = telemetry.ResultInternal
			} else if status == 0 {
				status = http.StatusOK
			}
			if result == "" {
				result = resultForStatus(status)
			}
			telemetry.RecordRequest(recorder, r.Context(), telemetry.RequestEvent{Method: telemetry.Method(r.Method), Route: routeTemplate(r.URL.Path), Status: status, Result: result, Duration: clock.Now().Sub(started)})
			if panicValue != nil {
				panic(panicValue)
			}
		}()

		w.Header().Add("Vary", "Origin")
		if deps.AllowedOrigin != "" && r.Header.Get("Origin") == deps.AllowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", deps.AllowedOrigin)
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key, X-Request-ID")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		if malformedAppointmentPath(r.URL.Path) {
			writeAPIError(w, r, http.StatusNotFound, "NOT_FOUND", "resource not found", nil)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func malformedAppointmentPath(path string) bool {
	const base = "/api/v1/appointments"
	if path != base && !strings.HasPrefix(path, base+"/") {
		return false
	}
	if strings.Contains(path, "//") {
		return true
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "." || segment == ".." {
			return true
		}
	}
	return false
}

func requireMethod(method string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Allow", method)
			writeAPIError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func validRequestID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, ch := range value {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-') {
			return false
		}
	}
	return true
}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic("unable to create request ID")
	}
	return hex.EncodeToString(value[:])
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

type observedResponseWriter struct {
	http.ResponseWriter
	status int
	result telemetry.Result
}

func (w *observedResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *observedResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}
func (w *observedResponseWriter) Unwrap() http.ResponseWriter                { return w.ResponseWriter }
func (w *observedResponseWriter) setTelemetryResult(result telemetry.Result) { w.result = result }

func routeTemplate(path string) telemetry.Route {
	switch path {
	case "/api/v1/health/live":
		return telemetry.RouteLive
	case "/api/v1/health/ready":
		return telemetry.RouteReady
	case "/api/v1/booking-options":
		return telemetry.RouteBookingOptions
	case "/api/v1/availability":
		return telemetry.RouteAvailability
	case "/api/v1/appointments":
		return telemetry.RouteAppointments
	default:
		if strings.HasPrefix(path, "/api/v1/appointments/") {
			return telemetry.RouteAppointmentByID
		}
		return telemetry.RouteUnknown
	}
}

func resultForStatus(status int) telemetry.Result {
	switch {
	case status == http.StatusCreated:
		return telemetry.ResultCreated
	case status >= 200 && status < 300:
		return telemetry.ResultSuccess
	case status == http.StatusBadRequest:
		return telemetry.ResultValidation
	case status == http.StatusNotFound:
		return telemetry.ResultNotFound
	case status == http.StatusConflict:
		return telemetry.ResultResourceConflict
	case status == http.StatusMethodNotAllowed:
		return telemetry.ResultMethodNotAllowed
	default:
		return telemetry.ResultDatabaseError
	}
}
