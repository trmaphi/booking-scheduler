package openapi

import _ "embed"

// BookingAPI is the canonical OpenAPI contract served by the API.
//
//go:embed booking-api.yaml
var BookingAPI []byte
