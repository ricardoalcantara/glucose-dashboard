package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"glucose-dashboard/internal/model"

	_ "modernc.org/sqlite"
)

// Store is the SQLite store for minute-resolution glucose readings.
type Store struct {
	db *sql.DB
}

// Open creates the database file, enables WAL mode and a busy timeout,
// and applies the schema. path must not contain '?' or '#'.
func Open(path string) (*Store, error) {
	if path == "" || strings.ContainsAny(path, "?#\n") {
		return nil, fmt.Errorf("open database")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("open database")
	}

	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database")
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || !strings.EqualFold(mode, "wal") {
		db.Close()
		return nil, fmt.Errorf("open database")
	}
	var busy int
	if err := db.QueryRow(`PRAGMA busy_timeout`).Scan(&busy); err != nil || busy < 5000 {
		db.Close()
		return nil, fmt.Errorf("open database")
	}

	const schema = `
CREATE TABLE IF NOT EXISTS glucose (
	ts INTEGER PRIMARY KEY,
	mgdl INTEGER NOT NULL,
	source TEXT NOT NULL,
	received_at INTEGER NOT NULL
);`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("open database")
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// Upsert writes readings in order. Timestamps are truncated to the UTC minute.
// An existing minute counts as an update, including when the value is unchanged.
func (s *Store) Upsert(ctx context.Context, readings []model.Reading) (inserted, updated int, err error) {
	if len(readings) == 0 {
		return 0, 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	receivedAt := time.Now().UTC().Unix()
	for _, reading := range readings {
		ts := reading.TS.UTC().Truncate(time.Minute).Unix()
		var one int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM glucose WHERE ts = ?`, ts).Scan(&one)
		exists := err == nil
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, 0, err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO glucose (ts, mgdl, source, received_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(ts) DO UPDATE SET
	mgdl = excluded.mgdl,
	source = excluded.source,
	received_at = excluded.received_at`,
			ts, reading.Mgdl, reading.Source, receivedAt)
		if err != nil {
			return 0, 0, err
		}
		if exists {
			updated++
		} else {
			inserted++
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return inserted, updated, nil
}

// List returns readings with stored timestamps in [from, to], oldest first.
func (s *Store) List(ctx context.Context, from, to time.Time) ([]model.Reading, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT ts, mgdl, source, received_at
FROM glucose
WHERE ts >= ? AND ts <= ?
ORDER BY ts ASC`, from.UTC().Unix(), to.UTC().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Reading{}
	for rows.Next() {
		reading, err := scanReading(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, reading)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Latest returns the newest reading. ok is false when the table is empty.
func (s *Store) Latest(ctx context.Context) (model.Reading, bool, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT ts, mgdl, source, received_at
FROM glucose
ORDER BY ts DESC
LIMIT 1`)
	var ts, received int64
	var reading model.Reading
	err := row.Scan(&ts, &reading.Mgdl, &reading.Source, &received)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Reading{}, false, nil
	}
	if err != nil {
		return model.Reading{}, false, err
	}
	reading.TS = time.Unix(ts, 0).UTC()
	reading.ReceivedAt = time.Unix(received, 0).UTC()
	return reading, true, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanReading(row scanner) (model.Reading, error) {
	var ts, received int64
	var reading model.Reading
	if err := row.Scan(&ts, &reading.Mgdl, &reading.Source, &received); err != nil {
		return model.Reading{}, err
	}
	reading.TS = time.Unix(ts, 0).UTC()
	reading.ReceivedAt = time.Unix(received, 0).UTC()
	return reading, nil
}
