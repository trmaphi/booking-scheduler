package domain

import (
	"errors"
	"sort"
	"strings"
)

type SkillID string
type TechnicianID string
type BayID string

// Technician exposes an immutable scheduling projection. The private marker
// keeps construction behind NewTechnician so invalid zero-valued resources
// cannot cross the domain boundary.
type Technician interface {
	ID() TechnicianID
	Active() bool
	Skills() []SkillID
	FutureLoad() int
	QualifiedFor(required []SkillID) bool
	domainTechnician()
}

type technician struct {
	id         TechnicianID
	active     bool
	skills     map[SkillID]struct{}
	futureLoad int
}

func NewTechnician(id TechnicianID, active bool, skills []SkillID, futureLoad int) (Technician, error) {
	if strings.TrimSpace(string(id)) == "" {
		return nil, errors.New("technician ID is required")
	}
	if futureLoad < 0 {
		return nil, errors.New("technician future load cannot be negative")
	}
	skillSet := make(map[SkillID]struct{}, len(skills))
	for _, skill := range skills {
		if strings.TrimSpace(string(skill)) == "" {
			return nil, errors.New("skill ID is required")
		}
		skillSet[skill] = struct{}{}
	}
	return technician{id: id, active: active, skills: skillSet, futureLoad: futureLoad}, nil
}

func (t technician) ID() TechnicianID { return t.id }
func (t technician) Active() bool     { return t.active }
func (t technician) FutureLoad() int  { return t.futureLoad }
func (technician) domainTechnician()  {}

func (t technician) Skills() []SkillID {
	skills := make([]SkillID, 0, len(t.skills))
	for skill := range t.skills {
		skills = append(skills, skill)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i] < skills[j] })
	return skills
}

func (t technician) QualifiedFor(required []SkillID) bool {
	if !t.active || len(required) == 0 {
		return false
	}
	for _, skill := range required {
		if strings.TrimSpace(string(skill)) == "" {
			return false
		}
		if _, exists := t.skills[skill]; !exists {
			return false
		}
	}
	return true
}

// EligibleTechnicians validates the service requirements and returns only
// active technicians who possess every required skill.
func EligibleTechnicians(candidates []Technician, required []SkillID) ([]Technician, error) {
	if len(required) == 0 {
		return nil, errors.New("at least one required skill is needed")
	}
	for _, skill := range required {
		if strings.TrimSpace(string(skill)) == "" {
			return nil, errors.New("required skill ID is invalid")
		}
	}
	eligible := make([]Technician, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate != nil && candidate.QualifiedFor(required) {
			eligible = append(eligible, candidate)
		}
	}
	return eligible, nil
}

// OrderTechnicians returns a sorted copy, preserving the caller's slice.
func OrderTechnicians(candidates []Technician) []Technician {
	ordered := append([]Technician(nil), candidates...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].FutureLoad() != ordered[j].FutureLoad() {
			return ordered[i].FutureLoad() < ordered[j].FutureLoad()
		}
		return ordered[i].ID() < ordered[j].ID()
	})
	return ordered
}

// Bay exposes an immutable scheduling projection.
type Bay interface {
	ID() BayID
	Active() bool
	FutureLoad() int
	domainBay()
}

type bay struct {
	id         BayID
	active     bool
	futureLoad int
}

func NewBay(id BayID, active bool, futureLoad int) (Bay, error) {
	if strings.TrimSpace(string(id)) == "" {
		return nil, errors.New("bay ID is required")
	}
	if futureLoad < 0 {
		return nil, errors.New("bay future load cannot be negative")
	}
	return bay{id: id, active: active, futureLoad: futureLoad}, nil
}

func (b bay) ID() BayID       { return b.id }
func (b bay) Active() bool    { return b.active }
func (b bay) FutureLoad() int { return b.futureLoad }
func (bay) domainBay()        {}

func ActiveBays(candidates []Bay) []Bay {
	active := make([]Bay, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate != nil && candidate.Active() {
			active = append(active, candidate)
		}
	}
	return active
}

// OrderBays returns a sorted copy, preserving the caller's slice.
func OrderBays(candidates []Bay) []Bay {
	ordered := append([]Bay(nil), candidates...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].FutureLoad() != ordered[j].FutureLoad() {
			return ordered[i].FutureLoad() < ordered[j].FutureLoad()
		}
		return ordered[i].ID() < ordered[j].ID()
	})
	return ordered
}
