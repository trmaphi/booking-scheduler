package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"scheduler/api/internal/application"
	"scheduler/api/internal/domain"
	"scheduler/api/internal/telemetry"
)

const maxAppointmentBodyBytes int64 = 1 << 20

type confirmAppointmentFunc func(context.Context, application.ConfirmCommand) (application.ConfirmationResult, error)
type appointmentByIDFunc func(context.Context, string) (application.Appointment, error)
type appointmentsFunc func(context.Context, string) ([]application.Appointment, error)

func appointmentCollection(list appointmentsFunc, confirm confirmAppointmentFunc, recorder telemetry.Recorder, clock telemetry.Clock) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			status := r.URL.Query().Get("status")
			if status != "" && status != "CONFIRMED" && status != "CANCELLED" {
				writeApplicationError(w, r, &application.ValidationError{Field: "status", Reason: "must be CONFIRMED or CANCELLED"})
				return
			}
			if list == nil {
				writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred", nil)
				return
			}
			appointments, err := list(r.Context(), status)
			if err != nil {
				writeApplicationError(w, r, err)
				return
			}
			items := make([]appointmentResponse, len(appointments))
			for index, appointment := range appointments {
				items[index] = mapAppointment(appointment)
			}
			writeJSON(w, http.StatusOK, struct {
				Appointments []appointmentResponse `json:"appointments"`
			}{Appointments: items})
		case http.MethodPost:
			confirmAppointment(confirm, recorder, clock).ServeHTTP(w, r)
		default:
			w.Header().Set("Allow", "GET, POST")
			writeAPIError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
	})
}

func confirmAppointment(confirm confirmAppointmentFunc, recorder telemetry.Recorder, clock telemetry.Clock) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		started := clock.Now()
		event := telemetry.ConfirmationEvent{Result: telemetry.ResultValidation}
		defer func() {
			event.Duration = clock.Now().Sub(started)
			telemetry.RecordConfirmation(recorder, r.Context(), event)
		}()
		contentType, field, reason := singleHeader(r.Header, "Content-Type", "contentType")
		if field != "" {
			writeApplicationError(w, r, &application.ValidationError{Field: field, Reason: reason})
			return
		}
		if field, reason := validateJSONContentType(contentType); field != "" {
			writeApplicationError(w, r, &application.ValidationError{Field: field, Reason: reason})
			return
		}
		key, field := singleRequiredHeader(r.Header, "Idempotency-Key", "idempotencyKey")
		if field != "" {
			writeApplicationError(w, r, &application.ValidationError{Field: field, Reason: "must be provided exactly once"})
			return
		}
		if reason := validateIdempotencyKey(key); reason != "" {
			writeApplicationError(w, r, &application.ValidationError{Field: "idempotencyKey", Reason: reason})
			return
		}
		request, field, reason := decodeConfirmRequest(w, r)
		if field != "" {
			writeApplicationError(w, r, &application.ValidationError{Field: field, Reason: reason})
			return
		}
		start, err := domain.ParseOffsetDateTime(request.StartAt)
		if err != nil {
			writeApplicationError(w, r, &application.ValidationError{Field: "startAt", Reason: err.Error()})
			return
		}
		declaredStart, parseErr := time.Parse(time.RFC3339, request.StartAt)
		if parseErr != nil || !domain.IsGridAligned(declaredStart, 30) {
			writeApplicationError(w, r, &application.ValidationError{Field: "startAt", Reason: "must align to a 30-minute boundary"})
			return
		}
		if confirm == nil {
			event.Result = telemetry.ResultDatabaseError
			writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred", nil)
			return
		}
		result, err := confirm(r.Context(), application.ConfirmCommand{VehicleID: request.VehicleID, DealershipID: request.DealershipID, ServiceTypeID: request.ServiceTypeID, StartAt: start, IdempotencyKey: key})
		event.RetryCount = result.RetryCount
		if err != nil {
			event.Result = telemetryResult(err)
			if event.Result == telemetry.ResultResourceConflict || event.Result == telemetry.ResultIdempotencyConflict {
				event.DealershipID = request.DealershipID
				event.ServiceTypeID = request.ServiceTypeID
			}
			writeApplicationError(w, r, err)
			return
		}
		event.DealershipID = request.DealershipID
		event.ServiceTypeID = request.ServiceTypeID
		status := http.StatusCreated
		if result.Replayed {
			status = http.StatusOK
			event.Result = telemetry.ResultReplay
		} else {
			event.Result = telemetry.ResultCreated
		}
		writeJSON(w, status, mapAppointment(result.Appointment))
	}
}

func singleHeader(headers http.Header, name, field string) (string, string, string) {
	values := headers.Values(name)
	if len(values) != 1 || strings.Contains(values[0], ",") {
		return "", field, "must be provided exactly once"
	}
	return values[0], "", ""
}

func singleRequiredHeader(headers http.Header, name, field string) (string, string) {
	value, invalidField, _ := singleHeader(headers, name, field)
	if invalidField != "" || value == "" {
		return "", field
	}
	return value, ""
}

func retrieveAppointment(load appointmentByIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const prefix = "/api/v1/appointments/"
		id := strings.TrimPrefix(r.URL.Path, prefix)
		if id == r.URL.Path || id == "" || strings.Contains(id, "/") {
			writeAPIError(w, r, http.StatusNotFound, "NOT_FOUND", "resource not found", nil)
			return
		}
		if !validUUID(id) {
			writeApplicationError(w, r, &application.ValidationError{Field: "appointmentId", Reason: "must be a valid UUID"})
			return
		}
		if load == nil {
			writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred", nil)
			return
		}
		appointment, err := load(r.Context(), id)
		if err != nil {
			writeApplicationError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, mapAppointment(appointment))
	}
}

func validateJSONContentType(raw string) (string, string) {
	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil || mediaType != "application/json" {
		return "contentType", "must be application/json"
	}
	return "", ""
}

func validateIdempotencyKey(key string) string {
	if len(key) < 16 {
		return "must contain at least 16 characters"
	}
	if len(key) > 128 {
		return "must contain at most 128 characters"
	}
	for _, character := range key {
		if character < 0x21 || character > 0x7e {
			return "must contain visible ASCII characters without spaces"
		}
	}
	return ""
}

func decodeConfirmRequest(w http.ResponseWriter, r *http.Request) (confirmAppointmentRequest, string, string) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAppointmentBodyBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return confirmAppointmentRequest{}, "body", "must not exceed 1 MiB"
		}
		return confirmAppointmentRequest{}, "body", "must be readable"
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return confirmAppointmentRequest{}, "body", "is required"
	}
	if field, reason, err := validateConfirmJSONStructure(data); err != nil {
		return confirmAppointmentRequest{}, "body", "must contain exactly one valid JSON object"
	} else if field != "" {
		return confirmAppointmentRequest{}, field, reason
	}
	var request confirmAppointmentRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		if field := unknownJSONField(err); field != "" {
			return request, field, "is not allowed"
		}
		return request, "body", "must contain exactly one valid JSON object"
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return request, "body", "must contain exactly one JSON object"
	}
	for _, item := range []struct{ name, value string }{{"vehicleId", request.VehicleID}, {"dealershipId", request.DealershipID}, {"serviceTypeId", request.ServiceTypeID}} {
		if item.value == "" {
			return request, item.name, "is required"
		}
		if !validUUID(item.value) {
			return request, item.name, "must be a valid UUID"
		}
	}
	if request.StartAt == "" {
		return request, "startAt", "is required"
	}
	return request, "", ""
}

func validateConfirmJSONStructure(data []byte) (string, string, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return "", "", errors.New("request is not an object")
	}
	allowed := map[string]struct{}{"vehicleId": {}, "dealershipId": {}, "serviceTypeId": {}, "startAt": {}}
	seen := map[string]struct{}{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return "", "", err
		}
		key, ok := keyToken.(string)
		if !ok {
			return "", "", errors.New("object key is not a string")
		}
		if _, exists := seen[key]; exists {
			return key, "must be provided exactly once", nil
		}
		seen[key] = struct{}{}
		if err := scanJSONValue(decoder); err != nil {
			var duplicate duplicateFieldError
			if errors.As(err, &duplicate) {
				return string(duplicate), "must be provided exactly once", nil
			}
			return "", "", err
		}
		if _, exists := allowed[key]; !exists {
			return key, "is not allowed", nil
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return "", "", errors.New("invalid object")
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return "", "", err
	}
	return "", "", nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return duplicateFieldError(key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("invalid object")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("invalid array")
		}
	default:
		return errors.New("unexpected delimiter")
	}
	return nil
}

type duplicateFieldError string

func (e duplicateFieldError) Error() string { return "duplicate JSON field: " + string(e) }

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	}
	return errors.New("trailing JSON value")
}

func unknownJSONField(err error) string {
	const prefix = "json: unknown field \""
	message := err.Error()
	if strings.HasPrefix(message, prefix) && strings.HasSuffix(message, "\"") {
		return strings.TrimSuffix(strings.TrimPrefix(message, prefix), "\"")
	}
	return ""
}

func mapAppointment(a application.Appointment) appointmentResponse {
	return appointmentResponse{
		ID: a.ID, Status: a.Status,
		Vehicle:     vehicleResponse{ID: a.VehicleID, CustomerID: a.CustomerID, Label: a.VehicleLabel, Registration: a.Registration},
		Dealership:  dealershipResponse{ID: a.DealershipID, Name: a.DealershipName, Address: a.DealershipAddress, Timezone: a.DealershipTimeZone},
		ServiceType: serviceTypeResponse{ID: a.ServiceTypeID, Name: a.ServiceTypeName, Description: a.ServiceTypeDescription, DurationMinutes: a.ServiceDurationMinutes},
		Technician:  assignedResourceResponse{ID: a.TechnicianID, Name: a.TechnicianName}, ServiceBay: assignedResourceResponse{ID: a.ServiceBayID, Name: a.ServiceBayName},
		StartAt: a.StartAt.UTC().Format(time.RFC3339), EndAt: a.EndAt.UTC().Format(time.RFC3339),
	}
}
