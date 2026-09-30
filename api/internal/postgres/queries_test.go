package postgres

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"scheduler/api/internal/application"
	"scheduler/api/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type queryCounter struct{ count atomic.Int64 }

func (counter *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	counter.count.Add(1)
	return ctx
}

func (*queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestBookingOptionsExcludeInactiveRows(t *testing.T) {
	f := newBookingFixture(t)
	inactiveVehicle := f.addInactiveVehicle(t)
	inactiveDealer := randomUUID(t)
	if _, err := f.pool.Exec(context.Background(), `insert into dealerships(id,name,address,timezone,active) values($1,'Inactive','Nowhere','UTC',false)`, inactiveDealer); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `delete from dealerships where id=$1`, inactiveDealer)
	})
	inactiveService := randomUUID(t)
	if _, err := f.pool.Exec(context.Background(), `insert into service_types(id,name,description,duration_minutes,active) values($1,$2,'Inactive',30,false)`, inactiveService, "Inactive "+inactiveService); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `delete from service_types where id=$1`, inactiveService)
	})
	options, err := NewRepository(f.pool).BookingOptions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range options.Vehicles {
		if v.ID == inactiveVehicle {
			t.Fatal("inactive vehicle returned")
		}
	}
	for _, d := range options.Dealerships {
		if d.ID == inactiveDealer {
			t.Fatal("inactive dealership returned")
		}
	}
	for _, s := range options.ServiceTypes {
		if s.ID == inactiveService {
			t.Fatal("inactive service returned")
		}
	}
	if !hasVehicle(options.Vehicles, f.vehicleID) || !hasDealer(options.Dealerships, f.dealershipID) || !hasService(options.ServiceTypes, f.serviceTypeID) {
		t.Fatal("active booking options missing")
	}
}

func TestLoadAvailabilityContextIsBoundedAndComplete(t *testing.T) {
	f := newBookingFixture(t)
	command := f.command()
	f.insertAppointment(t, f.qualifiedTechnicianID, f.bayID, command.StartAt, command.StartAt.Add(75*60*1e9))
	contextValue, err := NewRepository(f.pool).LoadAvailabilityContext(context.Background(), application.AvailabilityQuery{VehicleID: f.vehicleID, DealershipID: f.dealershipID, ServiceTypeID: f.serviceTypeID, Date: "2031-03-04"})
	if err != nil {
		t.Fatal(err)
	}
	if contextValue.TimeZone != "UTC" || len(contextValue.BusinessHours) != 1 || len(contextValue.RequiredSkills) != 2 || len(contextValue.Technicians) != 1 || len(contextValue.Bays) != 1 {
		t.Fatalf("incomplete context: %#v", contextValue)
	}
	if len(contextValue.TechnicianBusy[domain.TechnicianID(f.qualifiedTechnicianID)]) != 1 || len(contextValue.BayBusy[domain.BayID(f.bayID)]) != 1 {
		t.Fatal("bounded busy intervals missing")
	}
}

func TestLoadAvailabilityContextUsesConstantSetBasedReads(t *testing.T) {
	f := newBookingFixture(t)
	for range 8 {
		f.addQualifiedTechnician(t, randomUUID(t))
		f.addBay(t, randomUUID(t))
	}
	config, err := pgxpool.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	counter := &queryCounter{}
	config.ConnConfig.Tracer = counter
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, err = NewRepository(pool).LoadAvailabilityContext(context.Background(), application.AvailabilityQuery{VehicleID: f.vehicleID, DealershipID: f.dealershipID, ServiceTypeID: f.serviceTypeID, Date: "2031-03-04"})
	if err != nil {
		t.Fatal(err)
	}
	if got := counter.count.Load(); got != 6 {
		t.Fatalf("query count=%d want 6 fixed set-based reads", got)
	}
}

func TestAppointmentByIDSurvivesNewConnection(t *testing.T) {
	f := newBookingFixture(t)
	url := os.Getenv("TEST_DATABASE_URL")
	creator, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	command := f.command()
	command.IdempotencyKey = "durable-" + randomUUID(t)
	appointment, err := NewRepository(creator).Confirm(context.Background(), command)
	if err != nil {
		creator.Close()
		t.Fatal(err)
	}
	creator.Close()
	reader, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	loaded, err := NewRepository(reader).AppointmentByID(context.Background(), appointment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != appointment.ID || loaded.CustomerName == "" || loaded.VehicleLabel == "" || loaded.Registration == "" || loaded.DealershipName == "" || loaded.ServiceTypeName == "" || loaded.TechnicianName == "" || loaded.ServiceBayName == "" {
		t.Fatalf("incomplete persisted appointment: %#v", loaded)
	}
	_, err = NewRepository(reader).AppointmentByID(context.Background(), randomUUID(t))
	if !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("unknown error=%v", err)
	}
}

func TestAppointmentsFiltersStatusAndReturnsCompleteRows(t *testing.T) {
	f := newBookingFixture(t)
	start := time.Date(2031, 3, 4, 9, 30, 0, 0, time.UTC)
	f.insertAppointment(t, f.qualifiedTechnicianID, f.bayID, start, start.Add(75*time.Minute))
	if _, err := f.pool.Exec(context.Background(), `update appointments set status='CANCELLED' where vehicle_id=$1 and start_at=$2`, f.vehicleID, start); err != nil {
		t.Fatal(err)
	}
	appointments, err := NewRepository(f.pool).Appointments(context.Background(), "CANCELLED")
	if err != nil {
		t.Fatal(err)
	}
	for _, appointment := range appointments {
		if appointment.VehicleID == f.vehicleID {
			if appointment.Status != "CANCELLED" || appointment.VehicleLabel == "" || appointment.DealershipName == "" || appointment.ServiceTypeName == "" || appointment.TechnicianName == "" || appointment.ServiceBayName == "" {
				t.Fatalf("incomplete appointment: %#v", appointment)
			}
			return
		}
	}
	t.Fatal("cancelled appointment missing")
}

func hasVehicle(values []application.VehicleOption, id string) bool {
	for _, v := range values {
		if v.ID == id {
			return true
		}
	}
	return false
}
func hasDealer(values []application.DealershipOption, id string) bool {
	for _, v := range values {
		if v.ID == id {
			return true
		}
	}
	return false
}
func hasService(values []application.ServiceTypeOption, id string) bool {
	for _, v := range values {
		if v.ID == id {
			return true
		}
	}
	return false
}
