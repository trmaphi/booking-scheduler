package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"scheduler/api/internal/application"

	"github.com/jackc/pgx/v5"
)

const bookingPauseAdvisoryKey int64 = 810245901

func TestConfirmDoesNotDeadlockWithParentThenMappingMutation(t *testing.T) {
	tests := []struct {
		name      string
		prepare   func(*testing.T, *bookingFixture)
		lockRow   func(context.Context, pgx.Tx, *bookingFixture) error
		mutateMap func(context.Context, pgx.Tx, *bookingFixture) error
		want      error
	}{
		{
			name: "technician then technician skill",
			lockRow: func(ctx context.Context, tx pgx.Tx, f *bookingFixture) error {
				_, err := tx.Exec(ctx, `update technicians set name=name where id=$1`, f.qualifiedTechnicianID)
				return err
			},
			mutateMap: func(ctx context.Context, tx pgx.Tx, f *bookingFixture) error {
				_, err := tx.Exec(ctx, `delete from technician_skills where technician_id=$1 and skill_id=$2`, f.qualifiedTechnicianID, f.secondSkillID)
				return err
			},
			want: application.ErrResourceConflict,
		},
		{
			name:    "service then required skill mapping",
			prepare: retainOnlySecondRequiredSkill,
			lockRow: func(ctx context.Context, tx pgx.Tx, f *bookingFixture) error {
				_, err := tx.Exec(ctx, `update service_types set description=description where id=$1`, f.serviceTypeID)
				return err
			},
			mutateMap: deleteSecondRequiredSkill,
			want:      application.ErrInvalidReference,
		},
		{
			name:    "skill then required skill mapping",
			prepare: retainOnlySecondRequiredSkill,
			lockRow: func(ctx context.Context, tx pgx.Tx, f *bookingFixture) error {
				_, err := tx.Exec(ctx, `update skills set name=name where id=$1`, f.secondSkillID)
				return err
			},
			mutateMap: deleteSecondRequiredSkill,
			want:      application.ErrInvalidReference,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newBookingFixture(t)
			if test.prepare != nil {
				test.prepare(t, fixture)
			}
			admin, err := fixture.pool.Begin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer admin.Rollback(context.Background())
			if err := test.lockRow(context.Background(), admin, fixture); err != nil {
				t.Fatal(err)
			}

			confirmation := make(chan error, 1)
			go func() {
				_, err := fixture.gateway.Confirm(context.Background(), fixture.command())
				confirmation <- err
			}()
			waitForConfirmationLockWait(t, fixture)

			if err := test.mutateMap(context.Background(), admin, fixture); err != nil {
				t.Fatalf("mapping mutation deadlocked or failed: %v", err)
			}
			if err := admin.Commit(context.Background()); err != nil {
				t.Fatalf("admin commit: %v", err)
			}
			if err := <-confirmation; !errors.Is(err, test.want) {
				t.Fatalf("confirmation error=%v, want %v", err, test.want)
			}
			fixture.assertAppointmentCount(t, 0)
		})
	}
}

func retainOnlySecondRequiredSkill(t *testing.T, fixture *bookingFixture) {
	t.Helper()
	if _, err := fixture.pool.Exec(context.Background(), `delete from service_type_required_skills where service_type_id=$1 and skill_id=$2`, fixture.serviceTypeID, fixture.firstSkillID); err != nil {
		t.Fatal(err)
	}
}

func deleteSecondRequiredSkill(ctx context.Context, tx pgx.Tx, fixture *bookingFixture) error {
	_, err := tx.Exec(ctx, `delete from service_type_required_skills where service_type_id=$1 and skill_id=$2`, fixture.serviceTypeID, fixture.secondSkillID)
	return err
}

func waitForConfirmationLockWait(t *testing.T, fixture *bookingFixture) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waiting bool
		err := fixture.pool.QueryRow(context.Background(), `
			select exists (
				select 1 from pg_stat_activity
				where query like '%from confirm_appointment_observed(%' and wait_event_type = 'Lock'
			)
		`).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("confirmation did not wait on the parent-row lock")
}

func TestConfirmKeepsEligibilityStableThroughCommit(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(context.Context, *bookingFixture) error
	}{
		{"customer deactivation", updateFixtureActive("customers", func(f *bookingFixture) string { return f.customerID })},
		{"vehicle deactivation", updateFixtureActive("vehicles", func(f *bookingFixture) string { return f.vehicleID })},
		{"dealership deactivation", updateFixtureActive("dealerships", func(f *bookingFixture) string { return f.dealershipID })},
		{"service deactivation", updateFixtureActive("service_types", func(f *bookingFixture) string { return f.serviceTypeID })},
		{"required skill deactivation", updateFixtureActive("skills", func(f *bookingFixture) string { return f.secondSkillID })},
		{"required skill removal", func(ctx context.Context, f *bookingFixture) error {
			_, err := f.pool.Exec(ctx, `delete from service_type_required_skills where service_type_id=$1 and skill_id=$2`, f.serviceTypeID, f.secondSkillID)
			return err
		}},
		{"technician deactivation", updateFixtureActive("technicians", func(f *bookingFixture) string { return f.qualifiedTechnicianID })},
		{"technician skill removal", func(ctx context.Context, f *bookingFixture) error {
			_, err := f.pool.Exec(ctx, `delete from technician_skills where technician_id=$1 and skill_id=$2`, f.qualifiedTechnicianID, f.secondSkillID)
			return err
		}},
		{"bay deactivation", updateFixtureActive("service_bays", func(f *bookingFixture) string { return f.bayID })},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newBookingFixture(t)
			installBookingPauseTrigger(t, fixture)
			lockConnection, err := fixture.pool.Acquire(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer lockConnection.Release()
			if _, err := lockConnection.Exec(context.Background(), `select pg_advisory_lock($1)`, bookingPauseAdvisoryKey); err != nil {
				t.Fatal(err)
			}

			confirmation := make(chan error, 1)
			go func() {
				_, err := fixture.gateway.Confirm(context.Background(), fixture.command())
				confirmation <- err
			}()
			waitForPausedConfirmation(t, fixture)

			mutation := make(chan error, 1)
			go func() { mutation <- test.mutate(context.Background(), fixture) }()
			var earlyMutation error
			mutationFinishedEarly := false
			select {
			case err := <-mutation:
				earlyMutation = err
				mutationFinishedEarly = true
			case <-time.After(150 * time.Millisecond):
			}

			if _, err := lockConnection.Exec(context.Background(), `select pg_advisory_unlock($1)`, bookingPauseAdvisoryKey); err != nil {
				t.Fatal(err)
			}
			confirmationErr := <-confirmation
			if mutationFinishedEarly {
				t.Fatalf("eligibility mutation completed before confirmation commit: %v (confirmation: %v)", earlyMutation, confirmationErr)
			}
			if confirmationErr != nil {
				t.Fatalf("confirmation: %v", confirmationErr)
			}
			if !mutationFinishedEarly {
				if err := <-mutation; err != nil {
					t.Fatalf("mutation: %v", err)
				}
			}
			fixture.assertAppointmentCount(t, 1)
		})
	}
}

func updateFixtureActive(table string, id func(*bookingFixture) string) func(context.Context, *bookingFixture) error {
	return func(ctx context.Context, fixture *bookingFixture) error {
		_, err := fixture.pool.Exec(ctx, fmt.Sprintf("update %s set active=false where id=$1", table), id(fixture))
		return err
	}
}

func installBookingPauseTrigger(t *testing.T, fixture *bookingFixture) {
	t.Helper()
	_, err := fixture.pool.Exec(context.Background(), fmt.Sprintf(`
		create or replace function booking_test_pause_insert() returns trigger language plpgsql as $fn$
		begin
			perform pg_advisory_xact_lock(%d);
			return new;
		end
		$fn$;
		drop trigger if exists booking_test_pause_insert on appointments;
		create trigger booking_test_pause_insert before insert on appointments
		for each row execute function booking_test_pause_insert()
	`, bookingPauseAdvisoryKey))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = fixture.pool.Exec(context.Background(), `drop trigger if exists booking_test_pause_insert on appointments`)
		_, _ = fixture.pool.Exec(context.Background(), `drop function if exists booking_test_pause_insert()`)
	})
}

func waitForPausedConfirmation(t *testing.T, fixture *bookingFixture) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var paused bool
		err := fixture.pool.QueryRow(context.Background(), `
			select exists (
				select 1 from pg_stat_activity
				where query like '%from confirm_appointment_observed(%'
				  and wait_event_type = 'Lock' and wait_event = 'advisory'
			)
		`).Scan(&paused)
		if err != nil {
			t.Fatal(err)
		}
		if paused {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("confirmation did not reach the paused insert")
}
