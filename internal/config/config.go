package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Config is the process configuration loaded from the environment.
type Config struct {
	ListenAddr   string
	DBPath       string
	APIKey       string
	DashPassword string
	DisplayTZ    *time.Location
	TargetLow    int
	TargetHigh   int
}

// Load reads configuration from the environment.
// A .env file in the working directory is applied first. Variables already
// set in the environment are left unchanged. A missing .env file is ignored.
// API_KEY is required. The database directory is created if it is missing.
func Load() (Config, error) {
	if err := loadDotEnv(".env"); err != nil {
		return Config{}, err
	}
	cfg := Config{
		ListenAddr:   getenv("LISTEN_ADDR", ":8080"),
		DBPath:       getenv("DB_PATH", "./data/glucose.db"),
		APIKey:       os.Getenv("API_KEY"),
		DashPassword: os.Getenv("DASH_PASSWORD"),
		TargetLow:    70,
		TargetHigh:   180,
	}
	if cfg.APIKey == "" {
		return Config{}, fmt.Errorf("API_KEY is required")
	}

	loc, err := time.LoadLocation(getenv("DISPLAY_TZ", "America/Sao_Paulo"))
	if err != nil {
		return Config{}, fmt.Errorf("DISPLAY_TZ is not a valid timezone")
	}
	cfg.DisplayTZ = loc

	if v, ok := os.LookupEnv("TARGET_LOW"); ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("TARGET_LOW must be an integer")
		}
		cfg.TargetLow = n
	}
	if v, ok := os.LookupEnv("TARGET_HIGH"); ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("TARGET_HIGH must be an integer")
		}
		cfg.TargetHigh = n
	}
	if cfg.TargetLow >= cfg.TargetHigh {
		return Config{}, fmt.Errorf("TARGET_LOW must be less than TARGET_HIGH")
	}

	dir := filepath.Dir(cfg.DBPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Config{}, fmt.Errorf("create database directory")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadDotEnv sets variables from path when they are not already in the environment.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read .env")
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if first {
			line = strings.TrimPrefix(line, "\uFEFF")
			first = false
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if !validEnvKey(key) {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, envValue(value)); err != nil {
			return fmt.Errorf("read .env")
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read .env")
	}
	return nil
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		if i == 0 && !(r == '_' || unicode.IsLetter(r)) {
			return false
		}
		if i > 0 && !(r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

func envValue(raw string) string {
	value := strings.TrimSpace(raw)
	if len(value) >= 2 {
		quote := value[0]
		if (quote == '"' || quote == '\'') && value[len(value)-1] == quote {
			return value[1 : len(value)-1]
		}
	}
	if i := strings.Index(value, " #"); i >= 0 {
		value = strings.TrimSpace(value[:i])
	}
	return value
}
