package postgres

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func schemaPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestSchemaHasRequiredExtensionAndTables(t *testing.T) {
	pool := schemaPool(t)
	ctx := context.Background()
	var extension string
	if err := pool.QueryRow(ctx, "select extname from pg_extension where extname = 'btree_gist'").Scan(&extension); err != nil {
		t.Fatalf("btree_gist extension missing: %v", err)
	}
	tables := map[string]bool{}
	rows, err := pool.Query(ctx, "select table_name from information_schema.tables where table_schema = 'public'")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	for _, name := range []string{"appointments", "customers", "dealership_business_hours", "dealerships", "idempotency_records", "service_bays", "service_type_required_skills", "service_types", "skills", "technician_skills", "technicians", "vehicles"} {
		if !tables[name] {
			t.Errorf("required table %s missing", name)
		}
	}
}

func TestSchemaHasOwnershipForeignKeys(t *testing.T) {
	definitions := schemaConstraintDefinitions(t, 'f')
	for _, pattern := range []string{
		`^vehicles: FOREIGN KEY \(customer_id\)`,
		`^technicians: FOREIGN KEY \(dealership_id\)`,
		`^service_bays: FOREIGN KEY \(dealership_id\)`,
		`^appointments: FOREIGN KEY \(vehicle_id\)`,
		`^appointments: FOREIGN KEY \(technician_id\)`,
		`^appointments: FOREIGN KEY \(service_bay_id\)`,
	} {
		assertDefinition(t, definitions, pattern)
	}
}

func TestSchemaProtectsDurationsHoursAndStatus(t *testing.T) {
	pool := schemaPool(t)
	rows, err := pool.Query(context.Background(), `
		select table_name, column_name, is_nullable, column_default
		from information_schema.columns
		where table_schema = 'public' and column_name = 'active'
	`)
	if err != nil {
		t.Fatal(err)
	}
	active := map[string]bool{}
	for rows.Next() {
		var table, column, nullable string
		var defaultValue *string
		if err := rows.Scan(&table, &column, &nullable, &defaultValue); err != nil {
			t.Fatal(err)
		}
		active[table] = column == "active" && nullable == "NO" && defaultValue != nil && *defaultValue == "true"
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	for _, table := range []string{"dealerships", "service_bays", "service_types", "technicians"} {
		if !active[table] {
			t.Errorf("%s.active must be non-null with true default", table)
		}
	}
	definitions := schemaConstraintDefinitions(t, 'c')
	for _, pattern := range []string{
		`^service_types: CHECK .*duration_minutes > 0`,
		`^dealership_business_hours: CHECK .*day_of_week.*0.*6`,
		`^dealership_business_hours: CHECK .*opens_at < closes_at`,
		`^appointments: CHECK .*status.*CONFIRMED.*CANCELLED`,
		`^appointments: CHECK .*start_at < end_at`,
	} {
		assertDefinition(t, definitions, pattern)
	}
}

func TestSchemaStoresIdempotencyHashes(t *testing.T) {
	pool := schemaPool(t)
	ctx := context.Background()
	rows, err := pool.Query(ctx, `
		select column_name, is_nullable
		from information_schema.columns
		where table_schema = 'public' and table_name = 'idempotency_records'
	`)
	if err != nil {
		t.Fatal(err)
	}
	columns := map[string]string{}
	for rows.Next() {
		var name, nullable string
		if err := rows.Scan(&name, &nullable); err != nil {
			t.Fatal(err)
		}
		columns[name] = nullable
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	for name, nullable := range map[string]string{"idempotency_key": "NO", "request_hash": "NO", "appointment_id": "YES"} {
		if columns[name] != nullable {
			t.Errorf("%s nullable = %q, want %q", name, columns[name], nullable)
		}
	}
	var primaryKey string
	if err := pool.QueryRow(ctx, `
		select pg_get_constraintdef(oid) from pg_constraint
		where conrelid = 'idempotency_records'::regclass and contype = 'p'
	`).Scan(&primaryKey); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(primaryKey, "idempotency_key") {
		t.Errorf("primary key = %q, want idempotency_key", primaryKey)
	}
}

func TestSchemaHasBookingOverlapConstraintsAndIndexes(t *testing.T) {
	pool := schemaPool(t)
	ctx := context.Background()

	rows, err := pool.Query(ctx, `
		select conname, pg_get_constraintdef(oid)
		from pg_constraint
		where conrelid = 'appointments'::regclass and contype = 'x'
	`)
	if err != nil {
		t.Fatal(err)
	}
	constraints := map[string]string{}
	for rows.Next() {
		var name, definition string
		if err := rows.Scan(&name, &definition); err != nil {
			t.Fatal(err)
		}
		constraints[name] = definition
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	for _, name := range []string{"appointments_technician_no_overlap", "appointments_bay_no_overlap", "appointments_vehicle_no_overlap"} {
		definition := constraints[name]
		if !strings.Contains(definition, "tstzrange(start_at, end_at, '[)'::text) WITH &&") || !strings.Contains(definition, "status = 'CONFIRMED'::text") {
			t.Errorf("constraint %s = %q, want confirmed half-open exclusion", name, definition)
		}
	}

	rows, err = pool.Query(ctx, `
		select indexname from pg_indexes
		where schemaname = current_schema()
	`)
	if err != nil {
		t.Fatal(err)
	}
	indexes := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		indexes[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	for _, name := range []string{
		"appointments_dealership_interval_idx",
		"appointments_vehicle_id_idx",
		"appointments_service_type_id_idx",
		"technicians_active_dealership_idx",
		"service_bays_active_dealership_idx",
		"technician_skills_skill_technician_idx",
		"service_required_skills_skill_service_idx",
	} {
		if !indexes[name] {
			t.Errorf("required index %s missing", name)
		}
	}
}

func schemaConstraintDefinitions(t *testing.T, constraintType rune) []string {
	t.Helper()
	pool := schemaPool(t)
	rows, err := pool.Query(context.Background(), `
		select conrelid::regclass::text, pg_get_constraintdef(oid)
		from pg_constraint where contype = $1
	`, string(constraintType))
	if err != nil {
		t.Fatal(err)
	}
	var definitions []string
	for rows.Next() {
		var relation, definition string
		if err := rows.Scan(&relation, &definition); err != nil {
			t.Fatal(err)
		}
		definitions = append(definitions, relation+": "+definition)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	return definitions
}

func assertDefinition(t *testing.T, definitions []string, pattern string) {
	t.Helper()
	match := regexp.MustCompile(pattern)
	for _, definition := range definitions {
		if match.MatchString(definition) {
			return
		}
	}
	t.Errorf("missing constraint matching %q among %v", pattern, definitions)
}
