package digest

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(ctx context.Context, db *sql.DB) (*PostgresStore, error) {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS digests (
			date  DATE PRIMARY KEY,
			picks JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return nil, fmt.Errorf("digest: create table: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) Save(ctx context.Context, d Digest) error {
	picks, err := json.Marshal(d.Picks)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO digests (date, picks, created_at)
		VALUES ($1, $2, now())
		ON CONFLICT (date) DO UPDATE SET picks = EXCLUDED.picks, created_at = now()`,
		d.Date.Format("2006-01-02"), picks)
	return err
}

func (s *PostgresStore) Latest(ctx context.Context) (Digest, error) {
	var (
		date  time.Time
		picks []byte
	)
	err := s.db.QueryRowContext(ctx, `SELECT date, picks FROM digests ORDER BY date DESC LIMIT 1`).Scan(&date, &picks)
	if errors.Is(err, sql.ErrNoRows) {
		return Digest{}, ErrNoDigest
	}
	if err != nil {
		return Digest{}, err
	}
	d := Digest{Date: time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.Local), Picks: []Pick{}}
	if err := json.Unmarshal(picks, &d.Picks); err != nil {
		return Digest{}, err
	}
	return d, nil
}
