package favorites

import (
	"context"
	"database/sql"
	"fmt"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(ctx context.Context, db *sql.DB) (*PostgresStore, error) {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS favorites (
			id     SERIAL PRIMARY KEY,
			source TEXT NOT NULL,
			title  TEXT NOT NULL,
			url    TEXT NOT NULL UNIQUE,
			reason TEXT NOT NULL DEFAULT ''
		)`)
	if err != nil {
		return nil, fmt.Errorf("favorites: create table: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) Add(ctx context.Context, f Favorite) (Favorite, bool, error) {
	// ON CONFLICT DO NOTHING + RETURNING yields no row when the url already exists;
	// fall back to selecting the existing snapshot so the original row is returned.
	var saved Favorite
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO favorites (source, title, url, reason)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (url) DO NOTHING
		RETURNING id, source, title, url, reason`,
		f.Source, f.Title, f.URL, f.Reason,
	).Scan(&saved.ID, &saved.Source, &saved.Title, &saved.URL, &saved.Reason)
	if err == nil {
		return saved, true, nil
	}
	if err != sql.ErrNoRows {
		return Favorite{}, false, err
	}
	err = s.db.QueryRowContext(ctx, `
		SELECT id, source, title, url, reason FROM favorites WHERE url = $1`, f.URL,
	).Scan(&saved.ID, &saved.Source, &saved.Title, &saved.URL, &saved.Reason)
	if err != nil {
		return Favorite{}, false, err
	}
	return saved, false, nil
}

func (s *PostgresStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM favorites WHERE id = $1`, id)
	return err
}

func (s *PostgresStore) List(ctx context.Context, offset, limit int) ([]Favorite, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source, title, url, reason FROM favorites
		ORDER BY id DESC OFFSET $1 LIMIT $2`, offset, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Favorite{}
	for rows.Next() {
		var f Favorite
		if err := rows.Scan(&f.ID, &f.Source, &f.Title, &f.URL, &f.Reason); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *PostgresStore) Count(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM favorites`).Scan(&n)
	return n, err
}

func (s *PostgresStore) URLs(ctx context.Context) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, url FROM favorites`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var (
			id  int64
			url string
		)
		if err := rows.Scan(&id, &url); err != nil {
			return nil, err
		}
		out[url] = id
	}
	return out, rows.Err()
}
