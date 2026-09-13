package context

import "strings"

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
}

type HinfraConfig struct {
	App           string                     `yaml:"app"`
	Image         string                     `yaml:"image"`
	Images        map[string]string          `yaml:"images"`
	BuildContexts map[string]BuildContext    `yaml:"buildContexts"`
	AppPath       string                     `yaml:"appPath"`
	Namespace     string                     `yaml:"namespace"`
	Host          string                     `yaml:"host"`
	Exposure      string                     `yaml:"exposure"`
	Routing       *HinfraRouting             `yaml:"routing"`
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
