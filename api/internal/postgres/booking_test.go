package postgres

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"testing"
	"time"

	"scheduler/api/internal/application"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestConfirmAllocatesAndDerivesAppointment(t *testing.T) {
	fixture := newBookingFixture(t)
	start := time.Date(2031, 3, 4, 9, 30, 0, 0, time.UTC)

	appointment, err := fixture.gateway.Confirm(context.Background(), application.ConfirmCommand{
		VehicleID: fixture.vehicleID, DealershipID: fixture.dealershipID,
		ServiceTypeID: fixture.serviceTypeID, StartAt: start,
	})
	if err != nil {
		t.Fatal(err)
	}
	if appointment.CustomerID != fixture.customerID || appointment.VehicleID != fixture.vehicleID {
		t.Fatalf("derived ownership = customer %q vehicle %q", appointment.CustomerID, appointment.VehicleID)
	}
	if appointment.StartAt != start || appointment.EndAt != start.Add(75*time.Minute) {
		t.Fatalf("derived interval = %s..%s", appointment.StartAt, appointment.EndAt)
	}
	if appointment.TechnicianID != fixture.qualifiedTechnicianID || appointment.ServiceBayID != fixture.bayID {
		t.Fatalf("allocation = technician %q bay %q", appointment.TechnicianID, appointment.ServiceBayID)
	}
	if appointment.Status != "CONFIRMED" || appointment.ID == "" {
		t.Fatalf("appointment = %#v", appointment)
	}
}

func TestConfirmRequiresEverySkill(t *testing.T) {
	fixture := newBookingFixture(t)
	fixture.removeTechnicianSkill(t, fixture.secondSkillID)

	_, err := fixture.gateway.Confirm(context.Background(), fixture.command())
	if !errors.Is(err, application.ErrResourceConflict) {
		t.Fatalf("error = %v, want resource conflict", err)
	}
	fixture.assertAppointmentCount(t, 0)
}

func TestConfirmOrdersByFutureLoadThenStableID(t *testing.T) {
	fixture := newBookingFixture(t)
	preferredTechnician := fixture.addQualifiedTechnician(t, "00000000-0000-4000-8000-000000000001")
	preferredBay := fixture.addBay(t, "00000000-0000-4000-8000-000000000002")
	fixture.addFutureLoad(t, fixture.qualifiedTechnicianID, fixture.bayID)

	appointment, err := fixture.gateway.Confirm(context.Background(), fixture.command())
	if err != nil {
		t.Fatal(err)
	}
	if appointment.TechnicianID != preferredTechnician || appointment.ServiceBayID != preferredBay {
		t.Fatalf("allocation = technician %q bay %q, want least-loaded %q %q", appointment.TechnicianID, appointment.ServiceBayID, preferredTechnician, preferredBay)
	}
}

func TestConfirmRejectsInvalidReferencesAndRollsBack(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *bookingFixture, *application.ConfirmCommand)
	}{
		{"vehicle owned by inactive customer", func(t *testing.T, f *bookingFixture, _ *application.ConfirmCommand) {
			f.setActive(t, "customers", f.customerID, false)
		}},
		{"inactive vehicle", func(t *testing.T, f *bookingFixture, _ *application.ConfirmCommand) {
			f.setActive(t, "vehicles", f.vehicleID, false)
		}},
		{"inactive dealership", func(t *testing.T, f *bookingFixture, _ *application.ConfirmCommand) {
			f.setActive(t, "dealerships", f.dealershipID, false)
		}},
		{"inactive service", func(t *testing.T, f *bookingFixture, _ *application.ConfirmCommand) {
			f.setActive(t, "service_types", f.serviceTypeID, false)
		}},
		{"unknown vehicle", func(t *testing.T, _ *bookingFixture, c *application.ConfirmCommand) { c.VehicleID = randomUUID(t) }},
		{"vehicle from another customer", func(t *testing.T, f *bookingFixture, c *application.ConfirmCommand) {
			c.VehicleID = f.addInactiveVehicle(t)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newBookingFixture(t)
			command := fixture.command()
			test.mutate(t, fixture, &command)
			_, err := fixture.gateway.Confirm(context.Background(), command)
			if !errors.Is(err, application.ErrInvalidReference) {
				t.Fatalf("error = %v, want invalid reference", err)
			}
			fixture.assertAppointmentCount(t, 0)
		})
	}
}

func TestConfirmRejectsInactiveOrOccupiedResources(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *bookingFixture) int
	}{
		{"inactive technician", func(t *testing.T, f *bookingFixture) int {
			f.setActive(t, "technicians", f.qualifiedTechnicianID, false)
			return 0
		}},
		{"inactive bay", func(t *testing.T, f *bookingFixture) int { f.setActive(t, "service_bays", f.bayID, false); return 0 }},
		{"occupied technician", func(t *testing.T, f *bookingFixture) int { f.occupyTechnician(t); return 1 }},
		{"occupied bay", func(t *testing.T, f *bookingFixture) int { f.occupyBay(t); return 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newBookingFixture(t)
			wantCount := test.mutate(t, fixture)
			_, err := fixture.gateway.Confirm(context.Background(), fixture.command())
			if !errors.Is(err, application.ErrResourceConflict) {
				t.Fatalf("error = %v, want resource conflict", err)
			}
			fixture.assertAppointmentCount(t, wantCount)
		})
	}
}

func TestConfirmRejectsOverlappingAppointmentForSameVehicle(t *testing.T) {
	fixture := newBookingFixture(t)
	fixture.addQualifiedTechnician(t, randomUUID(t))
	fixture.addBay(t, randomUUID(t))
	command := fixture.command()
	fixture.insertAppointment(t, fixture.qualifiedTechnicianID, fixture.bayID, command.StartAt, command.StartAt.Add(75*time.Minute))

	_, err := fixture.gateway.Confirm(context.Background(), command)
	if !errors.Is(err, application.ErrResourceConflict) {
		t.Fatalf("error = %v, want resource conflict", err)
	}
	fixture.assertAppointmentCount(t, 1)
}

func TestConfirmValidatesCommandWithoutDatabaseText(t *testing.T) {
	fixture := newBookingFixture(t)
	command := fixture.command()
	command.VehicleID = "not-a-uuid"
	_, err := fixture.gateway.Confirm(context.Background(), command)
	if !errors.Is(err, application.ErrInvalidReference) {
		t.Fatalf("error = %v, want invalid reference", err)
	}
	if err != nil && (contains(err.Error(), "invalid input syntax") || contains(err.Error(), "SQLSTATE")) {
		t.Fatalf("database detail leaked: %v", err)
	}
}

func TestConfirmRejectsInvalidServiceReferenceData(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *bookingFixture)
	}{
		{"no required skills", func(t *testing.T, f *bookingFixture) {
			if _, err := f.pool.Exec(context.Background(), `delete from service_type_required_skills where service_type_id=$1`, f.serviceTypeID); err != nil {
				t.Fatal(err)
			}
		}},
		{"inactive required skill", func(t *testing.T, f *bookingFixture) { f.setActive(t, "skills", f.secondSkillID, false) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newBookingFixture(t)
			test.mutate(t, fixture)
			_, err := fixture.gateway.Confirm(context.Background(), fixture.command())
			if !errors.Is(err, application.ErrInvalidReference) {
				t.Fatalf("error=%v, want invalid reference", err)
			}
			fixture.assertAppointmentCount(t, 0)
		})
	}
}

func TestConfirmRejectsStartOutsideBusinessRules(t *testing.T) {
	tests := []struct {
		name  string
		start time.Time
	}{
		{"before opening", time.Date(2031, 3, 4, 8, 30, 0, 0, time.UTC)},
		{"ends after closing", time.Date(2031, 3, 4, 16, 0, 0, 0, time.UTC)},
		{"off grid", time.Date(2031, 3, 4, 9, 15, 0, 0, time.UTC)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newBookingFixture(t)
			command := fixture.command()
			command.StartAt = test.start
			_, err := fixture.gateway.Confirm(context.Background(), command)
			if !errors.Is(err, application.ErrValidation) {
				t.Fatalf("error=%v, want validation", err)
			}
			fixture.assertAppointmentCount(t, 0)
		})
	}
}

type bookingFixture struct {
	t                                                         *testing.T
	pool                                                      *pgxpool.Pool
	gateway                                                   *BookingGateway
	customerID, vehicleID, dealershipID, serviceTypeID        string
	firstSkillID, secondSkillID, qualifiedTechnicianID, bayID string
}

func newBookingFixture(t *testing.T) *bookingFixture {
	t.Helper()
	f := &bookingFixture{t: t, pool: schemaPool(t)}
	f.gateway = NewBookingGateway(f.pool)
	f.customerID, f.vehicleID, f.dealershipID, f.serviceTypeID = randomUUID(t), randomUUID(t), randomUUID(t), randomUUID(t)
	f.firstSkillID, f.secondSkillID, f.qualifiedTechnicianID, f.bayID = randomUUID(t), randomUUID(t), randomUUID(t), randomUUID(t)
	ctx := context.Background()
	statements := []struct {
		sql  string
		args []any
	}{
		{`insert into customers(id,name,email) values($1,'Booking Customer',$2)`, []any{f.customerID, f.customerID + "@example.invalid"}},
		{`insert into vehicles(id,customer_id,label,registration) values($1,$2,'Booking Vehicle',$3)`, []any{f.vehicleID, f.customerID, f.vehicleID}},
		{`insert into dealerships(id,name,address,timezone) values($1,'Booking Centre','Test Way','UTC')`, []any{f.dealershipID}},
		{`insert into dealership_business_hours(dealership_id,day_of_week,opens_at,closes_at) values($1,2,'09:00','17:00')`, []any{f.dealershipID}},
		{`insert into service_types(id,name,description,duration_minutes) values($1,$2,'Booking test',75)`, []any{f.serviceTypeID, "Service " + f.serviceTypeID}},
		{`insert into skills(id,name) values($1,$2),($3,$4)`, []any{f.firstSkillID, "Skill " + f.firstSkillID, f.secondSkillID, "Skill " + f.secondSkillID}},
		{`insert into service_type_required_skills(service_type_id,skill_id) values($1,$2),($1,$3)`, []any{f.serviceTypeID, f.firstSkillID, f.secondSkillID}},
		{`insert into technicians(id,dealership_id,name) values($1,$2,'Qualified Technician')`, []any{f.qualifiedTechnicianID, f.dealershipID}},
		{`insert into technician_skills(technician_id,skill_id) values($1,$2),($1,$3)`, []any{f.qualifiedTechnicianID, f.firstSkillID, f.secondSkillID}},
		{`insert into service_bays(id,dealership_id,name) values($1,$2,'Booking Bay')`, []any{f.bayID, f.dealershipID}},
	}
	for _, statement := range statements {
		if _, err := f.pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { f.cleanup() })
	return f
}

func (f *bookingFixture) command() application.ConfirmCommand {
	return application.ConfirmCommand{VehicleID: f.vehicleID, DealershipID: f.dealershipID, ServiceTypeID: f.serviceTypeID, StartAt: time.Date(2031, 3, 4, 9, 30, 0, 0, time.UTC)}
}
func (f *bookingFixture) setActive(t *testing.T, table, id string, active bool) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), fmt.Sprintf("update %s set active=$1 where id=$2", table), active, id); err != nil {
		t.Fatal(err)
	}
}
func (f *bookingFixture) removeTechnicianSkill(t *testing.T, skill string) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `delete from technician_skills where technician_id=$1 and skill_id=$2`, f.qualifiedTechnicianID, skill); err != nil {
		t.Fatal(err)
	}
}
func (f *bookingFixture) addQualifiedTechnician(t *testing.T, id string) string {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `insert into technicians(id,dealership_id,name) values($1,$2,'Alternative Technician')`, id, f.dealershipID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `insert into technician_skills(technician_id,skill_id) values($1,$2),($1,$3)`, id, f.firstSkillID, f.secondSkillID); err != nil {
		t.Fatal(err)
	}
	return id
}
func (f *bookingFixture) addBay(t *testing.T, id string) string {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `insert into service_bays(id,dealership_id,name) values($1,$2,$3)`, id, f.dealershipID, "Bay "+id); err != nil {
		t.Fatal(err)
	}
	return id
}
func (f *bookingFixture) addFutureLoad(t *testing.T, tech, bay string) {
	t.Helper()
	start := time.Date(2032, 1, 1, 9, 0, 0, 0, time.UTC)
	f.insertAppointment(t, tech, bay, start, start.Add(time.Hour))
}
func (f *bookingFixture) occupyTechnician(t *testing.T) {
	otherBay := f.addBay(t, randomUUID(t))
	c := f.command()
	f.insertAppointment(t, f.qualifiedTechnicianID, otherBay, c.StartAt, c.StartAt.Add(75*time.Minute))
}
func (f *bookingFixture) occupyBay(t *testing.T) {
	otherTech := f.addQualifiedTechnician(t, randomUUID(t))
	c := f.command()
	f.insertAppointment(t, otherTech, f.bayID, c.StartAt, c.StartAt.Add(75*time.Minute))
	f.setActive(t, "technicians", otherTech, false)
}
func (f *bookingFixture) insertAppointment(t *testing.T, tech, bay string, start, end time.Time) {
	t.Helper()
	_, err := f.pool.Exec(context.Background(), `insert into appointments(customer_id,vehicle_id,dealership_id,service_type_id,technician_id,service_bay_id,start_at,end_at) values($1,$2,$3,$4,$5,$6,$7,$8)`, f.customerID, f.vehicleID, f.dealershipID, f.serviceTypeID, tech, bay, start, end)
	if err != nil {
		t.Fatal(err)
	}
}
func (f *bookingFixture) addInactiveVehicle(t *testing.T) string {
	t.Helper()
	id := randomUUID(t)
	_, err := f.pool.Exec(context.Background(), `insert into vehicles(id,customer_id,label,registration,active) values($1,$2,'Inactive',$3,false)`, id, f.customerID, id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func (f *bookingFixture) addVehicle(t *testing.T) string {
	t.Helper()
	id := randomUUID(t)
	_, err := f.pool.Exec(context.Background(), `insert into vehicles(id,customer_id,label,registration) values($1,$2,'Additional Vehicle',$3)`, id, f.customerID, id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func (f *bookingFixture) assertAppointmentCount(t *testing.T, want int) {
	t.Helper()
	var got int
	if err := f.pool.QueryRow(context.Background(), `select count(*) from appointments where dealership_id=$1`, f.dealershipID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("appointment count=%d want %d", got, want)
	}
}
func (f *bookingFixture) cleanup() {
	ctx := context.Background()
	statements := []struct {
		sql  string
		args []any
	}{
		{`delete from idempotency_records where appointment_id in(select id from appointments where dealership_id=$1)`, []any{f.dealershipID}},
		{`delete from appointments where dealership_id=$1`, []any{f.dealershipID}},
		{`delete from technician_skills where technician_id in(select id from technicians where dealership_id=$1)`, []any{f.dealershipID}},
		{`delete from technicians where dealership_id=$1`, []any{f.dealershipID}},
		{`delete from service_bays where dealership_id=$1`, []any{f.dealershipID}},
		{`delete from service_type_required_skills where service_type_id=$1`, []any{f.serviceTypeID}},
		{`delete from skills where id in($1,$2)`, []any{f.firstSkillID, f.secondSkillID}},
		{`delete from service_types where id=$1`, []any{f.serviceTypeID}},
		{`delete from vehicles where customer_id=$1`, []any{f.customerID}},
		{`delete from dealerships where id=$1`, []any{f.dealershipID}},
		{`delete from customers where id=$1`, []any{f.customerID}},
	}
	for _, statement := range statements {
		_, _ = f.pool.Exec(ctx, statement.sql, statement.args...)
	}
}

func randomUUID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
func contains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
