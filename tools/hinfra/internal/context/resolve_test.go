package context

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gabehamasaki/infra/tools/hinfra/internal/git"
)

func TestParseOriginURL(t *testing.T) {
	cases := []struct {
		url         string
		owner, repo string
	}{
		{"https://github.com/gabehamasaki/my-portfolio.git", "gabehamasaki", "my-portfolio"},
		{"git@github.com:gabehamasaki/my-portfolio.git", "gabehamasaki", "my-portfolio"},
		{"https://github.com/gabehamasaki/my-portfolio", "gabehamasaki", "my-portfolio"},
	}
	for _, c := range cases {
		owner, repo, err := git.ParseOriginURL(c.url)
		if err != nil {
			t.Fatalf("%s: %v", c.url, err)
		}
		if owner != c.owner || repo != c.repo {
			t.Fatalf("%s: got %s/%s want %s/%s", c.url, owner, repo, c.owner, c.repo)
		}
	}
}

func TestResolveHTTPSWithoutHinfra(t *testing.T) {
	env := setupFixture(t, "https://github.com/gabehamasaki/my-portfolio.git", "")
	resolver := NewResolver(env.infra, env.kubeconfig)
	app, err := resolver.Resolve(env.project, "")
	if err != nil {
		t.Fatal(err)
	}
	if app.Name != "my-portfolio" || app.Image != "gabehamasaki/my-portfolio" {
		t.Fatalf("unexpected app: %+v", app)
	}
}

func TestResolveSSHWithGitSuffix(t *testing.T) {
	env := setupFixture(t, "git@github.com:gabehamasaki/my-portfolio.git", "")
	resolver := NewResolver(env.infra, env.kubeconfig)
	app, err := resolver.Resolve(env.project, "")
	if err != nil {
		t.Fatal(err)
	}
	if app.Name != "my-portfolio" {
		t.Fatalf("got %s", app.Name)
	}
}

func TestResolveHinfraOverridesFork(t *testing.T) {
	hinfra := "app: my-portfolio\nimage: gabehamasaki/my-portfolio\nappPath: apps/my-portfolio\n"
	env := setupFixture(t, "git@github.com:other/my-fork.git", hinfra)
	resolver := NewResolver(env.infra, env.kubeconfig)
	app, err := resolver.Resolve(env.project, "")
	if err != nil {
		t.Fatal(err)
	}
	if app.Name != "my-portfolio" {
		t.Fatalf("got %s", app.Name)
	}
}

func TestResolveForkWithoutHinfraFails(t *testing.T) {
	env := setupFixture(t, "git@github.com:other/my-fork.git", "")
	resolver := NewResolver(env.infra, env.kubeconfig)
	_, err := resolver.Resolve(env.project, "")
	if err == nil {
		t.Fatal("expected error for fork without hinfra")
	}
}

func TestResolveHinfraConflictsWithKustomization(t *testing.T) {
	hinfra := "app: wrong-app\nimage: gabehamasaki/wrong-app\nappPath: apps/my-portfolio\n"
	env := setupFixture(t, "https://github.com/gabehamasaki/my-portfolio.git", hinfra)
	resolver := NewResolver(env.infra, env.kubeconfig)
	_, err := resolver.Resolve(env.project, "")
	if err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestResolveMalformedHinfra(t *testing.T) {
	env := setupFixture(t, "https://github.com/gabehamasaki/my-portfolio.git", "app: [broken\n")
	resolver := NewResolver(env.infra, env.kubeconfig)
	_, err := resolver.Resolve(env.project, "")
	if err == nil {
		t.Fatal("expected hinfra parse error")
	}
}

func TestResolveMonorepoHinfra(t *testing.T) {
	hinfra := `app: my-app
host: my-app.hamasakis.dev
exposure: public
appPath: apps/my-app
namespace: my-app
images:
  api: gabehamasaki/my-app-api
  web: gabehamasaki/my-app-web
routing:
  apiPath: /api
  webPath: /
`
	env := setupMonorepoFixture(t, "https://github.com/gabehamasaki/my-app.git", hinfra)
	resolver := NewResolver(env.infra, env.kubeconfig)
	app, err := resolver.Resolve(env.project, "")
	if err != nil {
		t.Fatal(err)
	}
	if app.Name != "my-app" {
		t.Fatalf("got app %s", app.Name)
	}
	if !app.IsMonorepo() {
		t.Fatal("expected monorepo context")
	}
	if len(app.Images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(app.Images))
	}
	if app.Image != "gabehamasaki/my-app-web" {
		t.Fatalf("primary image should be web, got %s", app.Image)
	}
	if app.Routing == nil || app.Routing.ApiPath != "/api" || app.Routing.WebPath != "/" {
		t.Fatalf("unexpected routing: %+v", app.Routing)
	}
}

func TestResolveLegacyImageStillWorks(t *testing.T) {
	hinfra := "app: my-portfolio\nimage: gabehamasaki/my-portfolio\nappPath: apps/my-portfolio\n"
	env := setupFixture(t, "git@github.com:other/my-fork.git", hinfra)
	resolver := NewResolver(env.infra, env.kubeconfig)
	app, err := resolver.Resolve(env.project, "")
	if err != nil {
		t.Fatal(err)
	}
	if app.IsMonorepo() {
		t.Fatal("legacy hinfra should not be monorepo")
	}
	if app.Image != "gabehamasaki/my-portfolio" {
		t.Fatalf("got %s", app.Image)
	}
}

func TestImagesDeployed(t *testing.T) {
	refs := []string{"ghcr.io/gabehamasaki/demo-api", "ghcr.io/gabehamasaki/demo-web"}
	pods := []string{
		"ghcr.io/gabehamasaki/demo-api:abc123",
		"ghcr.io/gabehamasaki/demo-web:abc123",
	}
	if !ImagesDeployed(pods, refs, "abc123") {
		t.Fatal("expected all images deployed")
	}
	if ImagesDeployed(pods, refs, "def456") {
		t.Fatal("expected missing sha to fail")
	}
}

func setupMonorepoFixture(t *testing.T, origin, hinfraContent string) fixture {
	t.Helper()
	root := t.TempDir()
	infra := filepath.Join(root, "infra")
	project := filepath.Join(root, "project")
	mustMkdir(infra, "apps", "my-app")
	mustWrite(filepath.Join(infra, "apps", "my-app", "kustomization.yaml"), `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: my-app
images:
- name: ghcr.io/gabehamasaki/my-app-api
  newTag: abc123
- name: ghcr.io/gabehamasaki/my-app-web
  newTag: abc123
`)
	kubeconfig := filepath.Join(infra, ".secrets", "vps-1.kubeconfig")
	mustMkdir(infra, ".secrets")
	mustWrite(kubeconfig, "apiVersion: v1\nkind: Config\n")

	initGitRepo(project, origin)
	if hinfraContent != "" {
		mustWrite(filepath.Join(project, "hinfra.yml"), hinfraContent)
	}
	return fixture{infra: infra, project: project, kubeconfig: kubeconfig}
}

func TestResolveMissingKustomization(t *testing.T) {
	env := setupFixture(t, "https://github.com/gabehamasaki/missing-app.git", "")
	resolver := NewResolver(env.infra, env.kubeconfig)
	_, err := resolver.Resolve(env.project, "")
	if err == nil {
		t.Fatal("expected missing kustomization error")
	}
}

type fixture struct {
	infra      string
	project    string
	kubeconfig string
}

func setupFixture(t *testing.T, origin, hinfra string) fixture {
	t.Helper()
	root := t.TempDir()
	infra := filepath.Join(root, "infra")
	project := filepath.Join(root, "project")
	mustMkdir(infra, "apps", "my-portfolio")
	mustWrite(filepath.Join(infra, "apps", "my-portfolio", "kustomization.yaml"), `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: my-portfolio
images:
- name: ghcr.io/gabehamasaki/my-portfolio
  newTag: abc123
`)
	kubeconfig := filepath.Join(infra, ".secrets", "vps-1.kubeconfig")
	mustMkdir(infra, ".secrets")
	mustWrite(kubeconfig, "apiVersion: v1\nkind: Config\n")

	initGitRepo(project, origin)
	if hinfra != "" {
		mustWrite(filepath.Join(project, "hinfra.yml"), hinfra)
	}
	return fixture{infra: infra, project: project, kubeconfig: kubeconfig}
}

func initGitRepo(dir, origin string) {
	mustMkdir(dir)
	runGit(dir, "init")
	runGit(dir, "remote", "add", "origin", origin)
	mustWrite(filepath.Join(dir, "README.md"), "test\n")
	runGit(dir, "add", ".")
	runGit(dir, "commit", "-m", "init")
}

func runGit(dir string, args ...string) {
	r := git.NewRunner(dir)
	if _, err := r.Run(args...); err != nil {
		panic(err)
	}
}

func mustMkdir(parts ...string) {
	if err := os.MkdirAll(filepath.Join(parts...), 0o755); err != nil {
		panic(err)
	}
}

func mustWrite(path, content string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		panic(err)
	}
}

func asResolveError(err error, target **ResolveError) bool {
	if re, ok := err.(*ResolveError); ok {
		*target = re
		return true
	}
	return false
}
