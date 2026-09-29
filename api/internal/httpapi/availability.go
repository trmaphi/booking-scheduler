package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"scheduler/api/internal/application"
	"scheduler/api/internal/domain"
	"scheduler/api/internal/telemetry"
)

type availableSlotsFunc func(context.Context, application.AvailabilityQuery) ([]domain.Interval, error)

func availability(load availableSlotsFunc, recorder telemetry.Recorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		event := telemetry.AvailabilityEvent{Result: telemetry.ResultValidation}
		defer func() { telemetry.RecordAvailability(recorder, r.Context(), event) }()
		query, field, reason := parseAvailabilityQuery(r.URL.Query())
		if field != "" {
			writeApplicationError(w, r, &application.ValidationError{Field: field, Reason: reason})
			return
		}
		if load == nil {
			event.Result = telemetry.ResultDatabaseError
			writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred", nil)
			return
		}
		slots, err := load(r.Context(), query)
		if err != nil {
			event.Result = telemetryResult(err)
			writeApplicationError(w, r, err)
			return
		}
		response := availabilityResponse{Slots: make([]availabilitySlotResponse, 0, len(slots))}
		for _, slot := range slots {
			if slot == nil {
				event.Result = telemetry.ResultDatabaseError
				writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred", nil)
				return
			}
			response.Slots = append(response.Slots, availabilitySlotResponse{StartAt: slot.Start().UTC().Format(time.RFC3339), EndAt: slot.End().UTC().Format(time.RFC3339)})
		}
		event.DealershipID = query.DealershipID
		event.ServiceTypeID = query.ServiceTypeID
		event.Result = telemetry.ResultSuccess
		event.Count = len(slots)
		writeJSON(w, http.StatusOK, response)
	}
}

func parseAvailabilityQuery(values url.Values) (application.AvailabilityQuery, string, string) {
	allowed := map[string]bool{"vehicleId": true, "dealershipId": true, "serviceTypeId": true, "date": true}
	for key := range values {
		if !allowed[key] {
			return application.AvailabilityQuery{}, key, "is not allowed"
		}
	}
	fields := []string{"vehicleId", "dealershipId", "serviceTypeId", "date"}
	parsed := make(map[string]string, len(fields))
	for _, field := range fields {
		items, exists := values[field]
		if !exists || len(items) == 0 || items[0] == "" {
			return application.AvailabilityQuery{}, field, "is required"
		}
		if len(items) != 1 {
			return application.AvailabilityQuery{}, field, "must be provided exactly once"
		}
		parsed[field] = items[0]
	}
	for _, field := range []string{"vehicleId", "dealershipId", "serviceTypeId"} {
		if !validUUID(parsed[field]) {
			return application.AvailabilityQuery{}, field, "must be a valid UUID"
		}
	}
	date, err := time.Parse("2006-01-02", parsed["date"])
	if err != nil || date.Format("2006-01-02") != parsed["date"] {
		return application.AvailabilityQuery{}, "date", "must be a valid YYYY-MM-DD calendar date"
	}
	return application.AvailabilityQuery{VehicleID: parsed["vehicleId"], DealershipID: parsed["dealershipId"], ServiceTypeID: parsed["serviceTypeId"], Date: parsed["date"]}, "", ""
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}
