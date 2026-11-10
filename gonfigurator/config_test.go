package gonfigurator

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type testDatabase struct {
	Host string `map:"DB_HOST"`
	Max  int    `map:"DB_MAX"`
}

type testConfig struct {
	Name    string       `map:"TEST_NAME"`
	Port    int          `map:"TEST_PORT"`
	Enabled bool         `map:"TEST_ENABLED"`
	Limit   uint         `map:"TEST_LIMIT"`
	Ratio   float64      `map:"TEST_RATIO"`
	Users   []string     `map:"TEST_USERS"`
	DB      testDatabase // nested
	PtrDB   *testDatabase
}

func clearTestEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"TEST_NAME", "TEST_PORT", "TEST_ENABLED", "TEST_LIMIT",
		"TEST_RATIO", "TEST_USERS", "TEST_ARRAY", "DB_HOST", "DB_MAX",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	clearTestEnv(t)
	t.Setenv("TEST_NAME", "service")
	t.Setenv("TEST_PORT", "8080")
	t.Setenv("TEST_ENABLED", "true")
	t.Setenv("TEST_LIMIT", "42")
	t.Setenv("TEST_RATIO", "1.5")
	t.Setenv("TEST_USERS", " alice, bob ,carol ")
	t.Setenv("DB_HOST", "db.local")
	t.Setenv("DB_MAX", "10")

	cfg := testConfig{}
	if err := LoadConfig(&cfg, EnvOption{}); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Name != "service" {
		t.Errorf("Name = %q, want %q", cfg.Name, "service")
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if !cfg.Enabled {
		t.Error("Enabled = false, want true")
	}
	if cfg.Limit != 42 {
		t.Errorf("Limit = %d, want 42", cfg.Limit)
	}
	if cfg.Ratio != 1.5 {
		t.Errorf("Ratio = %v, want 1.5", cfg.Ratio)
	}
	wantUsers := []string{"alice", "bob", "carol"}
	if !reflect.DeepEqual(cfg.Users, wantUsers) {
		t.Errorf("Users = %v, want %v", cfg.Users, wantUsers)
	}
	if cfg.DB.Host != "db.local" || cfg.DB.Max != 10 {
		t.Errorf("DB = %+v, want {db.local 10}", cfg.DB)
	}
	if cfg.PtrDB == nil || cfg.PtrDB.Host != "db.local" || cfg.PtrDB.Max != 10 {
		t.Errorf("PtrDB = %+v, want {db.local 10}", cfg.PtrDB)
	}
}

func TestLoadConfigFromEnvEmptyValue(t *testing.T) {
	clearTestEnv(t)
	t.Setenv("TEST_NAME", "")

	cfg := testConfig{Name: "keep-me"}
	if err := LoadConfig(&cfg, EnvOption{}); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Name != "keep-me" {
		t.Errorf("Name = %q, want explicit value to be preserved", cfg.Name)
	}
}

func TestLoadConfigMissingKeysUseExistingValues(t *testing.T) {
	clearTestEnv(t)
	cfg := testConfig{Name: "keep-me", Port: 9090, Users: []string{"existing"}}
	if err := LoadConfig(&cfg, EnvOption{}); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Name != "keep-me" || cfg.Port != 9090 {
		t.Errorf("existing values overwritten: %+v", cfg)
	}
	if !reflect.DeepEqual(cfg.Users, []string{"existing"}) {
		t.Errorf("Users = %v, want [existing]", cfg.Users)
	}
}

func TestLoadConfigConversionError(t *testing.T) {
	clearTestEnv(t)
	t.Setenv("TEST_PORT", "not-a-number")

	cfg := testConfig{}
	err := LoadConfig(&cfg, EnvOption{})
	if err == nil {
		t.Fatal("LoadConfig() error = nil, want conversion error")
	}
	if !strings.Contains(err.Error(), "can't convert") {
		t.Errorf("LoadConfig() error = %v, want conversion details", err)
	}
}

func TestLoadConfigArray(t *testing.T) {
	type arrayConfig struct {
		Values [2]int `map:"TEST_ARRAY"`
	}
	clearTestEnv(t)
	t.Setenv("TEST_ARRAY", "1,2")

	cfg := arrayConfig{}
	if err := LoadConfig(&cfg, EnvOption{}); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Values != [2]int{1, 2} {
		t.Errorf("Values = %v, want [1 2]", cfg.Values)
	}
}

func TestLoadConfigInvalidTarget(t *testing.T) {
	var cfg *testConfig
	if err := LoadConfig(cfg, EnvOption{}); !errors.Is(err, ErrStructNotValid) {
		t.Errorf("LoadConfig(nil) error = %v, want ErrStructNotValid", err)
	}
}

type unsupportedOption struct{}

func (unsupportedOption) From() string { return "unsupported" }

func TestLoadConfigUnsupportedOption(t *testing.T) {
	var cfg testConfig
	if err := LoadConfig(&cfg, unsupportedOption{}); !errors.Is(err, ErrOptNotSupported) {
		t.Errorf("LoadConfig() error = %v, want ErrOptNotSupported", err)
	}
}

func TestLoadConfigPointerOption(t *testing.T) {
	clearTestEnv(t)
	t.Setenv("TEST_NAME", "ptr-option")

	cfg := testConfig{}
	if err := LoadConfig(&cfg, &EnvOption{}); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Name != "ptr-option" {
		t.Errorf("Name = %q, want %q", cfg.Name, "ptr-option")
	}
}

func TestLoadConfigFromJSON(t *testing.T) {
	type jsonDatabase struct {
		Host string `map:"DB.Host"`
		Max  int    `map:"DB.Max"`
	}
	type jsonConfig struct {
		Name    string       `map:"Name"`
		Big     int64        `map:"Big"`
		Enabled bool         `map:"Enabled"`
		Users   []string     `map:"Users"`
		DB      jsonDatabase // nested
	}
	content := `{
		"Name": "from-json",
		"Big": 9007199254740993,
		"Enabled": true,
		"Users": ["alice", "bob"],
		"DB": {"Host": "json.local", "Max": 7}
	}`
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg := jsonConfig{}
	if err := LoadConfig(&cfg, JsonOption{Path: path}); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Name != "from-json" {
		t.Errorf("Name = %q, want from-json", cfg.Name)
	}
	if cfg.Big != 9007199254740993 {
		t.Errorf("Big = %d, want 9007199254740993 (number precision lost)", cfg.Big)
	}
	if !cfg.Enabled {
		t.Error("Enabled = false, want true")
	}
	if !reflect.DeepEqual(cfg.Users, []string{"alice", "bob"}) {
		t.Errorf("Users = %v, want [alice bob]", cfg.Users)
	}
	if cfg.DB != (jsonDatabase{Host: "json.local", Max: 7}) {
		t.Errorf("DB = %+v, want {json.local 7}", cfg.DB)
	}
}

func TestLoadConfigFromMissingJSONFile(t *testing.T) {
	var cfg testConfig
	err := LoadConfig(&cfg, JsonOption{Path: filepath.Join(t.TempDir(), "missing.json")})
	if err == nil {
		t.Fatal("LoadConfig() error = nil, want file error")
	}
}
