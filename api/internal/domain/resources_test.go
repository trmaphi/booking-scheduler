package domain

import (
	"reflect"
	"testing"
)

func TestTechnicianQualificationRequiresEverySkill(t *testing.T) {
	technician := mustTechnician(t, "tech-a", true, []SkillID{"oil", "electrical"}, 0)
	if !technician.QualifiedFor([]SkillID{"oil", "electrical"}) {
		t.Fatal("technician with every required skill was not qualified")
	}
	if technician.QualifiedFor([]SkillID{"oil", "electrical", "diagnostics"}) {
		t.Fatal("technician missing one required skill was qualified")
	}
	if technician.QualifiedFor(nil) {
		t.Fatal("technician was qualified for an invalid empty requirement set")
	}
}

func TestEligibleTechniciansRejectsEmptyRequiredSkills(t *testing.T) {
	technicians := []Technician{mustTechnician(t, "tech-a", true, []SkillID{"oil"}, 0)}
	if _, err := EligibleTechnicians(technicians, nil); err == nil {
		t.Fatal("EligibleTechnicians() accepted zero required skills")
	}
}

func TestResourceFilteringExcludesInactiveResources(t *testing.T) {
	active := mustTechnician(t, "tech-active", true, []SkillID{"oil"}, 2)
	inactive := mustTechnician(t, "tech-inactive", false, []SkillID{"oil"}, 0)
	unqualified := mustTechnician(t, "tech-unqualified", true, []SkillID{"electrical"}, 0)

	gotTechnicians, err := EligibleTechnicians([]Technician{inactive, unqualified, active}, []SkillID{"oil"})
	if err != nil {
		t.Fatal(err)
	}
	if got := technicianIDs(gotTechnicians); !reflect.DeepEqual(got, []TechnicianID{"tech-active"}) {
		t.Fatalf("EligibleTechnicians() IDs = %v, want [tech-active]", got)
	}

	activeBay := mustBay(t, "bay-active", true, 1)
	inactiveBay := mustBay(t, "bay-inactive", false, 0)
	if got := bayIDs(ActiveBays([]Bay{inactiveBay, activeBay})); !reflect.DeepEqual(got, []BayID{"bay-active"}) {
		t.Fatalf("ActiveBays() IDs = %v, want [bay-active]", got)
	}
}

func TestOrderTechniciansUsesFutureLoadThenStableID(t *testing.T) {
	a := mustTechnician(t, "tech-a", true, []SkillID{"oil"}, 2)
	b := mustTechnician(t, "tech-b", true, []SkillID{"oil"}, 1)
	c := mustTechnician(t, "tech-c", true, []SkillID{"oil"}, 1)
	original := []Technician{c, a, b}

	got := OrderTechnicians(original)
	if ids := technicianIDs(got); !reflect.DeepEqual(ids, []TechnicianID{"tech-b", "tech-c", "tech-a"}) {
		t.Fatalf("OrderTechnicians() IDs = %v", ids)
	}
	if ids := technicianIDs(original); !reflect.DeepEqual(ids, []TechnicianID{"tech-c", "tech-a", "tech-b"}) {
		t.Fatalf("OrderTechnicians() mutated caller slice: %v", ids)
	}

	reversed := []Technician{a, b, c}
	if ids := technicianIDs(OrderTechnicians(reversed)); !reflect.DeepEqual(ids, technicianIDs(got)) {
		t.Fatalf("different input order produced %v, want %v", ids, technicianIDs(got))
	}
}

func TestOrderBaysUsesFutureLoadThenStableID(t *testing.T) {
	a := mustBay(t, "bay-a", true, 3)
	b := mustBay(t, "bay-b", true, 1)
	c := mustBay(t, "bay-c", true, 1)
	original := []Bay{c, a, b}

	got := OrderBays(original)
	if ids := bayIDs(got); !reflect.DeepEqual(ids, []BayID{"bay-b", "bay-c", "bay-a"}) {
		t.Fatalf("OrderBays() IDs = %v", ids)
	}
	if ids := bayIDs(original); !reflect.DeepEqual(ids, []BayID{"bay-c", "bay-a", "bay-b"}) {
		t.Fatalf("OrderBays() mutated caller slice: %v", ids)
	}

	reversed := []Bay{a, b, c}
	if ids := bayIDs(OrderBays(reversed)); !reflect.DeepEqual(ids, bayIDs(got)) {
		t.Fatalf("different input order produced %v, want %v", ids, bayIDs(got))
	}
}

func TestResourceConstructionRejectsInvalidValues(t *testing.T) {
	if _, err := NewTechnician("", true, []SkillID{"oil"}, 0); err == nil {
		t.Fatal("empty technician ID accepted")
	}
	if _, err := NewTechnician("tech-a", true, []SkillID{""}, 0); err == nil {
		t.Fatal("empty skill ID accepted")
	}
	if _, err := NewTechnician("tech-a", true, []SkillID{"oil"}, -1); err == nil {
		t.Fatal("negative technician future load accepted")
	}
	if _, err := NewBay("", true, 0); err == nil {
		t.Fatal("empty bay ID accepted")
	}
	if _, err := NewBay("bay-a", true, -1); err == nil {
		t.Fatal("negative bay future load accepted")
	}
}

func mustTechnician(t *testing.T, id TechnicianID, active bool, skills []SkillID, futureLoad int) Technician {
	t.Helper()
	technician, err := NewTechnician(id, active, skills, futureLoad)
	if err != nil {
		t.Fatal(err)
	}
	return technician
}

func mustBay(t *testing.T, id BayID, active bool, futureLoad int) Bay {
	t.Helper()
	bay, err := NewBay(id, active, futureLoad)
	if err != nil {
		t.Fatal(err)
	}
	return bay
}

func technicianIDs(technicians []Technician) []TechnicianID {
	ids := make([]TechnicianID, len(technicians))
	for index, technician := range technicians {
		ids[index] = technician.ID()
	}
	return ids
}

func bayIDs(bays []Bay) []BayID {
	ids := make([]BayID, len(bays))
	for index, bay := range bays {
		ids[index] = bay.ID()
	}
	return ids
}
