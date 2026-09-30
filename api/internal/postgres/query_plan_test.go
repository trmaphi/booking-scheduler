package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestQueryPlanUsesBoundedAppointmentIntervalAccess(t *testing.T) {
	tx := appointmentConstraintTx(t)
	ctx := context.Background()
	start := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	for index := 0; index < 600; index++ {
		appointmentStart := start.Add(time.Duration(index) * time.Hour)
		insertConstraintAppointment(t, tx, constraintTechnicianID, constraintBayID, "CONFIRMED", appointmentStart, appointmentStart.Add(time.Hour))
	}
	if _, err := tx.Exec(ctx, "set local enable_seqscan = off"); err != nil {
		t.Fatal(err)
	}

	plan := explainJSON(t, tx, `
		select id
		from appointments
		where dealership_id = $1
		  and status = 'CONFIRMED'
		  and tstzrange(start_at, end_at, '[)') && tstzrange($2::timestamptz, $3::timestamptz, '[)')
	`, constraintDealershipID, start.Add(300*time.Hour), start.Add(301*time.Hour))
	assertPlanUsesBoundedIndexAccess(t, plan, "appointments", "tstzrange")
}

func TestQueryPlanUsesQualificationAndActiveResourceIndexes(t *testing.T) {
	tx := appointmentConstraintTx(t)
	ctx := context.Background()
	const skillID = "10000000-0000-0000-0000-000000000009"
	if _, err := tx.Exec(ctx, `insert into skills (id, name) values ($1, 'Query Plan Skill')`, skillID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `insert into technician_skills (technician_id, skill_id) values ($1, $2)`, constraintTechnicianID, skillID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `insert into service_type_required_skills (service_type_id, skill_id) values ($1, $2)`, constraintServiceTypeID, skillID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "set local enable_seqscan = off"); err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		name            string
		query           string
		args            []any
		indexName       string
		boundedRelation string
	}{
		{
			name:      "technician qualification",
			query:     `select technician_id from technician_skills where skill_id = $1`,
			args:      []any{skillID},
			indexName: "technician_skills_skill_technician_idx",
		},
		{
			name:      "service requirements",
			query:     `select skill_id from service_type_required_skills where service_type_id = $1`,
			args:      []any{constraintServiceTypeID},
			indexName: "service_type_required_skills_pkey",
		},
		{
			name:            "active technicians",
			query:           `select id from technicians where dealership_id = $1 and active`,
			args:            []any{constraintDealershipID},
			boundedRelation: "technicians",
		},
		{
			name:            "active bays",
			query:           `select id from service_bays where dealership_id = $1 and active`,
			args:            []any{constraintDealershipID},
			boundedRelation: "service_bays",
		},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			plan := explainJSON(t, tx, check.query, check.args...)
			if check.indexName != "" {
				assertPlanUsesIndex(t, plan, check.indexName)
			} else {
				assertPlanUsesBoundedIndexAccess(t, plan, check.boundedRelation, "dealership_id")
			}
		})
	}
}

// assertPlanUsesBoundedIndexAccess intentionally does not prescribe one index.
// PostgreSQL may correctly prefer a covering unique index over the narrower
// partial index as table statistics change. The contract is bounded indexed
// access on the dealership predicate, with no sequential scan of the resource.
func assertPlanUsesBoundedIndexAccess(t *testing.T, raw []byte, relation, predicate string) {
	t.Helper()
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode query plan: %v", err)
	}
	bounded, sequential := inspectRelationAccess(decoded, relation, predicate)
	if sequential || !bounded {
		t.Fatalf("plan for %s must use bounded indexed %s access without a sequential scan: %s", relation, predicate, fmt.Sprintf("%.2000s", raw))
	}
}

func inspectRelationAccess(value any, relation, predicate string) (bounded, sequential bool) {
	switch typed := value.(type) {
	case map[string]any:
		relationName, _ := typed["Relation Name"].(string)
		nodeType, _ := typed["Node Type"].(string)
		if relationName == relation {
			if nodeType == "Seq Scan" {
				sequential = true
			}
			condition, _ := typed["Index Cond"].(string)
			if condition == "" {
				condition, _ = typed["Recheck Cond"].(string)
			}
			if strings.Contains(nodeType, "Index") || nodeType == "Bitmap Heap Scan" {
				bounded = strings.Contains(condition, predicate)
			}
		}
		for _, nested := range typed {
			childBounded, childSequential := inspectRelationAccess(nested, relation, predicate)
			bounded = bounded || childBounded
			sequential = sequential || childSequential
		}
	case []any:
		for _, nested := range typed {
			childBounded, childSequential := inspectRelationAccess(nested, relation, predicate)
			bounded = bounded || childBounded
			sequential = sequential || childSequential
		}
	}
	return bounded, sequential
}

func explainJSON(t *testing.T, queryer pgx.Tx, query string, args ...any) []byte {
	t.Helper()
	var raw []byte
	if err := queryer.QueryRow(context.Background(), "explain (format json) "+query, args...).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertPlanUsesIndex(t *testing.T, raw []byte, expected string) {
	t.Helper()
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode query plan: %v", err)
	}
	var indexes []string
	collectPlanIndexes(decoded, &indexes)
	for _, index := range indexes {
		if index == expected {
			return
		}
	}
	t.Fatalf("query plan indexes = %v, want %q; plan: %s", indexes, expected, fmt.Sprintf("%.2000s", raw))
}

func collectPlanIndexes(value any, indexes *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if key == "Index Name" {
				if name, ok := nested.(string); ok {
					*indexes = append(*indexes, name)
				}
			}
			collectPlanIndexes(nested, indexes)
		}
	case []any:
		for _, nested := range typed {
			collectPlanIndexes(nested, indexes)
		}
	}
}
