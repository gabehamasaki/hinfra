package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	DefaultTailnetAPI = "100.86.241.1:6443"
	KubeconfigName    = "vps-1.kubeconfig"
)

type Config struct {
	InfraRepo string `yaml:"infraRepo"`
}

type Runtime struct {
	InfraRepo  string
	Kubeconfig string
	TailnetAPI string
}

func ConfigPath() string {
	if override := os.Getenv("HINFRA_CONFIG"); override != "" {
		return override
	}
	if override := os.Getenv("INFRA_MCP_CONFIG"); override != "" {
		return override
	}
	newPath := filepath.Join(os.Getenv("HOME"), ".config", "hinfra", "config.yaml")
	if _, err := os.Stat(newPath); err == nil {
		return newPath
	}
	legacy := filepath.Join(os.Getenv("HOME"), ".config", "infra-mcp", "config.yaml")
	if _, err := os.Stat(legacy); err == nil {
		fmt.Fprintln(os.Stderr, "aviso: usando config legada em ~/.config/infra-mcp — migre para ~/.config/hinfra")
		return legacy
	}
	return newPath
}

func Load() (*Runtime, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config obrigatória em %s: %w (crie com infraRepo: /caminho/para/infra ou rode hinfra init --machine)", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config inválida em %s: %w", path, err)
	}
	if cfg.InfraRepo == "" {
		return nil, fmt.Errorf("config em %s: campo infraRepo é obrigatório", path)
	}

	infraRepo, err := filepath.Abs(cfg.InfraRepo)
	if err != nil {
		return nil, fmt.Errorf("infraRepo inválido: %w", err)
	}

	return &Runtime{
		InfraRepo:  infraRepo,
		Kubeconfig: filepath.Join(infraRepo, ".secrets", KubeconfigName),
		TailnetAPI: DefaultTailnetAPI,
	}, nil
}

func Save(infraRepo string) error {
	dir := filepath.Join(os.Getenv("HOME"), ".config", "hinfra")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.yaml")
	cfg := Config{InfraRepo: infraRepo}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (r *Runtime) ValidateFiles() error {
	if _, err := os.Stat(r.InfraRepo); err != nil {
		return fmt.Errorf("infraRepo não encontrado em %s: %w", r.InfraRepo, err)
	}
	if _, err := os.Stat(r.Kubeconfig); err != nil {
		return fmt.Errorf("kubeconfig não encontrado em %s: %w", r.Kubeconfig, err)
	}
	return nil
}
