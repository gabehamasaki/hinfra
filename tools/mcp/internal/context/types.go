package context

type AppContext struct {
	Name        string
	Namespace   string
	InfraRepo   string
	ProjectRepo string
	Image       string
	ImageRef    string
	AppPath     string
	Kubeconfig  string
	Host        string
	Exposure    string
}

type HinfraConfig struct {
	App       string `yaml:"app"`
	Image     string `yaml:"image"`
	AppPath   string `yaml:"appPath"`
	Namespace string `yaml:"namespace"`
	Host      string `yaml:"host"`
	Exposure  string `yaml:"exposure"`
}

const ImageRegistry = "ghcr.io"
