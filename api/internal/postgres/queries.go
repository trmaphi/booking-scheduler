package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"scheduler/api/internal/application"
	"scheduler/api/internal/domain"
	"scheduler/api/internal/telemetry"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) BookingOptions(ctx context.Context) (result application.BookingOptions, returnedErr error) {
	event := telemetry.DatabaseEvent{Operation: telemetry.DatabaseBookingOptions}
	started := r.now()
	defer func() {
		event.Result = databaseResult(returnedErr)
		event.Duration = r.now().Sub(started)
		clearUntrustedDatabaseIDs(&event)
		telemetry.RecordDatabase(r.recorder(), ctx, event)
	}()
	if r == nil || r.pool == nil {
		return application.BookingOptions{}, application.ErrPersistence
	}
	rows, err := r.pool.Query(ctx, `select v.id::text,v.customer_id::text,c.name,v.label,v.registration from vehicles v join customers c on c.id=v.customer_id where v.active and c.active order by v.id`)
	if err != nil {
		return result, mapReadError(ctx, err)
	}
	for rows.Next() {
		var value application.VehicleOption
		if err := rows.Scan(&value.ID, &value.CustomerID, &value.CustomerName, &value.Label, &value.Registration); err != nil {
			rows.Close()
			return result, mapReadError(ctx, err)
		}
		result.Vehicles = append(result.Vehicles, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, mapReadError(ctx, err)
	}
	rows.Close()
	rows, err = r.pool.Query(ctx, `select id::text,name,address,timezone from dealerships where active order by id`)
	if err != nil {
		return result, mapReadError(ctx, err)
	}
	for rows.Next() {
		var value application.DealershipOption
		if err := rows.Scan(&value.ID, &value.Name, &value.Address, &value.TimeZone); err != nil {
			rows.Close()
			return result, mapReadError(ctx, err)
		}
		result.Dealerships = append(result.Dealerships, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, mapReadError(ctx, err)
	}
	rows.Close()
	rows, err = r.pool.Query(ctx, `select id::text,name,description,duration_minutes from service_types where active and duration_minutes>0 order by id`)
	if err != nil {
		return result, mapReadError(ctx, err)
	}
	for rows.Next() {
		var value application.ServiceTypeOption
		if err := rows.Scan(&value.ID, &value.Name, &value.Description, &value.DurationMinutes); err != nil {
			rows.Close()
			return result, mapReadError(ctx, err)
		}
		result.ServiceTypes = append(result.ServiceTypes, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, mapReadError(ctx, err)
	}
	rows.Close()
	return result, nil
}

func (r *Repository) LoadAvailabilityContext(ctx context.Context, query application.AvailabilityQuery) (result application.AvailabilityContext, returnedErr error) {
	event := telemetry.DatabaseEvent{Operation: telemetry.DatabaseAvailability, DealershipID: query.DealershipID, ServiceTypeID: query.ServiceTypeID}
	started := r.now()
	defer func() {
		event.Result = databaseResult(returnedErr)
		event.Duration = r.now().Sub(started)
		clearUntrustedDatabaseIDs(&event)
		telemetry.RecordDatabase(r.recorder(), ctx, event)
	}()
	if r == nil || r.pool == nil {
		return result, application.ErrPersistence
	}
	if !validUUID(query.VehicleID) || !validUUID(query.DealershipID) || !validUUID(query.ServiceTypeID) {
		return result, application.ErrInvalidReference
	}
	date, err := time.Parse("2006-01-02", query.Date)
	if err != nil || date.Format("2006-01-02") != query.Date {
		return result, application.ErrValidation
	}
	var durationMinutes int
	err = r.pool.QueryRow(ctx, `select v.id::text,v.active and c.active,d.id::text,d.active,d.timezone,s.id::text,s.active,s.duration_minutes from vehicles v join customers c on c.id=v.customer_id cross join dealerships d cross join service_types s where v.id=$1 and d.id=$2 and s.id=$3`, query.VehicleID, query.DealershipID, query.ServiceTypeID).Scan(&result.Vehicle.ID, &result.Vehicle.Active, &result.Dealership.ID, &result.Dealership.Active, &result.TimeZone, &result.ServiceType.ID, &result.ServiceType.Active, &durationMinutes)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, application.ErrInvalidReference
	}
	if err != nil {
		return result, mapReadError(ctx, err)
	}
	if !result.Vehicle.Active || !result.Dealership.Active || !result.ServiceType.Active {
		return result, application.ErrInvalidReference
	}
	result.ServiceDuration, err = domain.NewDuration(durationMinutes)
	if err != nil {
		return result, application.ErrInvalidReference
	}
	location, err := time.LoadLocation(result.TimeZone)
	if err != nil {
		return result, application.ErrPersistence
	}
	localStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, location)
	localEnd := localStart.AddDate(0, 0, 1)
	rows, err := r.pool.Query(ctx, `select opens_at,closes_at from dealership_business_hours where dealership_id=$1 and day_of_week=$2 order by opens_at`, query.DealershipID, int(date.Weekday()))
	if err != nil {
		return result, mapReadError(ctx, err)
	}
	for rows.Next() {
		var open, close time.Time
		if err := rows.Scan(&open, &close); err != nil {
			rows.Close()
			return result, mapReadError(ctx, err)
		}
		result.BusinessHours = append(result.BusinessHours, application.LocalBusinessHours{OpensAt: clockDuration(open), ClosesAt: clockDuration(close)})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, mapReadError(ctx, err)
	}
	rows.Close()
	rows, err = r.pool.Query(ctx, `select r.skill_id::text,s.active from service_type_required_skills r join skills s on s.id=r.skill_id where r.service_type_id=$1 order by r.skill_id`, query.ServiceTypeID)
	if err != nil {
		return result, mapReadError(ctx, err)
	}
	for rows.Next() {
		var id string
		var active bool
		if err := rows.Scan(&id, &active); err != nil {
			rows.Close()
			return result, mapReadError(ctx, err)
		}
		if !active {
			rows.Close()
			return result, application.ErrInvalidReference
		}
		result.RequiredSkills = append(result.RequiredSkills, domain.SkillID(id))
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, mapReadError(ctx, err)
	}
	rows.Close()
	if len(result.RequiredSkills) == 0 {
		return result, application.ErrInvalidReference
	}
	rows, err = r.pool.Query(ctx, `select t.id::text,coalesce(array_agg(ts.skill_id::text order by ts.skill_id) filter(where sk.active),array[]::text[]),count(distinct a.id) filter(where a.status='CONFIRMED' and a.start_at >= $2 and a.start_at < $3) from technicians t left join technician_skills ts on ts.technician_id=t.id left join skills sk on sk.id=ts.skill_id left join appointments a on a.technician_id=t.id where t.dealership_id=$1 and t.active group by t.id order by t.id`, query.DealershipID, localStart.UTC(), localEnd.UTC())
	if err != nil {
		return result, mapReadError(ctx, err)
	}
	for rows.Next() {
		var id string
		var skills []string
		var load int
		if err := rows.Scan(&id, &skills, &load); err != nil {
			rows.Close()
			return result, mapReadError(ctx, err)
		}
		ids := make([]domain.SkillID, len(skills))
		for i := range skills {
			ids[i] = domain.SkillID(skills[i])
		}
		tech, err := domain.NewTechnician(domain.TechnicianID(id), true, ids, load)
		if err != nil {
			rows.Close()
			return result, application.ErrPersistence
		}
		result.Technicians = append(result.Technicians, tech)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, mapReadError(ctx, err)
	}
	rows.Close()
	rows, err = r.pool.Query(ctx, `select b.id::text,count(a.id) filter(where a.status='CONFIRMED' and a.start_at >= $2 and a.start_at < $3) from service_bays b left join appointments a on a.service_bay_id=b.id where b.dealership_id=$1 and b.active group by b.id order by b.id`, query.DealershipID, localStart.UTC(), localEnd.UTC())
	if err != nil {
		return result, mapReadError(ctx, err)
	}
	for rows.Next() {
		var id string
		var load int
		if err := rows.Scan(&id, &load); err != nil {
			rows.Close()
			return result, mapReadError(ctx, err)
		}
		bay, err := domain.NewBay(domain.BayID(id), true, load)
		if err != nil {
			rows.Close()
			return result, application.ErrPersistence
		}
		result.Bays = append(result.Bays, bay)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, mapReadError(ctx, err)
	}
	rows.Close()
	result.TechnicianBusy = map[domain.TechnicianID][]domain.Interval{}
	result.BayBusy = map[domain.BayID][]domain.Interval{}
	rows, err = r.pool.Query(ctx, `select technician_id::text,service_bay_id::text,start_at,end_at from appointments where dealership_id=$1 and status='CONFIRMED' and start_at<$3 and end_at>$2 order by start_at,id`, query.DealershipID, localStart.UTC(), localEnd.UTC())
	if err != nil {
		return result, mapReadError(ctx, err)
	}
	for rows.Next() {
		var tech, bay string
		var start, end time.Time
		if err := rows.Scan(&tech, &bay, &start, &end); err != nil {
			rows.Close()
			return result, mapReadError(ctx, err)
		}
		interval, err := domain.NewInterval(start, end)
		if err != nil {
			rows.Close()
			return result, application.ErrPersistence
		}
		result.TechnicianBusy[domain.TechnicianID(tech)] = append(result.TechnicianBusy[domain.TechnicianID(tech)], interval)
		result.BayBusy[domain.BayID(bay)] = append(result.BayBusy[domain.BayID(bay)], interval)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, mapReadError(ctx, err)
	}
	rows.Close()
	return result, nil
}

func clockDuration(value time.Time) time.Duration {
	return time.Duration(value.Hour())*time.Hour + time.Duration(value.Minute())*time.Minute + time.Duration(value.Second())*time.Second
}

func (r *Repository) AppointmentByID(ctx context.Context, id string) (appointment application.Appointment, returnedErr error) {
	event := telemetry.DatabaseEvent{Operation: telemetry.DatabaseAppointmentByID}
	started := r.now()
	defer func() {
		event.Result = databaseResult(returnedErr)
		event.Duration = r.now().Sub(started)
		clearUntrustedDatabaseIDs(&event)
		telemetry.RecordDatabase(r.recorder(), ctx, event)
	}()
	if r == nil || r.pool == nil {
		return application.Appointment{}, application.ErrPersistence
	}
	if !validUUID(id) {
		return application.Appointment{}, application.ErrNotFound
	}
	return appointmentByID(ctx, r.pool, id)
}

func (r *Repository) Appointments(ctx context.Context, status string) ([]application.Appointment, error) {
	if r == nil || r.pool == nil {
		return nil, application.ErrPersistence
	}
	if status != "" && status != "CONFIRMED" && status != "CANCELLED" {
		return nil, application.ErrValidation
	}
	rows, err := r.pool.Query(ctx, `select a.id::text,a.customer_id::text,a.vehicle_id::text,a.dealership_id::text,a.service_type_id::text,a.technician_id::text,a.service_bay_id::text,a.status,a.start_at,a.end_at,a.created_at,c.name,v.label,v.registration,d.name,d.address,d.timezone,s.name,s.description,s.duration_minutes,t.name,b.name from appointments a join customers c on c.id=a.customer_id join vehicles v on v.id=a.vehicle_id join dealerships d on d.id=a.dealership_id join service_types s on s.id=a.service_type_id join technicians t on t.id=a.technician_id join service_bays b on b.id=a.service_bay_id where ($1='' or a.status=$1) order by case when a.start_at >= now() then 0 else 1 end, case when a.start_at >= now() then a.start_at end asc, case when a.start_at < now() then a.start_at end desc, a.id`, status)
	if err != nil {
		return nil, mapReadError(ctx, err)
	}
	defer rows.Close()
	appointments := []application.Appointment{}
	for rows.Next() {
		var a application.Appointment
		if err := rows.Scan(&a.ID, &a.CustomerID, &a.VehicleID, &a.DealershipID, &a.ServiceTypeID, &a.TechnicianID, &a.ServiceBayID, &a.Status, &a.StartAt, &a.EndAt, &a.CreatedAt, &a.CustomerName, &a.VehicleLabel, &a.Registration, &a.DealershipName, &a.DealershipAddress, &a.DealershipTimeZone, &a.ServiceTypeName, &a.ServiceTypeDescription, &a.ServiceDurationMinutes, &a.TechnicianName, &a.ServiceBayName); err != nil {
			return nil, mapReadError(ctx, err)
		}
		a.StartAt = a.StartAt.UTC()
		a.EndAt = a.EndAt.UTC()
		a.CreatedAt = a.CreatedAt.UTC()
		appointments = append(appointments, a)
	}
	if err := rows.Err(); err != nil {
		return nil, mapReadError(ctx, err)
	}
	return appointments, nil
}

func appointmentByID(ctx context.Context, db queryRower, id string) (application.Appointment, error) {
	var a application.Appointment
	err := db.QueryRow(ctx, `select a.id::text,a.customer_id::text,a.vehicle_id::text,a.dealership_id::text,a.service_type_id::text,a.technician_id::text,a.service_bay_id::text,a.status,a.start_at,a.end_at,a.created_at,c.name,v.label,v.registration,d.name,d.address,d.timezone,s.name,s.description,s.duration_minutes,t.name,b.name from appointments a join customers c on c.id=a.customer_id join vehicles v on v.id=a.vehicle_id join dealerships d on d.id=a.dealership_id join service_types s on s.id=a.service_type_id join technicians t on t.id=a.technician_id join service_bays b on b.id=a.service_bay_id where a.id=$1`, id).Scan(&a.ID, &a.CustomerID, &a.VehicleID, &a.DealershipID, &a.ServiceTypeID, &a.TechnicianID, &a.ServiceBayID, &a.Status, &a.StartAt, &a.EndAt, &a.CreatedAt, &a.CustomerName, &a.VehicleLabel, &a.Registration, &a.DealershipName, &a.DealershipAddress, &a.DealershipTimeZone, &a.ServiceTypeName, &a.ServiceTypeDescription, &a.ServiceDurationMinutes, &a.TechnicianName, &a.ServiceBayName)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, application.ErrNotFound
	}
	if err != nil {
		return a, mapReadError(ctx, err)
	}
	a.StartAt = a.StartAt.UTC()
	a.EndAt = a.EndAt.UTC()
	a.CreatedAt = a.CreatedAt.UTC()
	return a, nil
}

func mapReadError(ctx context.Context, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		return errors.Join(ctx.Err(), err)
	}
	return fmt.Errorf("%w", application.ErrPersistence)
}

var _ application.AvailabilityRepository = (*Repository)(nil)
