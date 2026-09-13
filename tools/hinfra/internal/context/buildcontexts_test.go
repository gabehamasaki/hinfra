package context

import "testing"

func TestMergeBuildContexts(t *testing.T) {
	merged := MergeBuildContexts(map[string]BuildContext{
		"api": {Context: "./backend", File: "backend/docker/Dockerfile"},
	})
	if merged["api"].File != "backend/docker/Dockerfile" {
		t.Fatalf("unexpected api file: %v", merged["api"])
	}
	if merged["web"].Context != "./web" {
		t.Fatalf("web should keep default context: %v", merged["web"])
	}
}

func TestBackendFrontendBuildContexts(t *testing.T) {
	bc := BackendFrontendBuildContexts()
	if bc["api"].File != "backend/docker/Dockerfile" {
		t.Fatal(bc["api"])
	}
}
