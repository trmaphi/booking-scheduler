package httpapi

import (
	"context"
	"errors"
	"net/http"

	"scheduler/api/internal/application"
	"scheduler/api/internal/telemetry"
)

type apiError struct {
	Code      string           `json:"code"`
	Message   string           `json:"message"`
	RequestID string           `json:"requestId"`
	Details   *apiErrorDetails `json:"details,omitempty"`
}

type apiErrorDetails struct {
	Fields map[string]string `json:"fields"`
}

func writeApplicationError(w http.ResponseWriter, r *http.Request, err error) {
	var validation *application.ValidationError
	switch {
	case errors.As(err, &validation):
		writeAPIError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "request validation failed", map[string]string{validation.Field: validation.Reason})
	case errors.Is(err, application.ErrValidation), errors.Is(err, application.ErrInvalidReference):
		writeAPIError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "request validation failed", nil)
	case errors.Is(err, application.ErrResourceConflict):
		writeAPIError(w, r, http.StatusConflict, "RESOURCE_CONFLICT", "the requested appointment is no longer available", nil)
	case errors.Is(err, application.ErrIdempotencyConflict):
		writeAPIError(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key was already used for different input", nil)
	case errors.Is(err, application.ErrNotFound):
		writeAPIError(w, r, http.StatusNotFound, "NOT_FOUND", "appointment not found", nil)
	default:
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred", nil)
	}
}

func writeAPIError(w http.ResponseWriter, r *http.Request, status int, code, message string, fields map[string]string) {
	if observed, ok := w.(interface{ setTelemetryResult(telemetry.Result) }); ok {
		observed.setTelemetryResult(telemetryResultForCode(code))
	}
	requestID, _ := r.Context().Value(requestIDKey{}).(string)
	payload := apiError{Code: code, Message: message, RequestID: requestID}
	if len(fields) > 0 {
		payload.Details = &apiErrorDetails{Fields: fields}
	}
	writeJSON(w, status, payload)
}

func telemetryResult(err error) telemetry.Result {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return telemetry.ResultCanceled
	case errors.Is(err, application.ErrValidation), errors.Is(err, application.ErrInvalidReference):
		return telemetry.ResultValidation
	case errors.Is(err, application.ErrResourceConflict):
		return telemetry.ResultResourceConflict
	case errors.Is(err, application.ErrIdempotencyConflict):
		return telemetry.ResultIdempotencyConflict
	case errors.Is(err, application.ErrNotFound):
		return telemetry.ResultNotFound
	default:
		return telemetry.ResultDatabaseError
	}
}

func telemetryResultForCode(code string) telemetry.Result {
	switch code {
	case "VALIDATION_ERROR":
		return telemetry.ResultValidation
	case "RESOURCE_CONFLICT":
		return telemetry.ResultResourceConflict
	case "IDEMPOTENCY_CONFLICT":
		return telemetry.ResultIdempotencyConflict
	case "NOT_FOUND":
		return telemetry.ResultNotFound
	case "METHOD_NOT_ALLOWED":
		return telemetry.ResultMethodNotAllowed
	default:
		return telemetry.ResultDatabaseError
	}
}
