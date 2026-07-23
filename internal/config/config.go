package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Vault  string `toml:"vault"`
	Bind   string `toml:"bind"`
	Token  string `toml:"token"`
	Git    Git    `toml:"git"`
	Ollama Ollama `toml:"ollama"`
}

type Git struct {
	Enabled bool              `toml:"enabled"`
	Remotes map[string]string `toml:"remotes"`
}

type Ollama struct {
	BaseURL               string `toml:"base_url"`
	AgentModel            string `toml:"agent_model"`
	EmbedModel            string `toml:"embed_model"`
	EmbedQueryInstruction string `toml:"embed_query_instruction"`
}

func Default() Config {
	return Config{
		Bind: "127.0.0.1:7920",
		Git:  Git{Enabled: true},
		Ollama: Ollama{
			BaseURL:               "http://localhost:11434",
			AgentModel:            "qwen3.6:35b",
			EmbedModel:            "qwen3-embedding:8b",
			EmbedQueryInstruction: "Given a web search query, retrieve relevant passages that answer the query",
		},
	}
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolving config dir: %w", err)
	}
	return filepath.Join(dir, "mnemo", "config.toml"), nil
}

func Load(path string) (Config, error) {
	cfg := Default()
	meta, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return cfg, fmt.Errorf("loading config %s: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return cfg, fmt.Errorf("loading config %s: unknown key %s", path, undecoded[0])
	}
	if cfg.Vault == "" {
		return cfg, fmt.Errorf("loading config %s: vault path not set", path)
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("saving config %s: %w", path, err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("saving config %s: %w", path, err)
	}
	defer f.Close()
	if err := toml.NewEncoder(f).Encode(cfg); err != nil {
		return fmt.Errorf("saving config %s: %w", path, err)
	}
	return nil
}
