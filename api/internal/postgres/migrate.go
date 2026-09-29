package postgres

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const migrationLockKey int64 = 0x7363686564756c65

// Migrate applies SQL files in lexical order, once each, under a database-wide lock.
func Migrate(ctx context.Context, pool *pgxpool.Pool, migrations fs.FS) (resultErr error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "select pg_advisory_lock($1)", migrationLockKey); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var unlocked bool
		if err := conn.QueryRow(unlockCtx, "select pg_advisory_unlock($1)", migrationLockKey).Scan(&unlocked); err != nil {
			_ = conn.Conn().Close(context.Background())
			resultErr = errors.Join(resultErr, fmt.Errorf("release migration lock: %w", err))
		} else if !unlocked {
			resultErr = errors.Join(resultErr, errors.New("migration advisory lock was not held"))
		}
	}()

	if _, err := conn.Exec(ctx, `create table if not exists schema_migrations (
		name text primary key,
		applied_at timestamptz not null default now()
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrations, ".")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		if err := applyMigration(ctx, conn, migrations, entry.Name()); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(ctx context.Context, conn *pgxpool.Conn, migrations fs.FS, name string) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin %s: %w", name, err)
	}
	defer tx.Rollback(context.Background())

	var applied bool
	if err := tx.QueryRow(ctx, "select exists(select 1 from schema_migrations where name = $1)", name).Scan(&applied); err != nil {
		return fmt.Errorf("check %s: %w", name, err)
	}
	if applied {
		return nil
	}
	content, err := fs.ReadFile(migrations, name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	if _, err := tx.Exec(ctx, string(content), pgx.QueryExecModeSimpleProtocol); err != nil {
		return fmt.Errorf("apply %s: %w", name, err)
	}
	if _, err := tx.Exec(ctx, "insert into schema_migrations (name) values ($1)", name); err != nil {
		return fmt.Errorf("record %s: %w", name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit %s: %w", name, err)
	}
	return nil
}
