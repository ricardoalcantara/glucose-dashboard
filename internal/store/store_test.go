package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"glucose-dashboard/internal/model"
)

func TestOpenEnablesWALAndBusyTimeout(t *testing.T) {
	s := openTest(t)
	var mode string
	if err := s.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(mode, "wal") {
		t.Fatalf("journal_mode = %q", mode)
	}
	var busy int
	if err := s.db.QueryRow(`PRAGMA busy_timeout`).Scan(&busy); err != nil {
		t.Fatal(err)
	}
	if busy < 5000 {
		t.Fatalf("busy_timeout = %d", busy)
	}
	if s.db.Stats().MaxOpenConnections != 1 {
		t.Fatalf("max open = %d", s.db.Stats().MaxOpenConnections)
	}
}

func TestUpsertTruncatesMinuteAndIsIdempotent(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	raw := time.Date(2026, 10, 31, 14, 3, 45, 0, time.UTC)
	minute := time.Date(2026, 10, 31, 14, 3, 0, 0, time.UTC)

	inserted, updated, err := s.Upsert(ctx, []model.Reading{{
		TS: raw, Mgdl: 112, Source: "test",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if inserted != 1 || updated != 0 {
		t.Fatalf("first write inserted=%d updated=%d", inserted, updated)
	}

	got, ok, err := s.Latest(ctx)
	if err != nil || !ok {
		t.Fatalf("latest ok=%v err=%v", ok, err)
	}
	if !got.TS.Equal(minute) || got.Mgdl != 112 || got.Source != "test" || got.ReceivedAt.IsZero() {
		t.Fatalf("latest = %+v", got)
	}

	inserted, updated, err = s.Upsert(ctx, []model.Reading{{
		TS: raw.Add(10 * time.Second), Mgdl: 130, Source: "retry",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if inserted != 0 || updated != 1 {
		t.Fatalf("overwrite inserted=%d updated=%d", inserted, updated)
	}
	got, _, err = s.Latest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got.TS.Equal(minute) || got.Mgdl != 130 || got.Source != "retry" {
		t.Fatalf("after overwrite = %+v", got)
	}

	inserted, updated, err = s.Upsert(ctx, []model.Reading{{
		TS: minute, Mgdl: 130, Source: "retry",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if inserted != 0 || updated != 1 {
		t.Fatalf("idempotent inserted=%d updated=%d", inserted, updated)
	}
	rows, err := s.List(ctx, minute.Add(-time.Hour), minute.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Mgdl != 130 {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestUpsertLastWriteWinsInsideBatch(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	minute := time.Date(2026, 10, 31, 14, 3, 0, 0, time.UTC)

	inserted, updated, err := s.Upsert(ctx, []model.Reading{
		{TS: minute.Add(10 * time.Second), Mgdl: 100, Source: "a"},
		{TS: minute.Add(40 * time.Second), Mgdl: 140, Source: "b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if inserted != 1 || updated != 1 {
		t.Fatalf("inserted=%d updated=%d", inserted, updated)
	}
	got, ok, err := s.Latest(ctx)
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	if got.Mgdl != 140 || got.Source != "b" || !got.TS.Equal(minute) {
		t.Fatalf("latest = %+v", got)
	}
}

func TestListOrdersByTime(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 31, 14, 0, 0, 0, time.UTC)
	_, _, err := s.Upsert(ctx, []model.Reading{
		{TS: base.Add(2 * time.Minute), Mgdl: 120, Source: "t"},
		{TS: base, Mgdl: 100, Source: "t"},
		{TS: base.Add(time.Minute), Mgdl: 110, Source: "t"},
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.List(ctx, base, base.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].Mgdl != 100 || rows[1].Mgdl != 110 || rows[2].Mgdl != 120 {
		t.Fatalf("rows = %+v", rows)
	}

	outside, err := s.List(ctx, base.Add(3*time.Minute), base.Add(4*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(outside) != 0 {
		t.Fatalf("outside = %+v", outside)
	}
}

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "glucose.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
