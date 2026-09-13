package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const KubeconfigName = "vps-1.kubeconfig"

type Config struct {
	InfraRepo     string `yaml:"infraRepo"`
	WorkloadsRepo string `yaml:"workloadsRepo"`
	TailnetAPI    string `yaml:"tailnetAPI"` // opcional; default = server do kubeconfig
}

type Runtime struct {
	InfraRepo     string
	WorkloadsRepo string
	Kubeconfig    string
	TailnetAPI    string
}

// AppsRepo is where GitOps app manifests and kustomization.yaml live (often a private workloads repo).
func (r *Runtime) AppsRepo() string {
	if r.WorkloadsRepo != "" {
		return r.WorkloadsRepo
	}
	return r.InfraRepo
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
	workloadsRepo := infraRepo
	if strings.TrimSpace(cfg.WorkloadsRepo) != "" {
		workloadsRepo, err = filepath.Abs(cfg.WorkloadsRepo)
		if err != nil {
			return nil, fmt.Errorf("workloadsRepo inválido: %w", err)
		}
	}

	kubeconfig := filepath.Join(infraRepo, ".secrets", KubeconfigName)
	tailnetAPI, err := TailnetAPIAddr(kubeconfig, cfg.TailnetAPI)
	if err != nil {
		return nil, fmt.Errorf("config em %s: %w", path, err)
	}

	return &Runtime{
		InfraRepo:     infraRepo,
		WorkloadsRepo: workloadsRepo,
		Kubeconfig:    kubeconfig,
		TailnetAPI:    tailnetAPI,
	}, nil
}

func Save(infraRepo, workloadsRepo string) error {
	dir := filepath.Join(os.Getenv("HOME"), ".config", "hinfra")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.yaml")
	cfg := Config{InfraRepo: infraRepo, WorkloadsRepo: workloadsRepo}
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
	if _, err := os.Stat(r.AppsRepo()); err != nil {
		return fmt.Errorf("workloadsRepo não encontrado em %s: %w", r.AppsRepo(), err)
	}
	if _, err := os.Stat(r.Kubeconfig); err != nil {
		return fmt.Errorf("kubeconfig não encontrado em %s: %w", r.Kubeconfig, err)
	}
	return nil
}
