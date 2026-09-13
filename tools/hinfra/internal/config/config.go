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
	TailnetAPI    string `yaml:"tailnetAPI"`    // opcional; default = server do kubeconfig
	ArgocdRootApp string `yaml:"argocdRootApp"` // Application root no Argo CD (ex.: workloads-root)
}

type Runtime struct {
	InfraRepo     string
	WorkloadsRepo string
	Kubeconfig    string
	TailnetAPI    string
	ArgocdRootApp string
}

// ResolveArgocdRootApp picks the Argo CD Application used by `hinfra argocd refresh --root`.
func ResolveArgocdRootApp(explicit string, infraRepo, workloadsRepo string) string {
	if s := strings.TrimSpace(explicit); s != "" {
		return s
	}
	if workloadsRepo != infraRepo {
		return "workloads-root"
	}
	return "root-app"
}

func (r *Runtime) ArgocdRootApplication() string {
	return r.ArgocdRootApp
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

	rootApp := ResolveArgocdRootApp(cfg.ArgocdRootApp, infraRepo, workloadsRepo)

	return &Runtime{
		InfraRepo:     infraRepo,
		WorkloadsRepo: workloadsRepo,
		Kubeconfig:    kubeconfig,
		TailnetAPI:    tailnetAPI,
		ArgocdRootApp: rootApp,
	}, nil
}

func readConfigFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(infraRepo, workloadsRepo string) error {
	return SaveFull(Config{InfraRepo: infraRepo, WorkloadsRepo: workloadsRepo})
}

func SaveFull(cfg Config) error {
	dir := filepath.Join(os.Getenv("HOME"), ".config", "hinfra")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.yaml")

	infraAbs, err := filepath.Abs(cfg.InfraRepo)
	if err != nil {
		return fmt.Errorf("infraRepo inválido: %w", err)
	}
	workloadsAbs := infraAbs
	if strings.TrimSpace(cfg.WorkloadsRepo) != "" {
		workloadsAbs, err = filepath.Abs(cfg.WorkloadsRepo)
		if err != nil {
			return fmt.Errorf("workloadsRepo inválido: %w", err)
		}
	}

	if existing, err := readConfigFile(path); err == nil {
		if cfg.TailnetAPI == "" {
			cfg.TailnetAPI = existing.TailnetAPI
		}
		if strings.TrimSpace(cfg.ArgocdRootApp) == "" {
			cfg.ArgocdRootApp = existing.ArgocdRootApp
		}
	}
	cfg.InfraRepo = infraAbs
	cfg.WorkloadsRepo = workloadsAbs
	if workloadsAbs == infraAbs {
		cfg.WorkloadsRepo = ""
	}
	cfg.ArgocdRootApp = ResolveArgocdRootApp(cfg.ArgocdRootApp, infraAbs, workloadsAbs)

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
