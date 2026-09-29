package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"scheduler/api/internal/application"
)

func TestIdempotencyFirstSuccessAndSequentialReplay(t *testing.T) {
	f := newBookingFixture(t)
	command := f.command()
	command.IdempotencyKey = "first-and-replay-" + randomUUID(t)
	first, err := f.gateway.Confirm(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.gateway.Confirm(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if first.Appointment != second.Appointment || first.Replayed || !second.Replayed {
		t.Fatalf("replay differs:\nfirst=%#v\nsecond=%#v", first, second)
	}
	f.assertAppointmentCount(t, 1)
}

func TestIdempotencyConcurrentIdenticalReplay(t *testing.T) {
	f := newBookingFixture(t)
	command := f.command()
	command.IdempotencyKey = "concurrent-replay-" + randomUUID(t)
	const workers = 12
	start := make(chan struct{})
	results := make(chan struct {
		id  string
		err error
	}, workers)
	var ready sync.WaitGroup
	ready.Add(workers)
	for range workers {
		go func() {
			ready.Done()
			<-start
			a, err := f.gateway.Confirm(context.Background(), command)
			results <- struct {
				id  string
				err error
			}{a.ID, err}
		}()
	}
	ready.Wait()
	close(start)
	var id string
	for range workers {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent replay: %v", result.err)
		}
		if id == "" {
			id = result.id
		}
		if result.id != id {
			t.Fatalf("got appointment %s, want %s", result.id, id)
		}
	}
	f.assertAppointmentCount(t, 1)
}

func TestIdempotencyRejectsChangedInput(t *testing.T) {
	mutations := map[string]func(*application.ConfirmCommand){
		"vehicle":    func(c *application.ConfirmCommand) { c.VehicleID = randomUUID(t) },
		"dealership": func(c *application.ConfirmCommand) { c.DealershipID = randomUUID(t) },
		"service":    func(c *application.ConfirmCommand) { c.ServiceTypeID = randomUUID(t) },
		"start":      func(c *application.ConfirmCommand) { c.StartAt = c.StartAt.Add(30 * time.Minute) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			f := newBookingFixture(t)
			command := f.command()
			command.IdempotencyKey = "changed-" + randomUUID(t)
			if _, err := f.gateway.Confirm(context.Background(), command); err != nil {
				t.Fatal(err)
			}
			changed := command
			mutate(&changed)
			_, err := f.gateway.Confirm(context.Background(), changed)
			if !errors.Is(err, application.ErrIdempotencyConflict) {
				t.Fatalf("error=%v, want idempotency conflict", err)
			}
			f.assertAppointmentCount(t, 1)
		})
	}
}

func TestIdempotencyFailedAllocationLeavesNoRecord(t *testing.T) {
	f := newBookingFixture(t)
	f.setActive(t, "service_bays", f.bayID, false)
	command := f.command()
	command.IdempotencyKey = "failed-" + randomUUID(t)
	_, err := f.gateway.Confirm(context.Background(), command)
	if !errors.Is(err, application.ErrResourceConflict) {
		t.Fatalf("error=%v", err)
	}
	var count int
	if err := f.pool.QueryRow(context.Background(), `select count(*) from idempotency_records where idempotency_key=$1`, command.IdempotencyKey).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("idempotency rows=%d want 0", count)
	}
	f.setActive(t, "service_bays", f.bayID, true)
	if _, err := f.gateway.Confirm(context.Background(), command); err != nil {
		t.Fatalf("key was not reusable after rollback: %v", err)
	}
	f.assertAppointmentCount(t, 1)
}
