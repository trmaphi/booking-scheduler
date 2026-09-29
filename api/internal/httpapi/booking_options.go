package httpapi

import (
	"context"
	"net/http"

	"scheduler/api/internal/application"
)

type bookingOptionsFunc func(context.Context) (application.BookingOptions, error)

func bookingOptions(load bookingOptionsFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if load == nil {
			writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred", nil)
			return
		}
		options, err := load(r.Context())
		if err != nil {
			writeApplicationError(w, r, err)
			return
		}
		response := bookingOptionsResponse{
			Vehicles:     make([]vehicleResponse, 0, len(options.Vehicles)),
			Dealerships:  make([]dealershipResponse, 0, len(options.Dealerships)),
			ServiceTypes: make([]serviceTypeResponse, 0, len(options.ServiceTypes)),
		}
		for _, value := range options.Vehicles {
			response.Vehicles = append(response.Vehicles, vehicleResponse{ID: value.ID, CustomerID: value.CustomerID, Label: value.Label, Registration: value.Registration})
		}
		for _, value := range options.Dealerships {
			response.Dealerships = append(response.Dealerships, dealershipResponse{ID: value.ID, Name: value.Name, Address: value.Address, Timezone: value.TimeZone})
		}
		for _, value := range options.ServiceTypes {
			response.ServiceTypes = append(response.ServiceTypes, serviceTypeResponse{ID: value.ID, Name: value.Name, Description: value.Description, DurationMinutes: value.DurationMinutes})
		}
		writeJSON(w, http.StatusOK, response)
	}
}
