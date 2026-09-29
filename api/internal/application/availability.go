package application

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"scheduler/api/internal/domain"
)

const availabilitySlotInterval = 30 * time.Minute

// AvailabilityQuery identifies one local calendar date to search. Date uses
// YYYY-MM-DD and is interpreted only in the dealership timezone loaded by the
// repository.
type AvailabilityQuery struct {
	VehicleID     string
	DealershipID  string
	ServiceTypeID string
	Date          string
}

type AvailabilityReference struct {
	ID     string
	Active bool
}

// LocalBusinessHours stores clock offsets from local midnight. Instants are
// built strictly in the dealership timezone by AvailabilityService.
type LocalBusinessHours struct {
	OpensAt  time.Duration
	ClosesAt time.Duration
}

// AvailabilityContext is a bounded scheduling projection loaded in one
// repository call for the requested dealership and local date.
type AvailabilityContext struct {
	Vehicle         AvailabilityReference
	Dealership      AvailabilityReference
	ServiceType     AvailabilityReference
	TimeZone        string
	BusinessHours   []LocalBusinessHours
	ServiceDuration domain.Duration
	RequiredSkills  []domain.SkillID
	Technicians     []domain.Technician
	Bays            []domain.Bay
	TechnicianBusy  map[domain.TechnicianID][]domain.Interval
	BayBusy         map[domain.BayID][]domain.Interval
}

type AvailabilityRepository interface {
	LoadAvailabilityContext(context.Context, AvailabilityQuery) (AvailabilityContext, error)
}

type AvailabilityService struct {
	repository AvailabilityRepository
}

func NewAvailabilityService(repository AvailabilityRepository) AvailabilityService {
	return AvailabilityService{repository: repository}
}

func (s AvailabilityService) AvailableSlots(ctx context.Context, query AvailabilityQuery) ([]domain.Interval, error) {
	date, err := validateAvailabilityQuery(query)
	if err != nil {
		return nil, err
	}
	if s.repository == nil {
		return nil, &AvailabilityContextError{Reason: "repository is required"}
	}

	loaded, err := s.repository.LoadAvailabilityContext(ctx, query)
	if err != nil {
		return nil, err
	}
	location, technicians, bays, err := validateAvailabilityContext(query, loaded)
	if err != nil {
		return nil, err
	}
	if len(technicians) == 0 || len(bays) == 0 || len(loaded.BusinessHours) == 0 {
		return []domain.Interval{}, nil
	}

	hours := append([]LocalBusinessHours(nil), loaded.BusinessHours...)
	sort.Slice(hours, func(i, j int) bool {
		if hours[i].OpensAt != hours[j].OpensAt {
			return hours[i].OpensAt < hours[j].OpensAt
		}
		return hours[i].ClosesAt < hours[j].ClosesAt
	})

	slots := make([]domain.Interval, 0)
	seen := make(map[int64]struct{})
	for _, schedule := range hours {
		opening, err := strictLocalInstant(date, schedule.OpensAt, location)
		if err != nil {
			return nil, &AvailabilityContextError{Reason: fmt.Sprintf("opening time: %v", err)}
		}
		closing, err := strictLocalInstant(date, schedule.ClosesAt, location)
		if err != nil {
			return nil, &AvailabilityContextError{Reason: fmt.Sprintf("closing time: %v", err)}
		}
		businessInterval, err := domain.NewInterval(opening, closing)
		if err != nil {
			return nil, &AvailabilityContextError{Reason: "business hours must have a positive interval"}
		}

		start := nextGridBoundary(opening, location)
		for !start.After(closing) {
			candidate, err := domain.NewInterval(start, loaded.ServiceDuration.End(start))
			if err != nil {
				return nil, &AvailabilityContextError{Reason: "service duration produced an invalid interval"}
			}
			if !candidate.Within(businessInterval) {
				break
			}
			if hasFreeTechnician(candidate, technicians, loaded.TechnicianBusy) && hasFreeBay(candidate, bays, loaded.BayBusy) {
				key := candidate.Start().UnixNano()
				if _, exists := seen[key]; !exists {
					slots = append(slots, candidate)
					seen[key] = struct{}{}
				}
			}
			start = start.Add(availabilitySlotInterval)
		}
	}

	sort.Slice(slots, func(i, j int) bool { return slots[i].Start().Before(slots[j].Start()) })
	return slots, nil
}

func validateAvailabilityQuery(query AvailabilityQuery) (time.Time, error) {
	for _, input := range []struct {
		field string
		value string
	}{
		{"vehicleId", query.VehicleID},
		{"dealershipId", query.DealershipID},
		{"serviceTypeId", query.ServiceTypeID},
	} {
		if strings.TrimSpace(input.value) == "" {
			return time.Time{}, &ValidationError{Field: input.field, Reason: "is required"}
		}
	}
	date, err := time.Parse("2006-01-02", query.Date)
	if err != nil || date.Format("2006-01-02") != query.Date {
		return time.Time{}, &ValidationError{Field: "date", Reason: "must be a valid YYYY-MM-DD calendar date"}
	}
	return date, nil
}

func validateAvailabilityContext(query AvailabilityQuery, loaded AvailabilityContext) (*time.Location, []domain.Technician, []domain.Bay, error) {
	references := []struct {
		name string
		want string
		got  AvailabilityReference
	}{
		{"vehicle", query.VehicleID, loaded.Vehicle},
		{"dealership", query.DealershipID, loaded.Dealership},
		{"service type", query.ServiceTypeID, loaded.ServiceType},
	}
	for _, reference := range references {
		if reference.got.ID != reference.want {
			return nil, nil, nil, &AvailabilityContextError{Reason: reference.name + " does not match the query"}
		}
		if !reference.got.Active {
			return nil, nil, nil, &InvalidReferenceError{Reference: reference.name}
		}
	}
	if loaded.ServiceDuration == nil {
		return nil, nil, nil, &AvailabilityContextError{Reason: "service duration is required"}
	}
	if len(loaded.RequiredSkills) == 0 {
		return nil, nil, nil, &AvailabilityContextError{Reason: "service type must require at least one skill"}
	}
	location, err := time.LoadLocation(loaded.TimeZone)
	if err != nil {
		return nil, nil, nil, &AvailabilityContextError{Reason: "dealership timezone is invalid"}
	}
	for _, schedule := range loaded.BusinessHours {
		if schedule.OpensAt < 0 || schedule.ClosesAt > 24*time.Hour || schedule.OpensAt >= schedule.ClosesAt || schedule.OpensAt%time.Minute != 0 || schedule.ClosesAt%time.Minute != 0 {
			return nil, nil, nil, &AvailabilityContextError{Reason: "business hours are invalid"}
		}
	}
	technicians, err := domain.EligibleTechnicians(loaded.Technicians, loaded.RequiredSkills)
	if err != nil {
		return nil, nil, nil, &AvailabilityContextError{Reason: err.Error()}
	}
	return location, domain.OrderTechnicians(technicians), domain.OrderBays(domain.ActiveBays(loaded.Bays)), nil
}

func strictLocalInstant(date time.Time, sinceMidnight time.Duration, location *time.Location) (time.Time, error) {
	if sinceMidnight < 0 || sinceMidnight > 24*time.Hour || sinceMidnight%time.Minute != 0 {
		return time.Time{}, fmt.Errorf("invalid clock value")
	}
	day := date
	if sinceMidnight == 24*time.Hour {
		day = date.AddDate(0, 0, 1)
		sinceMidnight = 0
	}
	hour := int(sinceMidnight / time.Hour)
	minute := int((sinceMidnight % time.Hour) / time.Minute)
	local := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, location)
	if local.Year() != day.Year() || local.Month() != day.Month() || local.Day() != day.Day() || local.Hour() != hour || local.Minute() != minute {
		return time.Time{}, fmt.Errorf("local clock time does not exist in dealership timezone")
	}
	// Go selects one offset when a local wall time occurs twice. Discover the
	// offsets on both sides of nearby transitions, then test whether any other
	// offset maps the same wall clock to a distinct instant. This handles zones
	// with non-hour folds, including Lord Howe Island's 30-minute transition.
	wallAsUTC := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, time.UTC)
	_, selectedOffset := local.Zone()
	nearbyOffsets := map[int]struct{}{selectedOffset: {}}
	const transitionWindow = 7 * 24 * time.Hour
	const transitionProbe = 6 * time.Hour
	for delta := -transitionWindow; delta <= transitionWindow; delta += transitionProbe {
		_, offset := local.Add(delta).In(location).Zone()
		nearbyOffsets[offset] = struct{}{}
	}
	for offset := range nearbyOffsets {
		if offset == selectedOffset {
			continue
		}
		alternative := wallAsUTC.Add(-time.Duration(offset) * time.Second).In(location)
		if sameLocalMinute(alternative, day, hour, minute) {
			return time.Time{}, fmt.Errorf("local clock time is ambiguous in dealership timezone")
		}
	}
	return local.UTC(), nil
}

func sameLocalMinute(instant, day time.Time, hour, minute int) bool {
	return instant.Year() == day.Year() &&
		instant.Month() == day.Month() &&
		instant.Day() == day.Day() &&
		instant.Hour() == hour &&
		instant.Minute() == minute
}

func nextGridBoundary(instant time.Time, location *time.Location) time.Time {
	local := instant.In(location)
	minutes := local.Hour()*60 + local.Minute()
	remainder := minutes % int(availabilitySlotInterval/time.Minute)
	if remainder == 0 && local.Second() == 0 && local.Nanosecond() == 0 {
		return instant.UTC()
	}
	advance := int(availabilitySlotInterval/time.Minute) - remainder
	return local.Truncate(time.Minute).Add(time.Duration(advance) * time.Minute).UTC()
}

func hasFreeTechnician(candidate domain.Interval, technicians []domain.Technician, busy map[domain.TechnicianID][]domain.Interval) bool {
	for _, technician := range technicians {
		if isFree(candidate, busy[technician.ID()]) {
			return true
		}
	}
	return false
}

func hasFreeBay(candidate domain.Interval, bays []domain.Bay, busy map[domain.BayID][]domain.Interval) bool {
	for _, bay := range bays {
		if isFree(candidate, busy[bay.ID()]) {
			return true
		}
	}
	return false
}

func isFree(candidate domain.Interval, busy []domain.Interval) bool {
	for _, occupied := range busy {
		if occupied == nil || candidate.Overlaps(occupied) {
			return false
		}
	}
	return true
}
