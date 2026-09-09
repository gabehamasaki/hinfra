package initwizard

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gabehamasaki/infra/tools/hinfra/internal/actions"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/config"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/git"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/mcpinstall"
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

	fmt.Println("\n--- Preview manifestos infra ---")
	appOut, err := actions.ScaffoldApp(env, actions.ScaffoldAppInput{
		Name: name, Host: host, Exposure: exposure, Write: false,
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
		_, err := actions.ScaffoldApp(env, actions.ScaffoldAppInput{
			Name: name, Host: host, Exposure: exposure, Write: true,
		})
		if err != nil {
			return err
		}
		fmt.Println("Manifestos gravados no repo infra")
	}

	// Temporarily set app context via hinfra.yml preview
	hinfraPath := filepath.Join(cwd, "hinfra.yml")
	if _, err := os.Stat(hinfraPath); os.IsNotExist(err) {
		_, err := actions.ScaffoldWorkflow(env, name, false)
		if err == nil {
			fmt.Println("\n--- Preview workflow ---")
		}
	}

	fmt.Print("Gravar workflow + hinfra.yml no projeto? [y/N]: ")
	if strings.TrimSpace(strings.ToLower(readLine(reader))) == "y" {
		// Write minimal hinfra.yml first so resolve works
		writeMinimalHinfra(cwd, name, host, exposure)
		out, err := actions.ScaffoldWorkflow(env, name, true)
		if err != nil {
			return err
		}
		fmt.Println(out.Message)
		for _, c := range out.Checklist {
			fmt.Println("-", c)
		}
	}

	printChecklist(name, host, exposure)
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

func printChecklist(name, host, exposure string) {
	fmt.Println("\n=== Checklist ===")
	fmt.Println("[ ] Dockerfile na raiz (ou api/ + web/)")
	fmt.Printf("[ ] gh secret set INFRA_REPO_TOKEN --repo <owner>/%s\n", name)
	if exposure == "public" {
		fmt.Println("[ ] DNS no Cloudflare → 187.127.62.20")
	} else {
		fmt.Println("[ ] DNS no Cloudflare → 100.86.241.1 (tailnet)")
	}
	fmt.Println("[ ] git push em ambos os repos")
	fmt.Println("[ ] hinfra deploy status")
}
