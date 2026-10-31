package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRequiresAPIKey(t *testing.T) {
	setEnv(t, filepath.Join(t.TempDir(), "glucose.db"))
	t.Setenv("API_KEY", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected missing API_KEY to fail")
	}
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	setEnv(t, filepath.Join(dir, "glucose.db"))

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != ":8080" {
		t.Fatalf("listen = %q", cfg.ListenAddr)
	}
	if cfg.DisplayTZ.String() != "America/Sao_Paulo" {
		t.Fatalf("tz = %s", cfg.DisplayTZ)
	}
	if cfg.TargetLow != 70 || cfg.TargetHigh != 180 {
		t.Fatalf("targets = %d %d", cfg.TargetLow, cfg.TargetHigh)
	}
	if cfg.APIKey != "test-key" || cfg.DashPassword != "" {
		t.Fatal("credentials were not loaded")
	}
}

func TestLoadReadsDotEnv(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	unsetEnv(t, "API_KEY", "LISTEN_ADDR", "DASH_PASSWORD", "DISPLAY_TZ", "TARGET_LOW", "TARGET_HIGH", "DB_PATH")
	body := "API_KEY=from-file\nLISTEN_ADDR=:18081\nDASH_PASSWORD=\"secret value\"\n# ignored\nexport DB_PATH=" + filepath.Join(dir, "glucose.db") + "\n"
	if err := os.WriteFile(".env", []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "from-file" || cfg.ListenAddr != ":18081" || cfg.DashPassword != "secret value" {
		t.Fatalf("config = %+v", cfg)
	}
	if cfg.DBPath != filepath.Join(dir, "glucose.db") {
		t.Fatalf("db path = %s", cfg.DBPath)
	}
}

func TestLoadKeepsExistingEnvironment(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(".env", []byte("API_KEY=from-file\nLISTEN_ADDR=:18081\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	setEnv(t, filepath.Join(dir, "glucose.db"))

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "test-key" || cfg.ListenAddr != ":8080" {
		t.Fatalf("environment did not win: %+v", cfg)
	}
}

func TestLoadRejectsBadTimezoneAndTargets(t *testing.T) {
	setEnv(t, filepath.Join(t.TempDir(), "glucose.db"))
	t.Setenv("DISPLAY_TZ", "Not/AZone")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid timezone to fail")
	}

	setEnv(t, filepath.Join(t.TempDir(), "glucose.db"))
	t.Setenv("TARGET_LOW", "180")
	t.Setenv("TARGET_HIGH", "70")
	if _, err := Load(); err == nil {
		t.Fatal("expected inverted targets to fail")
	}
}

func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		prev, ok := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if ok {
				_ = os.Setenv(key, prev)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
}

func setEnv(t *testing.T, dbPath string) {
	t.Helper()
	t.Setenv("API_KEY", "test-key")
	t.Setenv("LISTEN_ADDR", "")
	t.Setenv("DB_PATH", dbPath)
	t.Setenv("DASH_PASSWORD", "")
	t.Setenv("DISPLAY_TZ", "")
	t.Setenv("TARGET_LOW", "")
	t.Setenv("TARGET_HIGH", "")
}
