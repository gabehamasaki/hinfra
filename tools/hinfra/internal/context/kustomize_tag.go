package context

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ProductionImageTag reads apps/<appName>/kustomization.yaml and returns the shared newTag.
func ProductionImageTag(infraRepo, appName string) (string, error) {
	path := filepath.Join(infraRepo, "apps", appName, "kustomization.yaml")
	return parseKustomizationNewTag(path)
}

// ImageTagForAppPath reads kustomization.yaml under infraRepo at appPath (e.g. apps/foo/overlays/dev).
func ImageTagForAppPath(infraRepo, appPath string) (string, error) {
	path := filepath.Join(infraRepo, appPath, "kustomization.yaml")
	return parseKustomizationNewTag(path)
}

func parseKustomizationNewTag(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var k kustomImages
	if err := yaml.Unmarshal(data, &k); err != nil {
		return "", fmt.Errorf("kustomization malformado em %s: %w", path, err)
	}
	if len(k.Images) == 0 {
		return "", fmt.Errorf("kustomization sem bloco images em %s", path)
	}
	tag := k.Images[0].NewTag
	if tag == "" {
		return "", fmt.Errorf("newTag vazio em %s", path)
	}
	for i := 1; i < len(k.Images); i++ {
		if k.Images[i].NewTag != tag {
			return "", fmt.Errorf("tags divergentes no kustomization %s", path)
		}
	}
	return tag, nil
}
