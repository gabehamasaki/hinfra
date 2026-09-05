package context

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

	images, legacyImage := resolveImages(hinfra, owner, repoName)
	imageName := primaryImage(images, legacyImage)

	appPath := "apps/" + appName
	if hinfra != nil && hinfra.AppPath != "" {
		appPath = hinfra.AppPath
	}

	namespace := appName
	if hinfra != nil && hinfra.Namespace != "" {
		namespace = hinfra.Namespace
	}

	kustomPath := filepath.Join(r.InfraRepo, appPath, "kustomization.yaml")
	kustImages, kustErr := parseKustomizationImages(kustomPath)

	if appOverride == "" {
		if err := validateClues(appName, images, legacyImage, appPath, repoName, owner, kustImages, kustErr, hinfra != nil); err != nil {
			return nil, err
		}
	}

	host := ""
	exposure := ""
	var routing *HinfraRouting
	if hinfra != nil {
		host = hinfra.Host
		exposure = hinfra.Exposure
		routing = hinfra.Routing
	}

	ctx := &AppContext{
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
		Routing:     routing,
	}
	if len(images) > 0 {
		ctx.Images = images
	}
	return ctx, nil
}

func resolveImages(hinfra *HinfraConfig, owner, repoName string) (map[string]string, string) {
	if hinfra != nil && len(hinfra.Images) > 0 {
		return hinfra.Images, ""
	}
	single := owner + "/" + repoName
	if hinfra != nil && hinfra.Image != "" {
		single = hinfra.Image
	}
	return nil, single
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

func parseKustomizationImages(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var k kustomImages
	if err := yaml.Unmarshal(data, &k); err != nil {
		return nil, err
	}
	if len(k.Images) == 0 {
		return nil, fmt.Errorf("kustomization sem bloco images em %s", path)
	}
	repos := make([]string, 0, len(k.Images))
	for _, img := range k.Images {
		name := img.Name
		if img.NewName != "" {
			name = img.NewName
		}
		repos = append(repos, normalizeImageRepo(name))
	}
	return repos, nil
}

func parseKustomizationImage(path string) (string, error) {
	repos, err := parseKustomizationImages(path)
	if err != nil {
		return "", err
	}
	return repos[0], nil
}

func validateClues(appName string, images map[string]string, legacyImage, appPath, repoName, owner string, kustImages []string, kustErr error, hasHinfra bool) error {
	expectedFromOrigin := owner + "/" + repoName
	originMatchesRepo := repoName == appName
	hinfraRepos := hintraImageRepos(images, legacyImage)

	if kustErr != nil {
		if hasHinfra {
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

	conflicts := []string{}
	if !imageSetsMatch(hinfraRepos, kustImages) {
		conflicts = append(conflicts, fmt.Sprintf("images em hinfra/origin (%s) ≠ kustomization (%s)", strings.Join(hinfraRepos, ", "), strings.Join(kustImages, ", ")))
	}
	if !originMatchesRepo && !hasHinfra {
		conflicts = append(conflicts, fmt.Sprintf("origin repo (%s) ≠ app (%s); crie hinfra.yml ou passe app explicitamente", repoName, appName))
	}
	if hasHinfra && !strings.HasSuffix(appPath, "/"+appName) && appPath != "apps/"+appName {
		if len(kustImages) > 0 && !anyImageContains(kustImages, appName) {
			conflicts = append(conflicts, fmt.Sprintf("hinfra app (%s) não corresponde às images do kustomization (%s)", appName, strings.Join(kustImages, ", ")))
		}
	}
	if len(images) == 0 && expectedFromOrigin != legacyImage && !hasHinfra && repoName == appName && len(kustImages) > 0 && kustImages[0] != legacyImage {
		conflicts = append(conflicts, fmt.Sprintf("origin image (%s) ≠ candidato (%s)", expectedFromOrigin, legacyImage))
	}

	if len(conflicts) > 0 {
		return &ResolveError{
			Code:    "conflicting_clues",
			Message: "pistas discordantes — confirme o app explicitamente: " + strings.Join(conflicts, "; "),
		}
	}
	return nil
}

func hintraImageRepos(images map[string]string, legacyImage string) []string {
	if len(images) > 0 {
		repos := make([]string, 0, len(images))
		for _, repo := range images {
			repos = append(repos, normalizeImageRepo(repo))
		}
		sort.Strings(repos)
		return repos
	}
	if legacyImage != "" {
		return []string{normalizeImageRepo(legacyImage)}
	}
	return nil
}

func imageSetsMatch(hinfraRepos, kustImages []string) bool {
	if len(hinfraRepos) == 0 {
		return true
	}
	if len(kustImages) == 0 {
		return true
	}
	if len(hinfraRepos) != len(kustImages) {
		return false
	}
	a := append([]string(nil), hinfraRepos...)
	b := append([]string(nil), kustImages...)
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func anyImageContains(images []string, needle string) bool {
	for _, img := range images {
		if strings.Contains(img, needle) {
			return true
		}
	}
	return false
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

func ImagesDeployed(podImages []string, imageRefs []string, sha string) bool {
	for _, ref := range imageRefs {
		repo := normalizeImageRepo(ref)
		found := false
		for _, podImage := range podImages {
			if !strings.Contains(podImage, sha) {
				continue
			}
			if strings.Contains(podImage, repo) || strings.Contains(podImage, ref) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
