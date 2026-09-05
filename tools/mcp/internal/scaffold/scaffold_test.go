package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderWorkflow(t *testing.T) {
	candidates := []string{
		filepath.Join("..", "..", "..", "docs", "deploy-workflow-template.yml"),
		filepath.Join("..", "..", "..", "..", "docs", "deploy-workflow-template.yml"),
	}
	var tmpl []byte
	var err error
	for _, c := range candidates {
		tmpl, err = os.ReadFile(c)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Skip("template not found from test cwd")
	}
	out := RenderWorkflow(string(tmpl), WorkflowParams{
		ImageName: "gabehamasaki/my-portfolio",
		AppPath:   "apps/my-portfolio",
	})
	if strings.Contains(out, "CHANGE-ME") {
		t.Fatalf("template still has CHANGE-ME")
	}
	if !strings.Contains(out, "gabehamasaki/my-portfolio") || !strings.Contains(out, "apps/my-portfolio") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRenderAppFilesGolden(t *testing.T) {
	files, err := RenderAppFiles(AppParams{
		Name: "demo", Host: "demo.hamasakis.dev", Exposure: "public",
		ImageRef: "ghcr.io/gabehamasaki/demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 5 {
		t.Fatalf("expected 5 files, got %d", len(files))
	}
	dep := files["apps/demo/deployment.yaml"]
	if !strings.Contains(dep, "cpu: 10m") || !strings.Contains(dep, "ghcr.io/gabehamasaki/demo") {
		t.Fatalf("unexpected deployment: %s", dep)
	}
	ing := files["apps/demo/ingress.yaml"]
	if strings.Contains(ing, "tailnet-only") {
		t.Fatal("public ingress should not have tailnet middleware")
	}
	ingTail, _ := RenderAppFiles(AppParams{Name: "admin", Host: "admin.hamasakis.cloud", Exposure: "tailnet", ImageRef: "ghcr.io/gabehamasaki/admin"})
	if !strings.Contains(ingTail["apps/admin/ingress.yaml"], "tailnet-only") {
		t.Fatal("tailnet ingress should have middleware")
	}
}

func TestWriteFilesRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := WriteFiles(dir, map[string]string{"a.txt": "new"}, true)
	if err == nil {
		t.Fatal("expected overwrite refusal")
	}
}
