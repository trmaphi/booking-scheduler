package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"scheduler/api/internal/application"
	"scheduler/api/internal/domain"
	"scheduler/api/internal/httpapi"
)

func TestAvailabilityReturnsUTCSlotsWithoutResourceIdentifiers(t *testing.T) {
	start := time.Date(2027, 9, 29, 9, 0, 0, 0, time.FixedZone("BST", 3600))
	interval, err := domain.NewInterval(start, start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var gotQuery application.AvailabilityQuery
	router := httpapi.NewRouter(httpapi.Dependencies{AvailableSlots: func(_ context.Context, query application.AvailabilityQuery) ([]domain.Interval, error) {
		gotQuery = query
		return []domain.Interval{interval}, nil
	}})
	response := serve(router, http.MethodGet, availabilityURL(url.Values{"vehicleId": {testVehicleID}, "dealershipId": {testDealershipID}, "serviceTypeId": {testServiceTypeID}, "date": {"2027-09-29"}}), nil)
	if response.Code != 200 {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if gotQuery != (application.AvailabilityQuery{VehicleID: testVehicleID, DealershipID: testDealershipID, ServiceTypeID: testServiceTypeID, Date: "2027-09-29"}) {
		t.Fatalf("query = %#v", gotQuery)
	}
	var got map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"slots": []any{map[string]any{"startAt": "2027-09-29T08:00:00Z", "endAt": "2027-09-29T09:00:00Z"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("body = %#v, want %#v", got, want)
	}
	for _, prohibited := range []string{"technician", "bay"} {
		if strings.Contains(strings.ToLower(response.Body.String()), prohibited) {
			t.Fatalf("body exposed %s: %s", prohibited, response.Body.String())
		}
	}
}

func TestAvailabilityReturnsEmptySlotsAsArray(t *testing.T) {
	router := httpapi.NewRouter(httpapi.Dependencies{AvailableSlots: func(context.Context, application.AvailabilityQuery) ([]domain.Interval, error) { return nil, nil }})
	response := serve(router, http.MethodGet, availabilityURL(validAvailabilityValues()), nil)
	if response.Code != 200 || response.Body.String() != "{\"slots\":[]}\n" {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestAvailabilityRejectsMalformedQueryBeforeDependency(t *testing.T) {
	tests := []struct {
		name   string
		values url.Values
		field  string
	}{
		{name: "missing vehicle", values: url.Values{"dealershipId": {testDealershipID}, "serviceTypeId": {testServiceTypeID}, "date": {"2027-09-29"}}, field: "vehicleId"},
		{name: "duplicate vehicle", values: url.Values{"vehicleId": {testVehicleID, testVehicleID}, "dealershipId": {testDealershipID}, "serviceTypeId": {testServiceTypeID}, "date": {"2027-09-29"}}, field: "vehicleId"},
		{name: "malformed uuid", values: url.Values{"vehicleId": {"not-a-uuid"}, "dealershipId": {testDealershipID}, "serviceTypeId": {testServiceTypeID}, "date": {"2027-09-29"}}, field: "vehicleId"},
		{name: "invalid date", values: url.Values{"vehicleId": {testVehicleID}, "dealershipId": {testDealershipID}, "serviceTypeId": {testServiceTypeID}, "date": {"2027-02-29"}}, field: "date"},
		{name: "timestamp date rejected", values: url.Values{"vehicleId": {testVehicleID}, "dealershipId": {testDealershipID}, "serviceTypeId": {testServiceTypeID}, "date": {"2027-09-29T09:00:00"}}, field: "date"},
		{name: "unexpected start time without offset", values: url.Values{"vehicleId": {testVehicleID}, "dealershipId": {testDealershipID}, "serviceTypeId": {testServiceTypeID}, "date": {"2027-09-29"}, "startAt": {"2027-09-29T09:00:00"}}, field: "startAt"},
		{name: "unexpected misaligned start time", values: url.Values{"vehicleId": {testVehicleID}, "dealershipId": {testDealershipID}, "serviceTypeId": {testServiceTypeID}, "date": {"2027-09-29"}, "startAt": {"2027-09-29T09:15:00Z"}}, field: "startAt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			router := httpapi.NewRouter(httpapi.Dependencies{AvailableSlots: func(context.Context, application.AvailabilityQuery) ([]domain.Interval, error) {
				called = true
				return nil, nil
			}})
			response := serve(router, http.MethodGet, availabilityURL(tt.values), nil)
			if response.Code != 400 {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			if called {
				t.Fatal("dependency called for invalid query")
			}
			assertAPIError(t, response, "VALIDATION_ERROR", tt.field, true)
		})
	}
}

func TestAvailabilityMapsTypedAndSafeErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		status  int
		code    string
		field   string
		details bool
	}{
		{name: "typed validation", err: &application.ValidationError{Field: "date", Reason: "must be future"}, status: 400, code: "VALIDATION_ERROR", field: "date", details: true},
		{name: "unknown reference", err: &application.InvalidReferenceError{Reference: "vehicle"}, status: 400, code: "VALIDATION_ERROR"},
		{name: "sentinel invalid reference", err: application.ErrInvalidReference, status: 400, code: "VALIDATION_ERROR"},
		{name: "invalid repository context", err: &application.AvailabilityContextError{Reason: "corrupt timezone"}, status: 500, code: "INTERNAL_ERROR"},
		{name: "context canceled", err: context.Canceled, status: 500, code: "INTERNAL_ERROR"},
		{name: "database detail", err: errors.New("password=secret host=10.0.0.1"), status: 500, code: "INTERNAL_ERROR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := httpapi.NewRouter(httpapi.Dependencies{AvailableSlots: func(context.Context, application.AvailabilityQuery) ([]domain.Interval, error) { return nil, tt.err }})
			response := serve(router, http.MethodGet, availabilityURL(validAvailabilityValues()), nil)
			if response.Code != tt.status {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			assertAPIError(t, response, tt.code, tt.field, tt.details)
			if strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), "10.0.0.1") {
				t.Fatal("backend details leaked")
			}
		})
	}
}

func TestAvailabilityRejectsWrongMethodWithRequestID(t *testing.T) {
	router := httpapi.NewRouter(httpapi.Dependencies{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/availability", nil)
	req.Header.Set("X-Request-ID", "caller-availability")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != 405 {
		t.Fatalf("status = %d", response.Code)
	}
	assertAPIError(t, response, "METHOD_NOT_ALLOWED", "", false)
}

func validAvailabilityValues() url.Values {
	return url.Values{"vehicleId": {testVehicleID}, "dealershipId": {testDealershipID}, "serviceTypeId": {testServiceTypeID}, "date": {"2027-09-29"}}
}
func availabilityURL(values url.Values) string { return "/api/v1/availability?" + values.Encode() }

func assertAPIError(t *testing.T, response *httptest.ResponseRecorder, code, field string, wantDetails bool) {
	t.Helper()
	var body struct {
		Code, Message, RequestID string
		Details                  *struct {
			Fields map[string]string `json:"fields"`
		} `json:"details"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != code || body.Message == "" || body.RequestID == "" {
		t.Fatalf("error = %#v", body)
	}
	if (body.Details != nil) != wantDetails {
		t.Fatalf("details = %#v, want presence %v", body.Details, wantDetails)
	}
	if field != "" && (body.Details == nil || body.Details.Fields[field] == "") {
		t.Fatalf("field detail %q missing: %#v", field, body)
	}
}
