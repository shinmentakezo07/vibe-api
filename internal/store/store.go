// Package store persists the provider key and usage history in PostgreSQL.
package store

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vibe-api/internal/usage"
)

const apiKeySetting = "mistral_api_key"

const schema = `
CREATE TABLE IF NOT EXISTS settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS usage_records (
    id                  BIGSERIAL PRIMARY KEY,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    route               TEXT NOT NULL,
    model               TEXT NOT NULL,
    prompt_tokens       INTEGER NOT NULL,
    completion_tokens   INTEGER NOT NULL,
    total_tokens        INTEGER NOT NULL,
    cached_input_tokens INTEGER NOT NULL DEFAULT 0,
    stream              BOOLEAN NOT NULL
);

CREATE INDEX IF NOT EXISTS usage_records_created_at_idx
    ON usage_records (created_at DESC);
`

// Postgres is the durable settings and usage store.
type Postgres struct {
	pool  *pgxpool.Pool
	codec *codec
}

// Open connects, verifies the database, and applies the schema.
func Open(ctx context.Context, databaseURL, secret string, log *slog.Logger) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.MaxConns = 5
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate postgres: %w", err)
	}
	c, err := newCodec(secret)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("init key encryption: %w", err)
	}
	if secret == "" {
		log.Warn("APP_SECRET is not set; the stored API key is unencrypted")
	}
	return &Postgres{pool: pool, codec: c}, nil
}

// Close releases pooled connections.
func (p *Postgres) Close() {
	p.pool.Close()
}

// LoadAPIKey returns the stored provider key, or "" when none exists.
func (p *Postgres) LoadAPIKey(ctx context.Context) (string, error) {
	var value string
	err := p.pool.QueryRow(ctx, `SELECT value FROM settings WHERE key = $1`, apiKeySetting).Scan(&value)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load API key: %w", err)
	}
	return p.codec.open(value)
}

// SaveAPIKey stores the provider key, encrypted when APP_SECRET is set.
func (p *Postgres) SaveAPIKey(ctx context.Context, key string) error {
	value, err := p.codec.seal(key)
	if err != nil {
		return fmt.Errorf("encrypt API key: %w", err)
	}
	_, err = p.pool.Exec(ctx, `
		INSERT INTO settings (key, value, updated_at) VALUES ($1, $2, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()
	`, apiKeySetting, value)
	if err != nil {
		return fmt.Errorf("save API key: %w", err)
	}
	return nil
}

// ClearAPIKey removes the stored provider key.
func (p *Postgres) ClearAPIKey(ctx context.Context) error {
	if _, err := p.pool.Exec(ctx, `DELETE FROM settings WHERE key = $1`, apiKeySetting); err != nil {
		return fmt.Errorf("clear API key: %w", err)
	}
	return nil
}

// AddUsage appends one completion record.
func (p *Postgres) AddUsage(ctx context.Context, r usage.Record) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO usage_records
			(created_at, route, model, prompt_tokens, completion_tokens, total_tokens, cached_input_tokens, stream)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, r.Time, r.Route, r.Model, r.Prompt, r.Completion, r.Total, r.CachedInput, r.Stream)
	if err != nil {
		return fmt.Errorf("insert usage: %w", err)
	}
	return nil
}

// Usage aggregates every stored record and returns the newest ones first.
func (p *Postgres) Usage(ctx context.Context, limit int) (usage.Snapshot, error) {
	var snapshot usage.Snapshot

	err := p.pool.QueryRow(ctx, `
		SELECT count(*),
		       COALESCE(sum(prompt_tokens), 0),
		       COALESCE(sum(completion_tokens), 0),
		       COALESCE(sum(total_tokens), 0),
		       COALESCE(min(created_at), now())
		FROM usage_records
	`).Scan(
		&snapshot.Totals.Requests,
		&snapshot.Totals.Prompt,
		&snapshot.Totals.Completion,
		&snapshot.Totals.Total,
		&snapshot.Since,
	)
	if err != nil {
		return usage.Snapshot{}, fmt.Errorf("aggregate usage: %w", err)
	}

	rows, err := p.pool.Query(ctx, `
		SELECT model, count(*),
		       COALESCE(sum(prompt_tokens), 0),
		       COALESCE(sum(completion_tokens), 0),
		       COALESCE(sum(total_tokens), 0)
		FROM usage_records
		GROUP BY model
		ORDER BY count(*) DESC, model
	`)
	if err != nil {
		return usage.Snapshot{}, fmt.Errorf("group usage: %w", err)
	}
	snapshot.ByModel = make([]usage.ModelTotals, 0)
	for rows.Next() {
		var m usage.ModelTotals
		if err := rows.Scan(&m.Model, &m.Totals.Requests, &m.Totals.Prompt, &m.Totals.Completion, &m.Totals.Total); err != nil {
			rows.Close()
			return usage.Snapshot{}, fmt.Errorf("scan model usage: %w", err)
		}
		snapshot.ByModel = append(snapshot.ByModel, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return usage.Snapshot{}, fmt.Errorf("read model usage: %w", err)
	}

	recent, err := p.pool.Query(ctx, `
		SELECT id, created_at, route, model,
		       prompt_tokens, completion_tokens, total_tokens,
		       cached_input_tokens, stream
		FROM usage_records
		ORDER BY created_at DESC, id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return usage.Snapshot{}, fmt.Errorf("query recent usage: %w", err)
	}
	snapshot.Recent = make([]usage.Record, 0, limit)
	for recent.Next() {
		var (
			id  int64
			rec usage.Record
		)
		if err := recent.Scan(
			&id, &rec.Time, &rec.Route, &rec.Model,
			&rec.Prompt, &rec.Completion, &rec.Total,
			&rec.CachedInput, &rec.Stream,
		); err != nil {
			recent.Close()
			return usage.Snapshot{}, fmt.Errorf("scan recent usage: %w", err)
		}
		rec.ID = fmt.Sprintf("db_%06d", id)
		snapshot.Recent = append(snapshot.Recent, rec)
	}
	recent.Close()
	if err := recent.Err(); err != nil {
		return usage.Snapshot{}, fmt.Errorf("read recent usage: %w", err)
	}
	if len(snapshot.Recent) > 0 {
		last := snapshot.Recent[0]
		snapshot.LastEvent = &last
	}
	return snapshot, nil
}
