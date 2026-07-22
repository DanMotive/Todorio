// Package db: PostgreSQL connection and migration runner.
package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct{ Pool *pgxpool.Pool }

func Connect(ctx context.Context, url string) (*DB, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connecting to DB: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("DB unreachable: %w", err)
	}
	return &DB{Pool: pool}, nil
}

// Migrate applies *.sql files from the directory in name order, each in its own transaction.
func (d *DB) Migrate(ctx context.Context, dir string) error {
	if _, err := d.Pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("migrations directory %s: %w", dir, err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for _, f := range files {
		var exists bool
		if err := d.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, f).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		sqlBytes, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			return err
		}
		tx, err := d.Pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", f, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, f); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Setting reads a value from system_settings (shared source of truth for the web UI and CLI).
func (d *DB) Setting(ctx context.Context, key string, def string) string {
	var v string
	err := d.Pool.QueryRow(ctx, `SELECT value #>> '{}' FROM system_settings WHERE key=$1`, key).Scan(&v)
	if err != nil {
		return def
	}
	return v
}

// SetSetting writes a value to system_settings (upsert).
func (d *DB) SetSetting(ctx context.Context, key string, jsonValue string) error {
	_, err := d.Pool.Exec(ctx, `INSERT INTO system_settings(key,value,updated_at) VALUES($1,$2::jsonb,now())
		ON CONFLICT (key) DO UPDATE SET value=$2::jsonb, updated_at=now()`, key, jsonValue)
	return err
}
