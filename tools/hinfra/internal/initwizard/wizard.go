package initwizard

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	appctx "github.com/gabehamasaki/infra/tools/hinfra/internal/context"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/actions"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/config"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/git"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/mcpinstall"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/scaffold"
)

func RunMachine() error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Caminho do repo infra: ")
	infraRepo, _ := reader.ReadString('\n')
	infraRepo = strings.TrimSpace(infraRepo)
	if infraRepo == "" {
		return fmt.Errorf("infraRepo obrigatório")
	}
	abs, err := filepath.Abs(infraRepo)
	if err != nil {
		return err
	}
	if err := config.Save(abs); err != nil {
		return err
	}
	fmt.Println("Config salva em ~/.config/hinfra/config.yaml")
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".config", "infra-mcp", "config.yaml")); err == nil {
		fmt.Println("Migre a config legada: mv ~/.config/infra-mcp/config.yaml ~/.config/hinfra/config.yaml")
	}
	fmt.Print("Configurar MCP nos agents? [y/N]: ")
	ans, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(ans)) == "y" {
		return mcpinstall.RunWizard()
	}
	return nil
}

func RunProject(env *actions.Env) error {
	cwd := env.CWD
	if filepath.Clean(cwd) == filepath.Clean(env.Runtime.InfraRepo) {
		return fmt.Errorf("rode hinfra init na raiz do repo do projeto, não no repo infra")
	}
	if _, err := os.Stat(filepath.Join(cwd, ".git")); err != nil {
		return fmt.Errorf("cwd não parece ser um repo git: %s", cwd)
	}

	reader := bufio.NewReader(os.Stdin)
	name := defaultAppName(cwd)
	fmt.Printf("Nome do app [%s]: ", name)
	if line, _ := reader.ReadString('\n'); strings.TrimSpace(line) != "" {
		name = strings.TrimSpace(line)
	}
	host := name + ".hamasakis.dev"
	fmt.Printf("Host [%s]: ", host)
	if line, _ := reader.ReadString('\n'); strings.TrimSpace(line) != "" {
		host = strings.TrimSpace(line)
	}
	exposure := "public"
	fmt.Printf("Exposure (public/tailnet) [%s]: ", exposure)
	if line, _ := reader.ReadString('\n'); strings.TrimSpace(line) != "" {
		exposure = strings.TrimSpace(line)
	}
	monorepo := false
	fmt.Print("Monorepo api+web? [y/N]: ")
	if strings.TrimSpace(strings.ToLower(readLine(reader))) == "y" {
		monorepo = true
	}
	buildContexts := appctx.DefaultBuildContexts()
	if monorepo {
		fmt.Print("Layout Docker (1=api/web, 2=backend/frontend) [1]: ")
		switch strings.TrimSpace(readLine(reader)) {
		case "2":
			buildContexts = appctx.BackendFrontendBuildContexts()
		}
	}

	fmt.Println("\n--- Preview manifestos infra ---")
	appOut, err := actions.ScaffoldApp(env, actions.ScaffoldAppInput{
		Name: name, Host: host, Exposure: exposure, Write: false, Monorepo: monorepo,
	})
	if err != nil {
		return err
	}
	fmt.Println(appOut.Content)
	if appOut.DNS != "" {
		fmt.Println(appOut.DNS)
	}

	fmt.Print("\nGravar manifestos no repo infra? [y/N]: ")
	if strings.TrimSpace(strings.ToLower(readLine(reader))) == "y" {
		out, err := actions.ScaffoldApp(env, actions.ScaffoldAppInput{
			Name: name, Host: host, Exposure: exposure, Write: true, Monorepo: monorepo,
		})
		if err != nil {
			return err
		}
		fmt.Println(out.Message)
		for _, c := range out.Checklist {
			fmt.Println("-", c)
		}
	}

	hinfraPath := filepath.Join(cwd, "hinfra.yml")
	if _, err := os.Stat(hinfraPath); os.IsNotExist(err) {
		fmt.Println("\n--- Preview workflow ---")
		previewEnv := *env
		if monorepo {
			writeMinimalHinfraMonorepo(cwd, name, host, exposure, buildContexts)
			previewEnv.CWD = cwd
		}
		out, err := actions.ScaffoldWorkflow(&previewEnv, actions.ScaffoldWorkflowInput{
			AppOverride: name, Write: false, Monorepo: monorepo,
		})
		if err == nil {
			fmt.Println(out.Hinfra)
			fmt.Println(out.Workflow)
		}
	}

	fmt.Print("Gravar workflow + hinfra.yml no projeto? [y/N]: ")
	if strings.TrimSpace(strings.ToLower(readLine(reader))) == "y" {
		if monorepo {
			writeMinimalHinfraMonorepo(cwd, name, host, exposure, buildContexts)
		} else {
			writeMinimalHinfra(cwd, name, host, exposure)
		}
		out, err := actions.ScaffoldWorkflow(env, actions.ScaffoldWorkflowInput{
			AppOverride: name, Write: true, Monorepo: monorepo,
		})
		if err != nil {
			return err
		}
		fmt.Println(out.Message)
		for _, c := range out.Checklist {
			fmt.Println("-", c)
		}
	}

	printChecklist(name, host, exposure, monorepo)
	return nil
}

func defaultAppName(cwd string) string {
	r := git.NewRunner(cwd)
	if origin, err := r.OriginURL(); err == nil {
		if _, repo, err := git.ParseOriginURL(origin); err == nil {
			return repo
		}
	}
	return filepath.Base(cwd)
}

func readLine(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func writeMinimalHinfra(cwd, name, host, exposure string) {
	content := fmt.Sprintf(`app: %s
image: gabehamasaki/%s
appPath: apps/%s
namespace: %s
host: %s
exposure: %s
`, name, name, name, name, host, exposure)
	_ = os.WriteFile(filepath.Join(cwd, "hinfra.yml"), []byte(content), 0o644)
}

func writeMinimalHinfraMonorepo(cwd, name, host, exposure string, buildContexts map[string]appctx.BuildContext) {
	content := scaffold.RenderHinfra(scaffold.HinfraParams{
		App: name, Host: host, Exposure: exposure,
		AppPath: "apps/" + name, Namespace: name,
		Images: map[string]string{
			"api": "gabehamasaki/" + name + "-api",
			"web": "gabehamasaki/" + name + "-web",
		},
		BuildContexts: buildContexts,
		Routing:       &appctx.HinfraRouting{ApiPath: "/api", WebPath: "/"},
	})
	_ = os.WriteFile(filepath.Join(cwd, "hinfra.yml"), []byte(content), 0o644)
}

func printChecklist(name, host, exposure string, monorepo bool) {
	fmt.Println("\n=== Checklist ===")
	if monorepo {
		fmt.Println("[ ] Dockerfiles nos paths de buildContexts (ver hinfra.yml)")
	} else {
		fmt.Println("[ ] Dockerfile na raiz")
	}
	fmt.Printf("[ ] gh secret set INFRA_REPO_TOKEN --repo <owner>/%s\n", name)
	fmt.Println("[ ] Pacotes GHCR públicos ou imagePullSecret")
	fmt.Println("[ ] hinfra argocd refresh --root (após Application no infra)")
	if monorepo {
		fmt.Println("[ ] Postgres: role/DB — docs/08-data-services.md")
		fmt.Println("[ ] hinfra seal secret → sealed-secret.yaml no kustomization")
	}
	if exposure == "public" {
		fmt.Println("[ ] DNS no Cloudflare → 187.127.62.20")
	} else {
		fmt.Println("[ ] DNS no Cloudflare → 100.86.241.1 (tailnet)")
	}
	fmt.Println("[ ] git push em ambos os repos")
	fmt.Println("[ ] Primeiro deploy: CI bump SHA → sync Argo → migration PreSync → hinfra deploy status")
}
