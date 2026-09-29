package postgres

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"scheduler/api/migrations"
)

const (
	seedDealershipID      = "20000000-0000-0000-0000-000000000021"
	seedRoutineServiceID  = "20000000-0000-0000-0000-000000000041"
	seedUnqualifiedTechID = "20000000-0000-0000-0000-000000000053"
	seedOccupiedApptID    = "20000000-0000-0000-0000-000000000071"
	seedOccupiedStart     = "2030-01-02 10:00:00+00"
	seedOccupiedEnd       = "2030-01-02 11:00:00+00"
)

func TestSeedProvidesBookingFixtures(t *testing.T) {
	pool := schemaPool(t)
	ctx := context.Background()

	var activeBays, qualifiedTechnicians, unqualifiedTechnicians, businessHours int
	if err := pool.QueryRow(ctx, `select count(*) from service_bays where dealership_id = $1 and active`, seedDealershipID).Scan(&activeBays); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		select count(*)
		from technicians technician
		where technician.dealership_id = $1 and technician.active
		  and not exists (
			select 1
			from service_type_required_skills required
			where required.service_type_id = $2
			  and not exists (
				select 1 from technician_skills possessed
				where possessed.technician_id = technician.id
				  and possessed.skill_id = required.skill_id
			  )
		  )
	`, seedDealershipID, seedRoutineServiceID).Scan(&qualifiedTechnicians); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		select count(*) from technicians technician
		where technician.id = $1 and technician.active
		  and not exists (select 1 from technician_skills where technician_id = technician.id)
	`, seedUnqualifiedTechID).Scan(&unqualifiedTechnicians); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `select count(*) from dealership_business_hours where dealership_id = $1`, seedDealershipID).Scan(&businessHours); err != nil {
		t.Fatal(err)
	}
	if activeBays != 2 || qualifiedTechnicians < 2 || unqualifiedTechnicians != 1 || businessHours != 7 {
		t.Fatalf("seed composition: active bays=%d qualified technicians=%d unqualified technicians=%d business hours=%d", activeBays, qualifiedTechnicians, unqualifiedTechnicians, businessHours)
	}

	inactiveFixtures := []struct {
		table string
		id    string
	}{
		{"customers", "20000000-0000-0000-0000-000000000002"},
		{"vehicles", "20000000-0000-0000-0000-000000000012"},
		{"dealerships", "20000000-0000-0000-0000-000000000022"},
		{"skills", "20000000-0000-0000-0000-000000000033"},
		{"service_types", "20000000-0000-0000-0000-000000000043"},
		{"technicians", "20000000-0000-0000-0000-000000000054"},
		{"service_bays", "20000000-0000-0000-0000-000000000063"},
	}
	for _, fixture := range inactiveFixtures {
		var count int
		query := fmt.Sprintf("select count(*) from %s where id = $1 and not active", fixture.table)
		if err := pool.QueryRow(ctx, query, fixture.id).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("inactive fixture %s/%s count = %d, want 1", fixture.table, fixture.id, count)
		}
	}

	var occupiedMatches, invalidActiveServices int
	if err := pool.QueryRow(ctx, `
		select count(*) from appointments
		where id = $1 and status = 'CONFIRMED'
		  and start_at = $2::timestamptz and end_at = $3::timestamptz
	`, seedOccupiedApptID, seedOccupiedStart, seedOccupiedEnd).Scan(&occupiedMatches); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		select count(*) from service_types service
		where service.active and not exists (
			select 1 from service_type_required_skills required where required.service_type_id = service.id
		)
	`).Scan(&invalidActiveServices); err != nil {
		t.Fatal(err)
	}
	if occupiedMatches != 1 || invalidActiveServices != 0 {
		t.Fatalf("occupied appointment matches=%d invalid active services=%d", occupiedMatches, invalidActiveServices)
	}
}

func TestSeedIsRepeatable(t *testing.T) {
	pool := schemaPool(t)
	ctx := context.Background()
	seedSQL, err := migrations.Files.ReadFile("003_seed_demo.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	if _, err := tx.Exec(ctx, string(seedSQL), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatalf("first seed application: %v", err)
	}
	first := seedFingerprint(t, tx)
	if _, err := tx.Exec(ctx, string(seedSQL), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatalf("second seed application: %v", err)
	}
	second := seedFingerprint(t, tx)
	if first != second {
		t.Fatalf("seed fingerprint changed across applications: %x != %x", first, second)
	}
}

func seedFingerprint(t *testing.T, tx pgx.Tx) [32]byte {
	t.Helper()
	rows, err := tx.Query(context.Background(), `
		select kind, id, payload
		from (
			select 'customer' kind, id::text, name || '|' || active::text payload from customers where id::text like '20000000-%'
			union all select 'vehicle', id::text, label || '|' || active::text from vehicles where id::text like '20000000-%'
			union all select 'dealership', id::text, name || '|' || active::text from dealerships where id::text like '20000000-%'
			union all select 'skill', id::text, name || '|' || active::text from skills where id::text like '20000000-%'
			union all select 'service', id::text, name || '|' || duration_minutes::text || '|' || active::text from service_types where id::text like '20000000-%'
			union all select 'technician', id::text, name || '|' || active::text from technicians where id::text like '20000000-%'
			union all select 'bay', id::text, name || '|' || active::text from service_bays where id::text like '20000000-%'
			union all select 'appointment', id::text, status || '|' || start_at::text || '|' || end_at::text from appointments where id::text like '20000000-%'
			union all select 'hours', dealership_id::text || ':' || day_of_week::text, opens_at::text || '|' || closes_at::text from dealership_business_hours where dealership_id = '20000000-0000-0000-0000-000000000021'
			union all select 'required-skill', service_type_id::text || ':' || skill_id::text, '' from service_type_required_skills where service_type_id::text like '20000000-%'
			union all select 'technician-skill', technician_id::text || ':' || skill_id::text, '' from technician_skills where technician_id::text like '20000000-%'
		) fixture
		order by kind, id
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	hash := sha256.New()
	for rows.Next() {
		var kind, id, payload string
		if err := rows.Scan(&kind, &id, &payload); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(hash, "%s\x00%s\x00%s\n", kind, id, payload)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var result [32]byte
	copy(result[:], hash.Sum(nil))
	return result
}
