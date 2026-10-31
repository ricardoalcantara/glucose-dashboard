package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"glucose-dashboard/internal/store"
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

func TestIngestValidationAndIdempotency(t *testing.T) {
	st, h := testMux(t, "")
	now := fixedNow()

	rec := postJSON(t, h, "test-key", map[string]any{
		"source": "librelinkup",
		"readings": []any{
			map[string]any{"ts": "2026-10-31T14:03:30Z", "mgdl": 112},
			map[string]any{"ts": "2026-10-31T14:03:50Z", "mgdl": 114},
			map[string]any{"ts": "2026-10-31T14:04:00Z", "mgdl": 19},
			map[string]any{"ts": "2026-10-31T14:05:00Z", "mgdl": 600},
			map[string]any{"ts": "2026-10-31T14:06:00Z", "mgdl": 601},
			map[string]any{"ts": "2026-10-31T15:05:00Z", "mgdl": 100},
			map[string]any{"ts": "2026-10-31T15:05:01Z", "mgdl": 100},
			map[string]any{"ts": "yesterday", "mgdl": 100},
			map[string]any{"ts": "2026-10-31T14:07:00Z", "mgdl": 112.5},
			map[string]any{"ts": "2026-10-31T14:08:00Z", "mgdl": nil},
			map[string]any{"ts": "2026-10-31T14:09:00Z", "mgdl": 20.0},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp ingestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Received != 11 || resp.Inserted != 4 || resp.Updated != 1 || len(resp.Rejected) != 6 {
		t.Fatalf("response = %+v", resp)
	}
	reasons := map[int]string{}
	for _, item := range resp.Rejected {
		reasons[item.Index] = item.Reason
	}
	if reasons[2] != "mgdl_out_of_range" || reasons[4] != "mgdl_out_of_range" ||
		reasons[6] != "timestamp_in_future" || reasons[7] != "timestamp_invalid" ||
		reasons[8] != "mgdl_not_integer" || reasons[9] != "mgdl_not_integer" {
		t.Fatalf("reasons = %+v", reasons)
	}

	rows, err := st.List(t.Context(), now.Add(-24*time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("stored %d rows: %+v", len(rows), rows)
	}
	if !rows[0].TS.Equal(time.Date(2026, 10, 31, 14, 3, 0, 0, time.UTC)) || rows[0].Mgdl != 114 || rows[0].Source != "librelinkup" {
		t.Fatalf("first row = %+v", rows[0])
	}
	if rows[1].Mgdl != 600 || rows[2].Mgdl != 20 || rows[3].Mgdl != 100 {
		t.Fatalf("rows = %+v", rows)
	}

	again := postJSON(t, h, "test-key", map[string]any{
		"source": "librelinkup",
		"readings": []any{
			map[string]any{"ts": "2026-10-31T14:03:00Z", "mgdl": 114},
		},
	})
	var second ingestResponse
	if err := json.Unmarshal(again.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if again.Code != http.StatusOK || second.Inserted != 0 || second.Updated != 1 || len(second.Rejected) != 0 {
		t.Fatalf("idempotent status=%d resp=%+v", again.Code, second)
	}
}

func TestIngestRejectsMalformedRequests(t *testing.T) {
	_, h := testMux(t, "")
	cases := []struct {
		name string
		body string
	}{
		{"json", "{"},
		{"source", `{"readings":[]}`},
		{"blank source", `{"source":"  ","readings":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/readings", strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer test-key")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d", rec.Code)
			}
			if strings.Contains(rec.Body.String(), "glucose.db") {
				t.Fatal("error exposed the database path")
			}
		})
	}
}

func TestIngestLimits(t *testing.T) {
	_, h := testMux(t, "")

	readings := make([]any, maxBatch+1)
	for i := range readings {
		readings[i] = map[string]any{}
	}
	rec := postJSON(t, h, "test-key", map[string]any{"source": "x", "readings": readings})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("batch status = %d", rec.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/readings", bytes.NewReader(bytes.Repeat([]byte("x"), maxBodyBytes+1)))
	req.Header.Set("Authorization", "Bearer test-key")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("body status = %d", rec.Code)
	}
}

func TestBearerAuth(t *testing.T) {
	_, h := testMux(t, "")
	body := `{"source":"x","readings":[{"ts":"2026-10-31T14:00:00Z","mgdl":110}]}`

	noAuth := httptest.NewRequest(http.MethodPost, "/api/v1/readings", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, noAuth)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing status = %d", rec.Code)
	}

	for _, header := range []string{"Bearer wrong", "Bearer test-key-extra", "Bearer ", "Basic test-key", "bearer wrong-length"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/readings", strings.NewReader(body))
		req.Header.Set("Authorization", header)
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%q status = %d", header, rec.Code)
		}
	}

	ok := httptest.NewRequest(http.MethodPost, "/api/v1/readings", strings.NewReader(body))
	ok.Header.Set("Authorization", "bearer test-key")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, ok)
	if rec.Code != http.StatusOK {
		t.Fatalf("good key status = %d body = %s", rec.Code, rec.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/api/v1/readings?from=2026-10-31T13:00:00Z&to=2026-10-31T15:00:00Z", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, get)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("get without key status = %d", rec.Code)
	}
}

func TestGetReadings(t *testing.T) {
	_, h := testMux(t, "")
	postJSON(t, h, "test-key", map[string]any{
		"source": "sensor",
		"readings": []any{
			map[string]any{"ts": "2026-10-31T14:10:00Z", "mgdl": 130},
			map[string]any{"ts": "2026-10-31T14:00:00Z", "mgdl": 120},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/readings?from=2026-10-31T14:00:00Z&to=2026-10-31T14:10:00Z", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var rows []readingJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Mgdl != 120 || rows[1].Mgdl != 130 || rows[0].TS != "2026-10-31T14:00:00Z" || rows[0].Source != "sensor" {
		t.Fatalf("rows = %+v", rows)
	}

	tooWide := httptest.NewRequest(http.MethodGet, "/api/v1/readings?from=2026-10-24T14:59:59Z&to=2026-10-31T15:00:00Z", nil)
	tooWide.Header.Set("Authorization", "Bearer test-key")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, tooWide)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("wide status = %d", rec.Code)
	}

	exact := httptest.NewRequest(http.MethodGet, "/api/v1/readings?from=2026-10-24T15:00:00Z&to=2026-10-31T15:00:00Z", nil)
	exact.Header.Set("Authorization", "Bearer test-key")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, exact)
	if rec.Code != http.StatusOK {
		t.Fatalf("7 day status = %d body = %s", rec.Code, rec.Body.String())
	}

	backwards := httptest.NewRequest(http.MethodGet, "/api/v1/readings?from=2026-10-31T15:00:00Z&to=2026-10-31T14:00:00Z", nil)
	backwards.Header.Set("Authorization", "Bearer test-key")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, backwards)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("backwards status = %d", rec.Code)
	}

	missing := httptest.NewRequest(http.MethodGet, "/api/v1/readings", nil)
	missing.Header.Set("Authorization", "Bearer test-key")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, missing)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing status = %d", rec.Code)
	}
}

func TestHealthzIsOpen(t *testing.T) {
	_, h := testMux(t, "secret")
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "ok" {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestDashboardBasicAuth(t *testing.T) {
	_, open := testMux(t, "")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	open.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "page" {
		t.Fatalf("open dashboard status=%d body=%q", rec.Code, rec.Body.String())
	}

	_, locked := testMux(t, "secret")
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	locked.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Header().Get("WWW-Authenticate"), "Basic") {
		t.Fatalf("locked status=%d auth=%q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("anyone", "secret")
	rec = httptest.NewRecorder()
	locked.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "page" {
		t.Fatalf("good password status=%d body=%q", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("anyone", "nope")
	rec = httptest.NewRecorder()
	locked.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad password status=%d", rec.Code)
	}

	health := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec = httptest.NewRecorder()
	locked.ServeHTTP(rec, health)
	if rec.Code != http.StatusOK {
		t.Fatalf("health behind dashboard password status=%d", rec.Code)
	}
}

func testMux(t *testing.T, dashPassword string) (*store.Store, http.Handler) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "glucose.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	page := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("page"))
	})
	now := fixedNow()
	h := NewMux(&Server{
		Store:  st,
		APIKey: "test-key",
		Now:    func() time.Time { return now },
	}, dashPassword, page)
	return st, h
}

func fixedNow() time.Time {
	return time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
}

func postJSON(t *testing.T, h http.Handler, key string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/readings", &buf)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
