package postgres

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"scheduler/api/internal/application"
	"scheduler/api/internal/telemetry"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool      *pgxpool.Pool
	telemetry telemetry.Recorder
	clock     telemetry.Clock
}

// BookingGateway is retained as a source-compatible name for callers that only
// need confirmation; Repository also exposes the read side.
type BookingGateway = Repository

func NewRepository(pool *pgxpool.Pool, recorders ...telemetry.Recorder) *Repository {
	var recorder telemetry.Recorder
	if len(recorders) > 0 {
		recorder = recorders[0]
	}
	return &Repository{pool: pool, telemetry: telemetry.Safe(recorder), clock: telemetry.SystemClock()}
}

func NewRepositoryWithClock(pool *pgxpool.Pool, recorder telemetry.Recorder, clock telemetry.Clock) *Repository {
	if clock == nil {
		clock = telemetry.SystemClock()
	}
	return &Repository{pool: pool, telemetry: telemetry.Safe(recorder), clock: clock}
}

func NewBookingGateway(pool *pgxpool.Pool) *Repository { return NewRepository(pool) }

func (gateway *Repository) Confirm(ctx context.Context, command application.ConfirmCommand) (confirmation application.ConfirmationResult, returnedErr error) {
	event := telemetry.DatabaseEvent{Operation: telemetry.DatabaseConfirm, DealershipID: command.DealershipID, ServiceTypeID: command.ServiceTypeID}
	started := gateway.now()
	defer func() {
		event.Result = databaseResult(returnedErr)
		event.RetryCount = confirmation.RetryCount
		event.Duration = gateway.now().Sub(started)
		clearUntrustedDatabaseIDs(&event)
		telemetry.RecordDatabase(gateway.recorder(), ctx, event)
	}()
	if gateway == nil || gateway.pool == nil {
		return application.ConfirmationResult{}, application.ErrPersistence
	}
	if !validUUID(command.VehicleID) || !validUUID(command.DealershipID) || !validUUID(command.ServiceTypeID) || command.StartAt.IsZero() {
		return application.ConfirmationResult{}, application.ErrInvalidReference
	}

	if command.IdempotencyKey == "" {
		appointment, retryCount, err := gateway.confirmWith(ctx, gateway.pool, command)
		return application.ConfirmationResult{Appointment: appointment, RetryCount: retryCount}, err
	}
	if strings.TrimSpace(command.IdempotencyKey) == "" || len(command.IdempotencyKey) > 255 {
		return application.ConfirmationResult{}, application.ErrValidation
	}
	tx, err := gateway.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return application.ConfirmationResult{}, mapBookingError(ctx, err)
	}
	defer tx.Rollback(context.Background())
	hash := application.HashConfirmCommand(command)
	hashText := hex.EncodeToString(hash[:])
	result, err := tx.Exec(ctx, `
		insert into idempotency_records(idempotency_key, request_hash)
		values($1, $2) on conflict (idempotency_key) do nothing
	`, command.IdempotencyKey, hashText)
	if err != nil {
		return application.ConfirmationResult{}, mapBookingError(ctx, err)
	}
	if result.RowsAffected() == 0 {
		var storedHash string
		var appointmentID *string
		err = tx.QueryRow(ctx, `select request_hash, appointment_id::text from idempotency_records where idempotency_key=$1 for update`, command.IdempotencyKey).Scan(&storedHash, &appointmentID)
		if err != nil {
			return application.ConfirmationResult{}, mapBookingError(ctx, err)
		}
		if storedHash != hashText {
			return application.ConfirmationResult{}, application.ErrIdempotencyConflict
		}
		if appointmentID == nil {
			return application.ConfirmationResult{}, application.ErrPersistence
		}
		appointment, err := appointmentByID(ctx, tx, *appointmentID)
		if err != nil {
			return application.ConfirmationResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return application.ConfirmationResult{}, mapBookingError(ctx, err)
		}
		return application.ConfirmationResult{Appointment: appointment, Replayed: true}, nil
	}

	appointment, retryCount, err := gateway.confirmWith(ctx, tx, command)
	if err != nil {
		return application.ConfirmationResult{RetryCount: retryCount}, err
	}
	if _, err := tx.Exec(ctx, `update idempotency_records set appointment_id=$2 where idempotency_key=$1`, command.IdempotencyKey, appointment.ID); err != nil {
		return application.ConfirmationResult{}, mapBookingError(ctx, err)
	}
	appointment, err = appointmentByID(ctx, tx, appointment.ID)
	if err != nil {
		return application.ConfirmationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return application.ConfirmationResult{}, mapBookingError(ctx, err)
	}
	return application.ConfirmationResult{Appointment: appointment, RetryCount: retryCount}, nil
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (gateway *Repository) confirmWith(ctx context.Context, db queryRower, command application.ConfirmCommand) (application.Appointment, int, error) {
	var appointment application.Appointment
	var retryCount int
	err := db.QueryRow(ctx, `
		select (confirmed_appointment).id::text, (confirmed_appointment).customer_id::text,
		       (confirmed_appointment).vehicle_id::text, (confirmed_appointment).dealership_id::text,
		       (confirmed_appointment).service_type_id::text, (confirmed_appointment).technician_id::text,
		       (confirmed_appointment).service_bay_id::text, (confirmed_appointment).status,
		       (confirmed_appointment).start_at, (confirmed_appointment).end_at,
		       (confirmed_appointment).created_at, retry_count
		from confirm_appointment_observed($1::uuid, $2::uuid, $3::uuid, $4)
	`, command.VehicleID, command.DealershipID, command.ServiceTypeID, command.StartAt.UTC()).Scan(
		&appointment.ID, &appointment.CustomerID, &appointment.VehicleID,
		&appointment.DealershipID, &appointment.ServiceTypeID,
		&appointment.TechnicianID, &appointment.ServiceBayID, &appointment.Status,
		&appointment.StartAt, &appointment.EndAt, &appointment.CreatedAt, &retryCount,
	)
	if err == nil {
		appointment.StartAt = appointment.StartAt.UTC()
		appointment.EndAt = appointment.EndAt.UTC()
		appointment.CreatedAt = appointment.CreatedAt.UTC()
		return appointment, retryCount, nil
	}
	return application.Appointment{}, retryCountFromError(err), mapBookingError(ctx, err)
}

func retryCountFromError(err error) int {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "P0001" || pgErr.Detail != "RESOURCE_CONFLICT" {
		return 0
	}
	value, parseErr := strconv.Atoi(pgErr.Hint)
	if parseErr != nil || value < 0 || value > 1000 {
		return 0
	}
	return value
}

func mapBookingError(ctx context.Context, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		return errors.Join(ctx.Err(), err)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23P01" && pgErr.ConstraintName == "appointments_vehicle_no_overlap" {
		return application.ErrResourceConflict
	}
	if errors.As(err, &pgErr) && pgErr.Code == "P0001" {
		switch pgErr.Detail {
		case "INVALID_REFERENCE":
			return application.ErrInvalidReference
		case "RESOURCE_CONFLICT":
			return application.ErrResourceConflict
		case "INVALID_REQUEST":
			return application.ErrValidation
		}
	}
	return fmt.Errorf("%w", application.ErrPersistence)
}

func validUUID(raw string) bool {
	var value pgtype.UUID
	return value.Scan(raw) == nil && value.Valid
}

var _ application.BookingGateway = (*Repository)(nil)

func (r *Repository) recorder() telemetry.Recorder {
	if r == nil {
		return telemetry.Nop()
	}
	return telemetry.Safe(r.telemetry)
}

func (r *Repository) now() time.Time {
	if r == nil || r.clock == nil {
		return time.Now()
	}
	return r.clock.Now()
}

func databaseResult(err error) telemetry.Result {
	switch {
	case err == nil:
		return telemetry.ResultSuccess
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return telemetry.ResultCanceled
	case errors.Is(err, application.ErrResourceConflict):
		return telemetry.ResultResourceConflict
	case errors.Is(err, application.ErrIdempotencyConflict):
		return telemetry.ResultIdempotencyConflict
	case errors.Is(err, application.ErrInvalidReference), errors.Is(err, application.ErrValidation):
		return telemetry.ResultValidation
	case errors.Is(err, application.ErrNotFound):
		return telemetry.ResultNotFound
	default:
		return telemetry.ResultDatabaseError
	}
}

func clearUntrustedDatabaseIDs(event *telemetry.DatabaseEvent) {
	if event.Result != telemetry.ResultSuccess && event.Result != telemetry.ResultResourceConflict {
		event.DealershipID = ""
		event.ServiceTypeID = ""
	}
}
