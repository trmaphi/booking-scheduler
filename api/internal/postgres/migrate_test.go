package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func freshMigrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("migration_test_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "create schema "+identifier); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		if _, err := admin.Exec(ctx, "drop schema "+identifier+" cascade"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
		admin.Close()
	})
	return pool
}

func TestMigrateAppliesFilesInLexicalOrder(t *testing.T) {
	pool := freshMigrationPool(t)
	migrations := fstest.MapFS{
		"002_second.sql": &fstest.MapFile{Data: []byte("insert into migration_order (step) values (2)")},
		"001_first.sql":  &fstest.MapFile{Data: []byte("create table migration_order (step integer); insert into migration_order (step) values (1)")},
		"README.txt":     &fstest.MapFile{Data: []byte("ignored")},
	}
	if err := Migrate(context.Background(), pool, migrations); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(context.Background(), "select step from migration_order order by step")
	if err != nil {
		t.Fatal(err)
	}
	steps, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 || steps[0] != 1 || steps[1] != 2 {
		t.Fatalf("migration steps = %v, want [1 2]", steps)
	}
	var names []string
	rows, err = pool.Query(context.Background(), "select name from schema_migrations order by name")
	if err != nil {
		t.Fatal(err)
	}
	names, err = pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "001_first.sql" || names[1] != "002_second.sql" {
		t.Fatalf("recorded migrations = %v", names)
	}
}

func TestMigrateSkipsAppliedFiles(t *testing.T) {
	pool := freshMigrationPool(t)
	first := fstest.MapFS{"001_once.sql": &fstest.MapFile{Data: []byte("create table migrated_once (id integer)")}}
	if err := Migrate(context.Background(), pool, first); err != nil {
		t.Fatal(err)
	}
	changed := fstest.MapFS{"001_once.sql": &fstest.MapFile{Data: []byte("this is not valid SQL")}}
	if err := Migrate(context.Background(), pool, changed); err != nil {
		t.Fatalf("already applied file ran again: %v", err)
	}
	var count int
	if err := pool.QueryRow(context.Background(), "select count(*) from schema_migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("recorded migrations = %d, want 1", count)
	}
}

func TestMigrateRollsBackFailedFile(t *testing.T) {
	pool := freshMigrationPool(t)
	migrations := fstest.MapFS{
		"001_good.sql": &fstest.MapFile{Data: []byte("create table committed_table (id integer)")},
		"002_bad.sql":  &fstest.MapFile{Data: []byte("create table rolled_back_table (id integer); select missing_column")},
	}
	if err := Migrate(context.Background(), pool, migrations); err == nil {
		t.Fatal("expected failing migration error")
	}
	var goodExists, badExists bool
	if err := pool.QueryRow(context.Background(), "select to_regclass('committed_table') is not null, to_regclass('rolled_back_table') is not null").Scan(&goodExists, &badExists); err != nil {
		t.Fatal(err)
	}
	if !goodExists || badExists {
		t.Fatalf("tables after rollback: committed=%v rolled_back=%v", goodExists, badExists)
	}
	var name string
	if err := pool.QueryRow(context.Background(), "select name from schema_migrations").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "001_good.sql" {
		t.Fatalf("recorded migration = %q, want 001_good.sql", name)
	}
}
