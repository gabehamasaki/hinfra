package context

import (
	"strings"
)

type AppContext struct {
	Name        string
	Namespace   string
	InfraRepo   string
	ProjectRepo string
	Image       string
	ImageRef    string
	Images      map[string]string
	AppPath     string
	Kubeconfig  string
	Host        string
	Exposure    string
	Routing       *HinfraRouting
	BuildContexts map[string]BuildContext
	Hinfra        *HinfraConfig
}

type HinfraConfig struct {
	App           string                       `yaml:"app"`
	Image         string                       `yaml:"image"`
	Images        map[string]string            `yaml:"images"`
	BuildContexts map[string]BuildContext      `yaml:"buildContexts"`
	AppPath       string                       `yaml:"appPath"`
	Namespace     string                       `yaml:"namespace"`
	Host          string                       `yaml:"host"`
	Exposure      string                       `yaml:"exposure"`
	Routing       *HinfraRouting               `yaml:"routing"`
	Environments  map[string]HinfraEnvironment `yaml:"environments"`
}

type HinfraEnvironment struct {
	Enabled   bool   `yaml:"enabled"`
	TagPrefix string `yaml:"tagPrefix"`
	Branch    string `yaml:"branch"`
	AppPath   string `yaml:"appPath"`
	Namespace string `yaml:"namespace"`
	Host      string `yaml:"host"`
}

const defaultDeployBranch = "main"

// EnvironmentBranch returns the git branch used to validate tagged commits for an environment.
func (e HinfraEnvironment) EnvironmentBranch() string {
	if e.Branch != "" {
		return e.Branch
	}
	return defaultDeployBranch
}

// DefaultEnvironments returns production + optional dev/homolog stubs for hinfrac.yml scaffolding.
func DefaultEnvironments(appName string) map[string]HinfraEnvironment {
	basePath := "apps/" + appName
	return map[string]HinfraEnvironment{
		"production": {
			Enabled:   true,
			TagPrefix: "v",
			Branch:    defaultDeployBranch,
			AppPath:   basePath,
			Namespace: appName,
		},
		"dev": {
			Enabled:   false,
			TagPrefix: "dev/",
			Branch:    defaultDeployBranch,
			AppPath:   basePath + "/overlays/dev",
			Namespace: appName + "-dev",
			Host:      appName + ".dev.hamasakis.dev",
		},
		"homolog": {
			Enabled:   false,
			TagPrefix: "hg/",
			Branch:    defaultDeployBranch,
			AppPath:   basePath + "/overlays/homolog",
			Namespace: appName + "-hg",
			Host:      appName + ".hg.hamasakis.dev",
		},
	}
}

// ProductionEnvironment returns the production config, with defaults when environments is absent.
func (c *HinfraConfig) ProductionEnvironment() HinfraEnvironment {
	if c == nil || len(c.Environments) == 0 {
		app := c.App
		if app == "" {
			app = "app"
		}
		path := c.AppPath
		if path == "" {
			path = "apps/" + app
		}
		ns := c.Namespace
		if ns == "" {
			ns = app
		}
		return HinfraEnvironment{
			Enabled:   true,
			TagPrefix: "v",
			Branch:    defaultDeployBranch,
			AppPath:   path,
			Namespace: ns,
		}
	}
	if prod, ok := c.Environments["production"]; ok {
		if prod.AppPath == "" && c.AppPath != "" {
			prod.AppPath = c.AppPath
		}
		if prod.Namespace == "" && c.Namespace != "" {
			prod.Namespace = c.Namespace
		}
		return prod
	}
	return DefaultEnvironments(c.App)["production"]
}

// EnabledTagTriggers returns GitHub Actions tag glob patterns for enabled environments.
func (c *HinfraConfig) EnabledTagTriggers() []string {
	envs := c.EffectiveEnvironments()
	var triggers []string
	for _, env := range envs {
		if !env.Enabled {
			continue
		}
		triggers = append(triggers, tagPrefixToGlob(env.TagPrefix))
	}
	if len(triggers) == 0 {
		return []string{"v*", "dev/**", "hg/**"}
	}
	return triggers
}

// EffectiveEnvironments returns configured environments or production-only defaults.
func (c *HinfraConfig) EffectiveEnvironments() map[string]HinfraEnvironment {
	if c == nil || len(c.Environments) == 0 {
		if c == nil {
			return map[string]HinfraEnvironment{}
		}
		return DefaultEnvironments(c.App)
	}
	return c.Environments
}

func tagPrefixToGlob(prefix string) string {
	switch prefix {
	case "v", "v*":
		return "v*"
	case "dev/", "dev/*":
		return "dev/**"
	case "hg/", "hg/*":
		return "hg/**"
	default:
		if strings.HasSuffix(prefix, "/") {
			return prefix + "**"
		}
		return prefix + "*"
	}
}

type HinfraRouting struct {
	ApiPath string `yaml:"apiPath"`
	WebPath string `yaml:"webPath"`
}

const ImageRegistry = "ghcr.io"

func (a *AppContext) IsMonorepo() bool {
	return len(a.Images) > 0
}

func (a *AppContext) AllImageRepos() []string {
	if len(a.Images) > 0 {
		repos := make([]string, 0, len(a.Images))
		for _, repo := range a.Images {
			repos = append(repos, repo)
		}
		return repos
	}
	if a.Image != "" {
		return []string{a.Image}
	}
	return nil
}

func (a *AppContext) AllImageRefs() []string {
	repos := a.AllImageRepos()
	refs := make([]string, len(repos))
	for i, repo := range repos {
		refs[i] = ImageRegistry + "/" + repo
	}
	return refs
}

func primaryImage(images map[string]string, legacy string) string {
	if legacy != "" {
		return legacy
	}
	if len(images) == 0 {
		return ""
	}
	if repo, ok := images["web"]; ok {
		return repo
	}
	if repo, ok := images["api"]; ok {
		return repo
	}
	for _, repo := range images {
		return repo
	}
	return ""
}

func normalizeImageRepo(image string) string {
	return strings.TrimPrefix(image, ImageRegistry+"/")
}
