package context

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProductionImageTag(t *testing.T) {
	dir := t.TempDir()
	appDir := filepath.Join(dir, "apps", "demo")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(filepath.Join(appDir, "kustomization.yaml"), `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
images:
- name: ghcr.io/gabehamasaki/demo
  newTag: v1.0.0
`)
	tag, err := ProductionImageTag(dir, "demo")
	if err != nil || tag != "v1.0.0" {
		t.Fatalf("got %q err %v", tag, err)
	}
}

func TestProductionImageTagMismatched(t *testing.T) {
	dir := t.TempDir()
	appDir := filepath.Join(dir, "apps", "demo")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(filepath.Join(appDir, "kustomization.yaml"), `images:
- name: a
  newTag: v1
- name: b
  newTag: v2
`)
	_, err := ProductionImageTag(dir, "demo")
	if err == nil {
		t.Fatal("expected mismatch error")
	}
}

func writeFile(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		panic(err)
	}
}
