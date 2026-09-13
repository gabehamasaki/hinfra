package actions

import (
	"context"
	"fmt"
	"strings"

	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/argocd"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/k8s"
)

// GitOpsWarnings flags a VPS that still uses the public hinfa root-app (example only).
func GitOpsWarnings(ctx context.Context, env *Env) ([]string, error) {
	if env.Runtime.WorkloadsRepo == "" || env.Runtime.WorkloadsRepo == env.Runtime.InfraRepo {
		return nil, nil
	}
	kc, err := k8s.NewClient(ctx, env.Runtime.Kubeconfig, env.Runtime.TailnetAPI)
	if err != nil {
		return nil, err
	}
	ac, err := argocd.NewClient(kc.Config)
	if err != nil {
		return nil, err
	}
	apps, err := ac.ListApplications(ctx)
	if err != nil {
		return nil, err
	}
	var warnings []string
	hasWorkloadsRoot := false
	projectApps := 0
	for _, app := range apps {
		switch app.Name {
		case "workloads-root":
			hasWorkloadsRoot = true
		case "root-app":
			warnings = append(warnings, "ArgoCD ainda tem Application root-app (repo público hinfa) — rode ansible role argocd ou apague root-app/example")
		case "example":
			warnings = append(warnings, "ArgoCD está sincronizando apps/example (template) — apague o Application example")
		}
		if app.Layer == argocd.LayerProjects && app.Name != "example" {
			projectApps++
		}
	}
	if !hasWorkloadsRoot {
		warnings = append(warnings, "falta Application workloads-root (repo hinfra-workloads) — ansible role argocd com vault_workloads_repo_token")
	}
	if hasWorkloadsRoot && projectApps == 0 {
		warnings = append(warnings, "workloads-root existe mas nenhum app de projeto no ArgoCD — verifique secret workloads-repo-creds e sync do workloads-root")
	}
	return warnings, nil
}

func FormatGitOpsWarnings(warnings []string) string {
	if len(warnings) == 0 {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("gitops:\n- %s", strings.Join(warnings, "\n- ")))
}
