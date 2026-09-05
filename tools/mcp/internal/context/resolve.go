package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gabehamasaki/infra/tools/mcp/internal/git"
	"gopkg.in/yaml.v3"
)

type ResolveError struct {
	Code    string
	Message string
}

func (e *ResolveError) Error() string {
	return e.Message
}

type Resolver struct {
	InfraRepo  string
	Kubeconfig string
}

func NewResolver(infraRepo, kubeconfig string) *Resolver {
	return &Resolver{InfraRepo: infraRepo, Kubeconfig: kubeconfig}
}

func (r *Resolver) Resolve(cwd string, appOverride string) (*AppContext, error) {
	projectRepo, err := filepath.Abs(cwd)
	if err != nil {
		return nil, err
	}

	runner := git.NewRunner(projectRepo)
	top, err := runner.TopLevel()
	if err != nil {
		return nil, fmt.Errorf("cwd não está em um repositório git: %w", err)
	}
	projectRepo = top

	hinfra, err := loadHinfra(projectRepo)
	if err != nil {
		return nil, &ResolveError{Code: "hinfra_invalid", Message: err.Error()}
	}

	owner, repoName, err := git.ParseOriginURL(mustOrigin(runner))
	if err != nil {
		return nil, err
	}

	appName := strings.TrimSpace(appOverride)
	if appName == "" && hinfra != nil && hinfra.App != "" {
		appName = hinfra.App
	}
	if appName == "" {
		appName = repoName
	}

	imageName := ""
	if hinfra != nil && hinfra.Image != "" {
		imageName = hinfra.Image
	} else {
		imageName = owner + "/" + repoName
	}

	appPath := "apps/" + appName
	if hinfra != nil && hinfra.AppPath != "" {
		appPath = hinfra.AppPath
	}

	namespace := appName
	if hinfra != nil && hinfra.Namespace != "" {
		namespace = hinfra.Namespace
	}

	kustomPath := filepath.Join(r.InfraRepo, appPath, "kustomization.yaml")
	kustImage, kustErr := parseKustomizationImage(kustomPath)

	if appOverride == "" {
		if err := validateClues(appName, imageName, appPath, repoName, owner, kustImage, kustErr, hinfra != nil); err != nil {
			return nil, err
		}
	}

	host := ""
	exposure := ""
	if hinfra != nil {
		host = hinfra.Host
		exposure = hinfra.Exposure
	}

	return &AppContext{
		Name:        appName,
		Namespace:   namespace,
		InfraRepo:   r.InfraRepo,
		ProjectRepo: projectRepo,
		Image:       imageName,
		ImageRef:    ImageRegistry + "/" + imageName,
		AppPath:     appPath,
		Kubeconfig:  r.Kubeconfig,
		Host:        host,
		Exposure:    exposure,
	}, nil
}

func mustOrigin(runner *git.Runner) string {
	out, err := runner.OriginURL()
	if err != nil {
		return ""
	}
	return out
}

func loadHinfra(projectRepo string) (*HinfraConfig, error) {
	path := filepath.Join(projectRepo, "hinfra.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cfg HinfraConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("hinfra.yml malformado em %s: %w", path, err)
	}
	return &cfg, nil
}

type kustomImages struct {
	Images []struct {
		Name    string `yaml:"name"`
		NewName string `yaml:"newName"`
		NewTag  string `yaml:"newTag"`
	} `yaml:"images"`
}

func parseKustomizationImage(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var k kustomImages
	if err := yaml.Unmarshal(data, &k); err != nil {
		return "", err
	}
	if len(k.Images) == 0 {
		return "", fmt.Errorf("kustomization sem bloco images em %s", path)
	}
	img := k.Images[0]
	name := img.Name
	if img.NewName != "" {
		name = img.NewName
	}
	return name, nil
}

func validateClues(appName, imageName, appPath, repoName, owner, kustImage string, kustErr error, hasHinfra bool) error {
	expectedFromOrigin := owner + "/" + repoName
	originMatchesRepo := repoName == appName

	if kustErr != nil {
		if hasHinfra {
			// hinfra aponta para appPath sem kustomization — só ok se for onboarding novo
			return nil
		}
		if !originMatchesRepo {
			return &ResolveError{
				Code: "ambiguous_app",
				Message: fmt.Sprintf(
					"não foi possível confirmar o app: origin aponta para %q mas app candidato é %q e kustomization ausente em apps/%s",
					repoName, appName, appName,
				),
			}
		}
		return fmt.Errorf("apps/%s/kustomization.yaml: %w", appName, kustErr)
	}

	kustRepo := strings.TrimPrefix(kustImage, ImageRegistry+"/")
	conflicts := []string{}

	if kustRepo != "" && kustRepo != imageName {
		conflicts = append(conflicts, fmt.Sprintf("image em hinfra/origin (%s) ≠ kustomization (%s)", imageName, kustRepo))
	}
	if !originMatchesRepo && !hasHinfra {
		conflicts = append(conflicts, fmt.Sprintf("origin repo (%s) ≠ app (%s); crie hinfra.yml ou passe app explicitamente", repoName, appName))
	}
	if hasHinfra && !strings.HasSuffix(appPath, "/"+appName) && appPath != "apps/"+appName {
		if kustRepo != "" && !strings.Contains(kustRepo, appName) {
			conflicts = append(conflicts, fmt.Sprintf("hinfra app (%s) não corresponde à image do kustomization (%s)", appName, kustRepo))
		}
	}
	if expectedFromOrigin != imageName && !hasHinfra && repoName == appName && kustRepo != imageName {
		conflicts = append(conflicts, fmt.Sprintf("origin image (%s) ≠ candidato (%s)", expectedFromOrigin, imageName))
	}

	if len(conflicts) > 0 {
		return &ResolveError{
			Code:    "conflicting_clues",
			Message: "pistas discordantes — confirme o app explicitamente: " + strings.Join(conflicts, "; "),
		}
	}
	return nil
}

func ImageFromKustomization(path string) (imageRef, tag string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	var k kustomImages
	if err := yaml.Unmarshal(data, &k); err != nil {
		return "", "", err
	}
	if len(k.Images) == 0 {
		return "", "", fmt.Errorf("sem images")
	}
	img := k.Images[0]
	name := img.Name
	if img.NewName != "" {
		name = img.NewName
	}
	return name, img.NewTag, nil
}

func AppDirExists(infraRepo, namespace string) bool {
	path := filepath.Join(infraRepo, "apps", namespace)
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
