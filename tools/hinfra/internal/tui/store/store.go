package store

import (
	"context"
	"strings"
	"time"

	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/actions"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/argocd"
)

type ClusterSnapshot struct {
	FetchedAt    time.Time
	Applications []argocd.ApplicationRow
	Err          error
}

type AppSnapshot struct {
	Name      string
	Namespace string
	Health    actions.AppHealthResult
	Deploy    actions.DeployStatusResult
	Err       error
}

func FetchCluster(ctx context.Context, env *actions.Env) ClusterSnapshot {
	apps, err := actions.ListApplications(ctx, env)
	return ClusterSnapshot{FetchedAt: time.Now(), Applications: apps, Err: err}
}

func FetchApp(ctx context.Context, env *actions.Env, row argocd.ApplicationRow) AppSnapshot {
	namespace := row.Namespace
	if namespace == "" {
		namespace = row.Name
	}
	health, err := actions.AppHealthByName(ctx, env, namespace)
	if err != nil {
		return AppSnapshot{Name: row.Name, Namespace: namespace, Err: err}
	}
	// deploy_status depende de resolver o repo do projeto e falha para apps de
	// plataforma; o painel de saúde ainda vale sem ele.
	deploy, _ := actions.DeployStatus(ctx, env, row.Name)
	return AppSnapshot{Name: row.Name, Namespace: namespace, Health: health, Deploy: deploy}
}

// FilterApplications aplica camada e busca por substring. Vive aqui, e não na
// scene, para poder ser testado sem instanciar a TUI.
func FilterApplications(apps []argocd.ApplicationRow, layer argocd.Layer, filter string) []argocd.ApplicationRow {
	out := make([]argocd.ApplicationRow, 0, len(apps))
	needle := strings.ToLower(filter)
	for _, app := range apps {
		if layer != "" && app.Layer != layer {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(app.Name), needle) {
			continue
		}
		out = append(out, app)
	}
	return out
}

// ApplicationTotals resume a saúde do conjunto para o cabeçalho do dashboard.
type ApplicationTotals struct {
	Total    int
	Synced   int
	Healthy  int
	Problems []argocd.ApplicationRow
	ByLayer  map[argocd.Layer]int
}

func SummarizeApplications(apps []argocd.ApplicationRow) ApplicationTotals {
	totals := ApplicationTotals{Total: len(apps), ByLayer: map[argocd.Layer]int{}}
	for _, app := range apps {
		totals.ByLayer[app.Layer]++
		if app.Sync == "Synced" {
			totals.Synced++
		}
		if app.Health == "Healthy" {
			totals.Healthy++
		}
		if app.Sync != "Synced" || app.Health != "Healthy" {
			totals.Problems = append(totals.Problems, app)
		}
	}
	return totals
}
