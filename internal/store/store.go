// Package store persists the provider key and usage history in PostgreSQL.
package store

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vibe-api/internal/keys"
	"vibe-api/internal/usage"
)

const (
	apiKeySetting     = "mistral_api_key"
	roundRobinSetting = "round_robin"
)

const schema = `
CREATE TABLE IF NOT EXISTS settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS api_keys (
    id         BIGSERIAL PRIMARY KEY,
    label      TEXT NOT NULL DEFAULT '',
    secret     TEXT NOT NULL,
    enabled    BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
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
    reasoning_tokens    INTEGER NOT NULL DEFAULT 0,
    latency_ms          INTEGER NOT NULL DEFAULT 0,
    stream              BOOLEAN NOT NULL
);

CREATE INDEX IF NOT EXISTS usage_records_created_at_idx
    ON usage_records (created_at DESC);

ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS reasoning_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS latency_ms INTEGER NOT NULL DEFAULT 0;
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

// ListAPIKeys returns every stored credential, decrypting secrets in memory.
// A credential saved by the previous single-key schema is migrated on first use.
func (p *Postgres) ListAPIKeys(ctx context.Context) ([]keys.Key, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, label, secret, enabled, created_at
		FROM api_keys ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list API keys: %w", err)
	}
	result := make([]keys.Key, 0)
	for rows.Next() {
		var (
			id     int64
			stored string
			key    keys.Key
		)
		if err := rows.Scan(&id, &key.Label, &stored, &key.Enabled, &key.CreatedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan API key: %w", err)
		}
		secret, err := p.codec.open(stored)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("read API key %d: %w", id, err)
		}
		key.ID = fmt.Sprintf("%d", id)
		key.Secret = secret
		key.Source = "database"
		result = append(result, key)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read API keys: %w", err)
	}
	if len(result) > 0 {
		return result, nil
	}
	return p.migrateLegacyKey(ctx)
}

// migrateLegacyKey imports the single key used before the multi-key schema.
func (p *Postgres) migrateLegacyKey(ctx context.Context) ([]keys.Key, error) {
	var stored string
	err := p.pool.QueryRow(ctx, `SELECT value FROM settings WHERE key = $1`, apiKeySetting).Scan(&stored)
	if err == pgx.ErrNoRows {
		return []keys.Key{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load legacy API key: %w", err)
	}
	secret, err := p.codec.open(stored)
	if err != nil {
		return nil, fmt.Errorf("read legacy API key: %w", err)
	}
	key, err := p.AddAPIKey(ctx, "Imported", secret)
	if err != nil {
		return nil, err
	}
	if _, err := p.pool.Exec(ctx, `DELETE FROM settings WHERE key = $1`, apiKeySetting); err != nil {
		return nil, fmt.Errorf("remove legacy API key: %w", err)
	}
	return []keys.Key{key}, nil
}

// AddAPIKey stores a credential and returns it with its database id.
func (p *Postgres) AddAPIKey(ctx context.Context, label, secret string) (keys.Key, error) {
	stored, err := p.codec.seal(secret)
	if err != nil {
		return keys.Key{}, fmt.Errorf("encrypt API key: %w", err)
	}
	var (
		id    int64
		key   keys.Key
		value string
	)
	err = p.pool.QueryRow(ctx, `
		INSERT INTO api_keys (label, secret) VALUES ($1, $2)
		RETURNING id, label, secret, enabled, created_at
	`, label, stored).Scan(&id, &key.Label, &value, &key.Enabled, &key.CreatedAt)
	if err != nil {
		return keys.Key{}, fmt.Errorf("save API key: %w", err)
	}
	key.ID = fmt.Sprintf("%d", id)
	key.Secret = secret
	key.Source = "database"
	return key, nil
}

// UpdateAPIKey changes the label and enabled state of a stored credential.
func (p *Postgres) UpdateAPIKey(ctx context.Context, id, label string, enabled bool) error {
	tag, err := p.pool.Exec(ctx, `UPDATE api_keys SET label = $2, enabled = $3 WHERE id = $1`, id, label, enabled)
	if err != nil {
		return fmt.Errorf("update API key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// DeleteAPIKey removes a stored credential.
func (p *Postgres) DeleteAPIKey(ctx context.Context, id string) error {
	tag, err := p.pool.Exec(ctx, `DELETE FROM api_keys WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete API key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// LoadRoundRobin reads the persisted rotation setting.
func (p *Postgres) LoadRoundRobin(ctx context.Context) (bool, error) {
	var value string
	err := p.pool.QueryRow(ctx, `SELECT value FROM settings WHERE key = $1`, roundRobinSetting).Scan(&value)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load round-robin setting: %w", err)
	}
	return value == "true", nil
}

// SaveRoundRobin persists the rotation setting.
func (p *Postgres) SaveRoundRobin(ctx context.Context, enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	_, err := p.pool.Exec(ctx, `
		INSERT INTO settings (key, value, updated_at) VALUES ($1, $2, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()
	`, roundRobinSetting, value)
	if err != nil {
		return fmt.Errorf("save round-robin setting: %w", err)
	}
	return nil
}

// AddUsage appends one completion record.
func (p *Postgres) AddUsage(ctx context.Context, r usage.Record) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO usage_records
			(created_at, route, model, prompt_tokens, completion_tokens, total_tokens, cached_input_tokens, reasoning_tokens, latency_ms, stream)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, r.Time, r.Route, r.Model, r.Prompt, r.Completion, r.Total, r.CachedInput, r.Reasoning, r.LatencyMS, r.Stream)
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
		       COALESCE(sum(cached_input_tokens), 0),
		       COALESCE(sum(reasoning_tokens), 0),
		       COALESCE(min(created_at), now())
		FROM usage_records
	`).Scan(
		&snapshot.Totals.Requests,
		&snapshot.Totals.Prompt,
		&snapshot.Totals.Completion,
		&snapshot.Totals.Total,
		&snapshot.Totals.Cached,
		&snapshot.Totals.Reasoning,
		&snapshot.Since,
	)
	if err != nil {
		return usage.Snapshot{}, fmt.Errorf("aggregate usage: %w", err)
	}

	rows, err := p.pool.Query(ctx, `
		SELECT model, count(*),
		       COALESCE(sum(prompt_tokens), 0),
		       COALESCE(sum(completion_tokens), 0),
		       COALESCE(sum(total_tokens), 0),
		       COALESCE(sum(cached_input_tokens), 0),
		       COALESCE(sum(reasoning_tokens), 0)
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
		if err := rows.Scan(&m.Model, &m.Totals.Requests, &m.Totals.Prompt, &m.Totals.Completion, &m.Totals.Total, &m.Totals.Cached, &m.Totals.Reasoning); err != nil {
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
		       cached_input_tokens, reasoning_tokens, latency_ms, stream
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
			&rec.CachedInput, &rec.Reasoning, &rec.LatencyMS, &rec.Stream,
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
