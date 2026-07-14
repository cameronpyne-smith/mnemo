package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	cfg := Default()
	cfg.Vault = `C:\vaults\brain`
	cfg.Token = "secret"

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != cfg {
		t.Errorf("Load = %+v, want %+v", got, cfg)
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("vault = 'C:\\vaults\\brain'\n"), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	def := Default()
	if got.Bind != def.Bind || got.Ollama != def.Ollama {
		t.Errorf("defaults not applied: %+v", got)
	}
}

func TestLoadRejectsMissingVault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("bind = '127.0.0.1:1'\n"), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load accepted a config without a vault path")
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("vault = 'x'\ntypo_key = true\n"), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load accepted a config with an unknown key")
	}
}
