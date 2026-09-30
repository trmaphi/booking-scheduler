package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"

	"scheduler/api/internal/application"
)

func TestConcurrentConfirmationAllowsAtMostOneWinner(t *testing.T) {
	fixture := newBookingFixture(t)
	const workers = 20
	start := make(chan struct{})
	results := make(chan error, workers)
	var ready sync.WaitGroup
	ready.Add(workers)
	for range workers {
		go func() {
			ready.Done()
			<-start
			_, err := fixture.gateway.Confirm(context.Background(), fixture.command())
			results <- err
		}()
	}
	ready.Wait()
	close(start)

	successes, conflicts := 0, 0
	for range workers {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, application.ErrResourceConflict):
			conflicts++
		default:
			t.Fatalf("unexpected or leaked persistence error: %v", err)
		}
	}
	if successes != 1 || conflicts != workers-1 {
		t.Fatalf("results = %d successes, %d conflicts; want 1 and %d", successes, conflicts, workers-1)
	}
	fixture.assertAppointmentCount(t, 1)
}

func TestConcurrentConfirmRetriesAlternativePairs(t *testing.T) {
	fixture := newBookingFixture(t)
	fixture.addQualifiedTechnician(t, randomUUID(t))
	fixture.addBay(t, randomUUID(t))
	otherVehicle := fixture.addVehicle(t)
	commands := []application.ConfirmCommand{fixture.command(), fixture.command()}
	commands[1].VehicleID = otherVehicle
	start := make(chan struct{})
	results := make(chan struct {
		appointment application.Appointment
		err         error
	}, 2)
	for _, command := range commands {
		go func(command application.ConfirmCommand) {
			<-start
			appointment, err := fixture.gateway.Confirm(context.Background(), command)
			results <- struct {
				appointment application.Appointment
				err         error
			}{appointment.Appointment, err}
		}(command)
	}
	close(start)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("alternative allocation errors: %v, %v", first.err, second.err)
	}
	if first.appointment.TechnicianID == second.appointment.TechnicianID || first.appointment.ServiceBayID == second.appointment.ServiceBayID {
		t.Fatalf("allocations overlap: %#v %#v", first.appointment, second.appointment)
	}
	fixture.assertAppointmentCount(t, 2)
}
