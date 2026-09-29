package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"scheduler/api/internal/config"
	"scheduler/api/internal/postgres"
	"scheduler/api/migrations"
)

func main() {
	// The migration command needs only DATABASE_URL. Supply a local origin for
	// config validation; it is never used to serve HTTP traffic.
	cfg, err := config.Load(func(name string) string {
		if name == "ALLOWED_ORIGIN" {
			return "http://localhost:3000"
		}
		return os.Getenv(name)
	})
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("invalid DATABASE_URL")
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool, migrations.Files); err != nil {
		log.Fatal("database migration failed using DATABASE_URL")
	}
	log.Print("database migrations complete")
}
