package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/actions"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/argocd"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/tailnet"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/tui/store"
)

// watchInterval é curto porque o dashboard só faz leituras na API — o custo
// fica no cliente, não no orçamento de CPU do cluster.
const watchInterval = 5 * time.Second

type tailnetMsg struct {
	ok  bool
	err string
}

type metricsMsg struct {
	metrics actions.ClusterMetrics
	err     string
}

type clusterMsg struct {
	snap store.ClusterSnapshot
}

type appMsg struct {
	snap store.AppSnapshot
}

type logsMsg struct {
	text string
	pod  string
	err  string
}

type deployMsg struct {
	res actions.DeployStatusResult
	err string
}

type docsMsg struct {
	results []string
	content string
	err     string
}

type actionDoneMsg struct {
	message string
	err     string
}

type tickMsg struct{}

func tick() tea.Cmd {
	return tea.Tick(watchInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func checkTailnet(env *actions.Env) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := env.Runtime.ValidateFiles(); err != nil {
			return tailnetMsg{err: err.Error()}
		}
		if err := tailnet.Require(ctx, env.Runtime.TailnetAPI, 2*time.Second); err != nil {
			return tailnetMsg{err: err.Error()}
		}
		return tailnetMsg{ok: true}
	}
}

func fetchMetrics(env *actions.Env) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		metrics, err := actions.FetchClusterMetrics(ctx, env)
		if err != nil {
			return metricsMsg{err: err.Error()}
		}
		return metricsMsg{metrics: metrics}
	}
}

func fetchCluster(env *actions.Env) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return clusterMsg{snap: store.FetchCluster(ctx, env)}
	}
}

func fetchApp(env *actions.Env, row argocd.ApplicationRow) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return appMsg{snap: store.FetchApp(ctx, env, row)}
	}
}

func fetchLogs(env *actions.Env, namespace, pod string, previous bool, tail int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		out, err := actions.AppLogsByNamespace(ctx, env, namespace, actions.AppLogsInput{
			Pod: pod, Previous: previous, Tail: tail,
		})
		if err != nil {
			return logsMsg{err: err.Error()}
		}
		return logsMsg{text: out.Logs, pod: out.Pod}
	}
}

func fetchDeploy(env *actions.Env, appName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		out, err := actions.DeployStatus(ctx, env, appName)
		if err != nil {
			return deployMsg{err: err.Error()}
		}
		return deployMsg{res: out}
	}
}

func fetchDocs(env *actions.Env, search, read string) tea.Cmd {
	return func() tea.Msg {
		out, err := actions.InfraDocs(env, search, read)
		if err != nil {
			return docsMsg{err: err.Error()}
		}
		if read != "" {
			return docsMsg{content: out.Content}
		}
		results := make([]string, 0, len(out.Results))
		for _, r := range out.Results {
			results = append(results, r.File)
		}
		return docsMsg{results: results}
	}
}

func refreshArgoCD(env *actions.Env, appName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		out, err := actions.ArgoCDRefreshByName(ctx, env, appName)
		if err != nil {
			return actionDoneMsg{err: err.Error()}
		}
		return actionDoneMsg{message: out.Message + " em " + out.Target}
	}
}
