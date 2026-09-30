package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	constraintCustomerID     = "10000000-0000-0000-0000-000000000001"
	constraintVehicleID      = "10000000-0000-0000-0000-000000000002"
	constraintOtherVehicleID = "10000000-0000-0000-0000-000000000009"
	constraintDealershipID   = "10000000-0000-0000-0000-000000000003"
	constraintServiceTypeID  = "10000000-0000-0000-0000-000000000004"
	constraintTechnicianID   = "10000000-0000-0000-0000-000000000005"
	constraintOtherTechID    = "10000000-0000-0000-0000-000000000006"
	constraintBayID          = "10000000-0000-0000-0000-000000000007"
	constraintOtherBayID     = "10000000-0000-0000-0000-000000000008"
)

func TestAppointmentIntervalIsHalfOpen(t *testing.T) {
	tx := appointmentConstraintTx(t)
	start := time.Date(2030, 1, 2, 9, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	insertConstraintAppointment(t, tx, constraintTechnicianID, constraintBayID, "CONFIRMED", start, end)

	var lowerInclusive, upperInclusive bool
	err := tx.QueryRow(context.Background(), `
		select lower_inc(slot), upper_inc(slot)
		from (
			select tstzrange(start_at, end_at, '[)') as slot
			from appointments
			where technician_id = $1 and start_at = $2
		) ranges
	`, constraintTechnicianID, start).Scan(&lowerInclusive, &upperInclusive)
	if err != nil {
		t.Fatal(err)
	}
	if !lowerInclusive || upperInclusive {
		t.Fatalf("range bounds = lower inclusive %v, upper inclusive %v; want true, false", lowerInclusive, upperInclusive)
	}
}

func TestAppointmentOverlapRejectsTechnician(t *testing.T) {
	tx := appointmentConstraintTx(t)
	start := time.Date(2030, 1, 2, 9, 0, 0, 0, time.UTC)
	insertConstraintAppointment(t, tx, constraintTechnicianID, constraintBayID, "CONFIRMED", start, start.Add(time.Hour))

	err := insertConstraintAppointmentForVehicleError(tx, constraintOtherVehicleID, constraintTechnicianID, constraintOtherBayID, "CONFIRMED", start.Add(30*time.Minute), start.Add(90*time.Minute))
	assertExclusionViolation(t, err, "appointments_technician_no_overlap")
}

func TestAppointmentOverlapRejectsBay(t *testing.T) {
	tx := appointmentConstraintTx(t)
	start := time.Date(2030, 1, 2, 9, 0, 0, 0, time.UTC)
	insertConstraintAppointment(t, tx, constraintTechnicianID, constraintBayID, "CONFIRMED", start, start.Add(time.Hour))

	err := insertConstraintAppointmentForVehicleError(tx, constraintOtherVehicleID, constraintOtherTechID, constraintBayID, "CONFIRMED", start.Add(30*time.Minute), start.Add(90*time.Minute))
	assertExclusionViolation(t, err, "appointments_bay_no_overlap")
}

func TestAppointmentOverlapRejectsVehicle(t *testing.T) {
	tx := appointmentConstraintTx(t)
	start := time.Date(2030, 1, 2, 9, 0, 0, 0, time.UTC)
	insertConstraintAppointment(t, tx, constraintTechnicianID, constraintBayID, "CONFIRMED", start, start.Add(time.Hour))

	err := insertConstraintAppointmentError(tx, constraintOtherTechID, constraintOtherBayID, "CONFIRMED", start.Add(30*time.Minute), start.Add(90*time.Minute))
	assertExclusionViolation(t, err, "appointments_vehicle_no_overlap")
}

func TestAppointmentBoundaryTouchingSucceeds(t *testing.T) {
	tx := appointmentConstraintTx(t)
	start := time.Date(2030, 1, 2, 9, 0, 0, 0, time.UTC)
	insertConstraintAppointment(t, tx, constraintTechnicianID, constraintBayID, "CONFIRMED", start, start.Add(time.Hour))
	insertConstraintAppointment(t, tx, constraintTechnicianID, constraintBayID, "CONFIRMED", start.Add(time.Hour), start.Add(2*time.Hour))
}

func TestAppointmentCancelledOverlapSucceeds(t *testing.T) {
	tx := appointmentConstraintTx(t)
	start := time.Date(2030, 1, 2, 9, 0, 0, 0, time.UTC)
	insertConstraintAppointment(t, tx, constraintTechnicianID, constraintBayID, "CONFIRMED", start, start.Add(time.Hour))
	insertConstraintAppointment(t, tx, constraintTechnicianID, constraintBayID, "CANCELLED", start.Add(30*time.Minute), start.Add(90*time.Minute))
}

func appointmentConstraintTx(t *testing.T) pgx.Tx {
	t.Helper()
	pool := schemaPool(t)
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	statements := []string{
		`insert into customers (id, name, email) values ($1, 'Constraint Customer', 'constraint@example.invalid')`,
		`insert into dealerships (id, name, address, timezone) values ($1, 'Constraint Centre', '1 Test Way', 'UTC')`,
		`insert into vehicles (id, customer_id, label, registration) values ($1, $2, 'Test Vehicle', 'TEST-CONSTRAINT')`,
		`insert into vehicles (id, customer_id, label, registration) values ($1, $2, 'Other Test Vehicle', 'OTHER-CONSTRAINT')`,
		`insert into service_types (id, name, description, duration_minutes) values ($1, 'Constraint Service', 'Test only', 60)`,
		`insert into technicians (id, dealership_id, name) values ($1, $2, 'Constraint Technician')`,
		`insert into technicians (id, dealership_id, name) values ($1, $2, 'Other Constraint Technician')`,
		`insert into service_bays (id, dealership_id, name) values ($1, $2, 'Constraint Bay')`,
		`insert into service_bays (id, dealership_id, name) values ($1, $2, 'Other Constraint Bay')`,
	}
	args := [][]any{
		{constraintCustomerID},
		{constraintDealershipID},
		{constraintVehicleID, constraintCustomerID},
		{constraintOtherVehicleID, constraintCustomerID},
		{constraintServiceTypeID},
		{constraintTechnicianID, constraintDealershipID},
		{constraintOtherTechID, constraintDealershipID},
		{constraintBayID, constraintDealershipID},
		{constraintOtherBayID, constraintDealershipID},
	}
	for index, statement := range statements {
		if _, err := tx.Exec(context.Background(), statement, args[index]...); err != nil {
			t.Fatalf("insert constraint fixture %d: %v", index, err)
		}
	}
	return tx
}

func insertConstraintAppointment(t *testing.T, tx pgx.Tx, technicianID, bayID, status string, start, end time.Time) {
	t.Helper()
	if err := insertConstraintAppointmentError(tx, technicianID, bayID, status, start, end); err != nil {
		t.Fatal(err)
	}
}

func insertConstraintAppointmentError(tx pgx.Tx, technicianID, bayID, status string, start, end time.Time) error {
	return insertConstraintAppointmentForVehicleError(tx, constraintVehicleID, technicianID, bayID, status, start, end)
}

func insertConstraintAppointmentForVehicleError(tx pgx.Tx, vehicleID, technicianID, bayID, status string, start, end time.Time) error {
	_, err := tx.Exec(context.Background(), `
		insert into appointments (
			customer_id, vehicle_id, dealership_id, service_type_id,
			technician_id, service_bay_id, status, start_at, end_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, constraintCustomerID, vehicleID, constraintDealershipID, constraintServiceTypeID, technicianID, bayID, status, start, end)
	return err
}

func assertExclusionViolation(t *testing.T, err error, constraint string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected exclusion violation from %s", constraint)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("error type = %T, want *pgconn.PgError: %v", err, err)
	}
	if pgErr.Code != "23P01" || pgErr.ConstraintName != constraint {
		t.Fatalf("PostgreSQL error = code %s constraint %q, want 23P01 %q", pgErr.Code, pgErr.ConstraintName, constraint)
	}
}
