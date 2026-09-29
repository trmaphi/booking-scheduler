package application

import (
	"context"
	"time"
)

// ConfirmCommand contains only caller-controlled booking choices. Duration,
// ownership, and assigned resources are derived by the booking transaction.
type ConfirmCommand struct {
	VehicleID      string
	DealershipID   string
	ServiceTypeID  string
	StartAt        time.Time
	IdempotencyKey string
}

type Appointment struct {
	ID                     string
	CustomerID             string
	VehicleID              string
	DealershipID           string
	ServiceTypeID          string
	TechnicianID           string
	ServiceBayID           string
	Status                 string
	StartAt                time.Time
	EndAt                  time.Time
	CreatedAt              time.Time
	CustomerName           string
	VehicleLabel           string
	Registration           string
	DealershipName         string
	DealershipAddress      string
	DealershipTimeZone     string
	ServiceTypeName        string
	ServiceTypeDescription string
	ServiceDurationMinutes int
	TechnicianName         string
	ServiceBayName         string
}

// ConfirmationResult distinguishes a new allocation from an idempotent replay
// without requiring the transport layer to perform a racy preliminary read.
type ConfirmationResult struct {
	Appointment
	Replayed   bool
	RetryCount int
}

type VehicleOption struct {
	ID, CustomerID, CustomerName, Label, Registration string
}

type DealershipOption struct{ ID, Name, Address, TimeZone string }

type ServiceTypeOption struct {
	ID, Name, Description string
	DurationMinutes       int
}

type BookingOptions struct {
	Vehicles     []VehicleOption
	Dealerships  []DealershipOption
	ServiceTypes []ServiceTypeOption
}

type BookingGateway interface {
	Confirm(context.Context, ConfirmCommand) (ConfirmationResult, error)
}
