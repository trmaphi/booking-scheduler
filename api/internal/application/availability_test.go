package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"scheduler/api/internal/domain"
)

const (
	vehicleID     = "vehicle-active"
	dealershipID  = "dealership-active"
	serviceTypeID = "service-active"
)

type countingAvailabilityRepository struct {
	context AvailabilityContext
	err     error
	calls   int
	queries []AvailabilityQuery
}

func (r *countingAvailabilityRepository) LoadAvailabilityContext(_ context.Context, query AvailabilityQuery) (AvailabilityContext, error) {
	r.calls++
	r.queries = append(r.queries, query)
	return r.context, r.err
}

func TestAvailabilityReturnsOnlySlotsWithCompleteResourcePairs(t *testing.T) {
	ctx := baseAvailabilityContext(t)
	ctx.TechnicianBusy["tech-a"] = []domain.Interval{interval(t, "2030-01-02T09:30:00Z", "2030-01-02T11:30:00Z")}
	ctx.TechnicianBusy["tech-b"] = []domain.Interval{interval(t, "2030-01-02T10:00:00Z", "2030-01-02T11:30:00Z")}
	ctx.BayBusy["bay-a"] = []domain.Interval{interval(t, "2030-01-02T09:30:00Z", "2030-01-02T11:30:00Z")}
	ctx.BayBusy["bay-b"] = []domain.Interval{interval(t, "2030-01-02T10:00:00Z", "2030-01-02T11:30:00Z")}
	repo := &countingAvailabilityRepository{context: ctx}

	got, err := NewAvailabilityService(repo).AvailableSlots(context.Background(), validAvailabilityQuery())
	if err != nil {
		t.Fatal(err)
	}
	assertSlotStarts(t, got, "2030-01-02T09:00:00Z", "2030-01-02T11:30:00Z")
	if repo.calls != 1 {
		t.Fatalf("repository calls = %d, want 1", repo.calls)
	}
}

func TestAvailabilityRejectsPartialTechnicianOverlap(t *testing.T) {
	ctx := baseAvailabilityContext(t)
	ctx.Technicians = ctx.Technicians[:1]
	ctx.TechnicianBusy["tech-a"] = []domain.Interval{interval(t, "2030-01-02T09:59:00Z", "2030-01-02T10:01:00Z")}
	assertMissingSlot(t, ctx, "2030-01-02T09:30:00Z")
}

func TestAvailabilityRejectsPartialBayOverlap(t *testing.T) {
	ctx := baseAvailabilityContext(t)
	ctx.Bays = ctx.Bays[:1]
	ctx.BayBusy["bay-a"] = []domain.Interval{interval(t, "2030-01-02T09:59:00Z", "2030-01-02T10:01:00Z")}
	assertMissingSlot(t, ctx, "2030-01-02T09:30:00Z")
}

func TestAvailabilityAllowsBoundaryTouchingAppointments(t *testing.T) {
	ctx := baseAvailabilityContext(t)
	ctx.Technicians = ctx.Technicians[:1]
	ctx.Bays = ctx.Bays[:1]
	ctx.TechnicianBusy["tech-a"] = []domain.Interval{interval(t, "2030-01-02T08:00:00Z", "2030-01-02T09:00:00Z")}
	ctx.BayBusy["bay-a"] = []domain.Interval{interval(t, "2030-01-02T10:00:00Z", "2030-01-02T11:00:00Z")}
	assertContainsSlot(t, ctx, "2030-01-02T09:00:00Z")
}

func TestAvailabilityReturnsEmptyWithoutQualifiedTechnician(t *testing.T) {
	ctx := baseAvailabilityContext(t)
	ctx.Technicians = []domain.Technician{technician(t, "tech-unqualified", true, []domain.SkillID{"paint"})}
	assertNoSlots(t, ctx)
}

func TestAvailabilityReturnsEmptyWithoutActiveBay(t *testing.T) {
	ctx := baseAvailabilityContext(t)
	ctx.Bays = []domain.Bay{bay(t, "bay-inactive", false)}
	assertNoSlots(t, ctx)
}

func TestAvailabilityFiltersInactiveResources(t *testing.T) {
	ctx := baseAvailabilityContext(t)
	ctx.Technicians = []domain.Technician{
		technician(t, "tech-inactive", false, []domain.SkillID{"routine", "diagnostic"}),
		technician(t, "tech-active", true, []domain.SkillID{"routine", "diagnostic"}),
	}
	ctx.Bays = []domain.Bay{bay(t, "bay-inactive", false), bay(t, "bay-active", true)}
	ctx.TechnicianBusy["tech-active"] = []domain.Interval{interval(t, "2030-01-02T09:00:00Z", "2030-01-02T12:00:00Z")}
	ctx.BayBusy["bay-active"] = []domain.Interval{interval(t, "2030-01-02T09:00:00Z", "2030-01-02T12:00:00Z")}
	assertNoSlots(t, ctx)
}

func TestAvailabilityRequiresEveryServiceSkill(t *testing.T) {
	ctx := baseAvailabilityContext(t)
	ctx.RequiredSkills = []domain.SkillID{"routine", "diagnostic"}
	ctx.Technicians = []domain.Technician{
		technician(t, "tech-routine", true, []domain.SkillID{"routine"}),
		technician(t, "tech-diagnostic", true, []domain.SkillID{"diagnostic"}),
	}
	assertNoSlots(t, ctx)
}

func TestAvailabilityExcludesDurationCrossingClosingTime(t *testing.T) {
	ctx := baseAvailabilityContext(t)
	assertContainsSlot(t, ctx, "2030-01-02T11:30:00Z")
	assertMissingSlot(t, ctx, "2030-01-02T12:00:00Z")
}

func TestAvailabilityReturnsEmptyForNoBusinessHours(t *testing.T) {
	ctx := baseAvailabilityContext(t)
	ctx.BusinessHours = nil
	assertNoSlots(t, ctx)
}

func TestAvailabilityResultsAreDeterministicUTCAndUseOneRead(t *testing.T) {
	ctx := baseAvailabilityContext(t)
	ctx.BusinessHours = []LocalBusinessHours{{OpensAt: 11 * time.Hour, ClosesAt: 12*time.Hour + 30*time.Minute}, {OpensAt: 9 * time.Hour, ClosesAt: 10*time.Hour + 30*time.Minute}}
	ctx.Technicians[0], ctx.Technicians[1] = ctx.Technicians[1], ctx.Technicians[0]
	ctx.Bays[0], ctx.Bays[1] = ctx.Bays[1], ctx.Bays[0]
	repo := &countingAvailabilityRepository{context: ctx}
	got, err := NewAvailabilityService(repo).AvailableSlots(context.Background(), validAvailabilityQuery())
	if err != nil {
		t.Fatal(err)
	}
	assertSlotStarts(t, got, "2030-01-02T09:00:00Z", "2030-01-02T09:30:00Z", "2030-01-02T11:00:00Z", "2030-01-02T11:30:00Z")
	for _, slot := range got {
		if slot.Start().Location() != time.UTC || slot.End().Location() != time.UTC {
			t.Fatalf("slot is not UTC: %v", slot)
		}
	}
	if repo.calls != 1 {
		t.Fatalf("repository calls = %d, want 1 for %d slots", repo.calls, len(got))
	}
}

func TestAvailabilityUsesDealershipTimezoneAcrossDST(t *testing.T) {
	ctx := baseAvailabilityContext(t)
	ctx.TimeZone = "Europe/London"
	ctx.BusinessHours = []LocalBusinessHours{{OpensAt: 9 * time.Hour, ClosesAt: 10 * time.Hour}}
	query := validAvailabilityQuery()
	query.Date = "2030-06-03"
	repo := &countingAvailabilityRepository{context: ctx}
	got, err := NewAvailabilityService(repo).AvailableSlots(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	assertSlotStarts(t, got, "2030-06-03T08:00:00Z")
}

func TestAvailabilityRejectsNonexistentLocalBusinessTime(t *testing.T) {
	ctx := baseAvailabilityContext(t)
	ctx.TimeZone = "Europe/London"
	ctx.BusinessHours = []LocalBusinessHours{{OpensAt: 90 * time.Minute, ClosesAt: 3 * time.Hour}}
	query := validAvailabilityQuery()
	query.Date = "2030-03-31"
	repo := &countingAvailabilityRepository{context: ctx}
	_, err := NewAvailabilityService(repo).AvailableSlots(context.Background(), query)
	if !errors.Is(err, ErrInvalidAvailabilityContext) {
		t.Fatalf("error = %v, want ErrInvalidAvailabilityContext", err)
	}
}

func TestStrictLocalInstantRejectsLordHoweHalfHourFold(t *testing.T) {
	location, err := time.LoadLocation("Australia/Lord_Howe")
	if err != nil {
		t.Fatal(err)
	}
	date := time.Date(2030, time.April, 7, 0, 0, 0, 0, time.UTC)

	for _, clock := range []time.Duration{90 * time.Minute, 105 * time.Minute} {
		if got, err := strictLocalInstant(date, clock, location); err == nil {
			t.Fatalf("strictLocalInstant(%v) = %v, want ambiguous-time error", clock, got)
		}
	}

	for _, clock := range []time.Duration{75 * time.Minute, 2 * time.Hour} {
		if _, err := strictLocalInstant(date, clock, location); err != nil {
			t.Fatalf("strictLocalInstant(%v) rejected valid time: %v", clock, err)
		}
	}
}

func TestAvailabilityValidatesQueryBeforeRepositoryCall(t *testing.T) {
	for name, mutate := range map[string]func(*AvailabilityQuery){
		"vehicle":      func(q *AvailabilityQuery) { q.VehicleID = "" },
		"dealership":   func(q *AvailabilityQuery) { q.DealershipID = "" },
		"service type": func(q *AvailabilityQuery) { q.ServiceTypeID = "" },
		"date":         func(q *AvailabilityQuery) { q.Date = "2030-02-30" },
	} {
		t.Run(name, func(t *testing.T) {
			query := validAvailabilityQuery()
			mutate(&query)
			repo := &countingAvailabilityRepository{}
			_, err := NewAvailabilityService(repo).AvailableSlots(context.Background(), query)
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("error = %v, want ErrValidation", err)
			}
			if repo.calls != 0 {
				t.Fatalf("repository calls = %d, want 0", repo.calls)
			}
		})
	}
}

func TestAvailabilityValidatesActiveReferenceContext(t *testing.T) {
	for name, mutate := range map[string]func(*AvailabilityContext){
		"vehicle":      func(c *AvailabilityContext) { c.Vehicle.Active = false },
		"dealership":   func(c *AvailabilityContext) { c.Dealership.Active = false },
		"service type": func(c *AvailabilityContext) { c.ServiceType.Active = false },
	} {
		t.Run(name, func(t *testing.T) {
			ctx := baseAvailabilityContext(t)
			mutate(&ctx)
			repo := &countingAvailabilityRepository{context: ctx}
			_, err := NewAvailabilityService(repo).AvailableSlots(context.Background(), validAvailabilityQuery())
			if !errors.Is(err, ErrInvalidReference) {
				t.Fatalf("error = %v, want ErrInvalidReference", err)
			}
		})
	}
}

func TestAvailabilityRejectsMismatchedAndInvalidLoadedContext(t *testing.T) {
	tests := map[string]func(*AvailabilityContext){
		"mismatched vehicle": func(c *AvailabilityContext) { c.Vehicle.ID = "other" },
		"bad timezone":       func(c *AvailabilityContext) { c.TimeZone = "Mars/Olympus" },
		"no skills":          func(c *AvailabilityContext) { c.RequiredSkills = nil },
		"bad hours": func(c *AvailabilityContext) {
			c.BusinessHours = []LocalBusinessHours{{OpensAt: 10 * time.Hour, ClosesAt: 9 * time.Hour}}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := baseAvailabilityContext(t)
			mutate(&ctx)
			repo := &countingAvailabilityRepository{context: ctx}
			_, err := NewAvailabilityService(repo).AvailableSlots(context.Background(), validAvailabilityQuery())
			if !errors.Is(err, ErrInvalidAvailabilityContext) {
				t.Fatalf("error = %v, want ErrInvalidAvailabilityContext", err)
			}
		})
	}
}

func TestAvailabilityPropagatesRepositoryAndContextErrors(t *testing.T) {
	want := context.Canceled
	repo := &countingAvailabilityRepository{err: want}
	_, err := NewAvailabilityService(repo).AvailableSlots(context.Background(), validAvailabilityQuery())
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func baseAvailabilityContext(t *testing.T) AvailabilityContext {
	t.Helper()
	duration, err := domain.NewDuration(60)
	if err != nil {
		t.Fatal(err)
	}
	return AvailabilityContext{
		Vehicle:         AvailabilityReference{ID: vehicleID, Active: true},
		Dealership:      AvailabilityReference{ID: dealershipID, Active: true},
		ServiceType:     AvailabilityReference{ID: serviceTypeID, Active: true},
		TimeZone:        "UTC",
		BusinessHours:   []LocalBusinessHours{{OpensAt: 9 * time.Hour, ClosesAt: 12*time.Hour + 30*time.Minute}},
		ServiceDuration: duration,
		RequiredSkills:  []domain.SkillID{"routine"},
		Technicians: []domain.Technician{
			technician(t, "tech-a", true, []domain.SkillID{"routine", "diagnostic"}),
			technician(t, "tech-b", true, []domain.SkillID{"routine"}),
		},
		Bays:           []domain.Bay{bay(t, "bay-a", true), bay(t, "bay-b", true)},
		TechnicianBusy: map[domain.TechnicianID][]domain.Interval{},
		BayBusy:        map[domain.BayID][]domain.Interval{},
	}
}

func validAvailabilityQuery() AvailabilityQuery {
	return AvailabilityQuery{VehicleID: vehicleID, DealershipID: dealershipID, ServiceTypeID: serviceTypeID, Date: "2030-01-02"}
}

func technician(t *testing.T, id domain.TechnicianID, active bool, skills []domain.SkillID) domain.Technician {
	t.Helper()
	value, err := domain.NewTechnician(id, active, skills, 0)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func bay(t *testing.T, id domain.BayID, active bool) domain.Bay {
	t.Helper()
	value, err := domain.NewBay(id, active, 0)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func interval(t *testing.T, start, end string) domain.Interval {
	t.Helper()
	startAt, err := time.Parse(time.RFC3339, start)
	if err != nil {
		t.Fatal(err)
	}
	endAt, err := time.Parse(time.RFC3339, end)
	if err != nil {
		t.Fatal(err)
	}
	value, err := domain.NewInterval(startAt, endAt)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func slots(t *testing.T, ctx AvailabilityContext) []domain.Interval {
	t.Helper()
	repo := &countingAvailabilityRepository{context: ctx}
	got, err := NewAvailabilityService(repo).AvailableSlots(context.Background(), validAvailabilityQuery())
	if err != nil {
		t.Fatal(err)
	}
	if repo.calls != 1 {
		t.Fatalf("repository calls = %d, want 1", repo.calls)
	}
	return got
}

func assertNoSlots(t *testing.T, ctx AvailabilityContext) {
	t.Helper()
	if got := slots(t, ctx); len(got) != 0 {
		t.Fatalf("slots = %v, want none", got)
	}
}

func assertContainsSlot(t *testing.T, ctx AvailabilityContext, want string) {
	t.Helper()
	for _, slot := range slots(t, ctx) {
		if slot.Start().Format(time.RFC3339) == want {
			return
		}
	}
	t.Fatalf("slot %s not found", want)
}

func assertMissingSlot(t *testing.T, ctx AvailabilityContext, unwanted string) {
	t.Helper()
	for _, slot := range slots(t, ctx) {
		if slot.Start().Format(time.RFC3339) == unwanted {
			t.Fatalf("unexpected slot %s found", unwanted)
		}
	}
}

func assertSlotStarts(t *testing.T, got []domain.Interval, want ...string) {
	t.Helper()
	starts := make([]string, len(got))
	for index, slot := range got {
		starts[index] = slot.Start().Format(time.RFC3339)
	}
	if !reflect.DeepEqual(starts, want) {
		t.Fatalf("slot starts = %v, want %v", starts, want)
	}
}
