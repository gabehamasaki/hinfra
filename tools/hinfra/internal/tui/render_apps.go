package tui

import (
	"fmt"
	"strings"

	"github.com/gabehamasaki/infra/tools/hinfra/internal/argocd"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/tui/components"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/tui/styles"
)

// layerCycle percorre as camadas e volta para "todas", para uma tecla só dar
// conta de filtrar sem submenu.
func layerCycle(current argocd.Layer) argocd.Layer {
	switch current {
	case "":
		return argocd.LayerProjects
	case argocd.LayerProjects:
		return argocd.LayerData
	case argocd.LayerData:
		return argocd.LayerPlatform
	default:
		return ""
	}
}

func (m model) renderAppList() string {
	header := "todas as camadas"
	if m.layer != "" {
		header = "camada " + string(m.layer)
	}

	rows := []string{
		styles.Label.Render(fmt.Sprintf("%s %s %s %s",
			components.PadRight("APP", 26),
			components.PadRight("SYNC", 11),
			components.PadRight("HEALTH", 12),
			"CAMADA")),
	}

	if len(m.apps) == 0 {
		rows = append(rows, styles.StatusMuted.Render("nenhum Application com esse filtro"))
	}
	for i, app := range m.apps {
		line := fmt.Sprintf("%s %s %s %s",
			components.PadRight(app.Name, 26),
			styles.SyncStyle(app.Sync).Render(components.PadRight(app.Sync, 11)),
			styles.HealthStyle(app.Health).Render(components.PadRight(app.Health, 12)),
			styles.StatusMuted.Render(string(app.Layer)))
		if i == m.cursor {
			line = styles.Selected.Render("▸ " + line)
		} else {
			line = "  " + line
		}
		rows = append(rows, line)
	}
	if m.filterInput.Focused() {
		rows = append(rows, "", m.filterInput.View())
	} else if m.filter != "" {
		rows = append(rows, "", styles.StatusMuted.Render("filtro: "+m.filter))
	}

	return components.Panel("Applications · "+header, m.width, rows)
}

func (m model) renderAppDetail() string {
	snap := m.appSnap
	var b strings.Builder

	lines := []string{
		styles.Label.Render(components.PadRight("app", 12)) + m.selected.Name,
		styles.Label.Render(components.PadRight("namespace", 12)) + m.selectedNamespace(),
		styles.Label.Render(components.PadRight("sync", 12)) + styles.SyncStyle(m.selected.Sync).Render(m.selected.Sync),
		styles.Label.Render(components.PadRight("health", 12)) + styles.HealthStyle(m.selected.Health).Render(m.selected.Health),
		styles.Label.Render(components.PadRight("revision", 12)) + styles.StatusMuted.Render(shortSHA(m.selected.Revision)),
		styles.Label.Render(components.PadRight("path", 12)) + styles.StatusMuted.Render(m.selected.SourcePath),
	}
	b.WriteString(components.Panel("Application", m.width, lines))
	b.WriteString("\n")

	podRows := []string{}
	if len(snap.Health.Pods) == 0 {
		podRows = append(podRows, styles.StatusMuted.Render("nenhum pod no namespace"))
	}
	for _, pod := range snap.Health.Pods {
		phase := styles.HealthStyle(podPhaseHealth(pod.Phase)).Render(components.PadRight(pod.Phase, 11))
		reason := pod.WaitingReason
		if reason == "" {
			reason = pod.TerminatedReason
		}
		row := fmt.Sprintf("%s %s restarts=%d",
			components.PadRight(pod.Name, 34), phase, pod.Restarts)
		if reason != "" {
			row += "  " + styles.StatusWarn.Render(reason)
		}
		podRows = append(podRows, row)
	}
	b.WriteString(components.Panel("Pods", m.width, podRows))

	// O consumo por pod vem do metrics-server, que indexa por namespace.
	if usage := m.podUsageFor(m.selectedNamespace()); len(usage) > 0 && m.height >= 32 {
		b.WriteString("\n")
		labels := make([]string, 0, len(usage))
		values := make([]float64, 0, len(usage))
		for _, pod := range usage {
			labels = append(labels, pod.Name)
			values = append(values, float64(pod.CPUMilli))
		}
		b.WriteString(components.Panel("CPU por pod", m.width,
			components.BarChart(labels, values, chartWidth(m.width), func(v float64) string {
				return components.Millicores(int64(v))
			})))
	}

	if len(snap.Health.Events) > 0 && m.height >= 38 {
		b.WriteString("\n")
		events := snap.Health.Events
		if len(events) > 5 {
			events = events[len(events)-5:]
		}
		b.WriteString(components.Panel("Eventos recentes", m.width, events))
	}

	return b.String()
}

func (m model) podUsageFor(namespace string) []struct {
	Name     string
	CPUMilli int64
} {
	var out []struct {
		Name     string
		CPUMilli int64
	}
	for _, pod := range m.metrics.TopPods {
		if pod.Namespace != namespace {
			continue
		}
		out = append(out, struct {
			Name     string
			CPUMilli int64
		}{pod.Name, pod.CPUMilli})
	}
	return out
}

func (m model) renderLogs() string {
	previous := "off"
	if m.logPrevious {
		previous = styles.StatusWarn.Render("on")
	}
	title := fmt.Sprintf("Logs · %s · tail %d · previous %s", m.logPod, m.logTail, previous)
	return components.Panel(title, m.width, strings.Split(m.logViewport.View(), "\n"))
}

// deploySteps descreve os quatro elos da cadeia de deploy, na ordem em que
// actions.DeployStatus os verifica.
var deploySteps = []string{
	"CI construiu a imagem (SHA no registry)",
	"commit de bump chegou ao repo infra",
	"ArgoCD sincronizou o Application",
	"pods rodam a imagem com esse SHA",
}

func (m model) renderDeploy() string {
	res := m.deploy
	lines := []string{
		styles.Label.Render(components.PadRight("app", 10)) + m.selected.Name,
		styles.Label.Render(components.PadRight("sha", 10)) + styles.StatusMuted.Render(shortSHA(res.SHA)),
		"",
	}
	for i, description := range deploySteps {
		step := i + 1
		var marker, text string
		switch {
		case step < res.Step || (step == res.Step && res.OK):
			marker = styles.StatusOK.Render("✓")
			text = description
		case step == res.Step:
			marker = styles.StatusErr.Render("✗")
			text = styles.StatusErr.Render(description)
		default:
			marker = styles.StatusMuted.Render("·")
			text = styles.StatusMuted.Render(description)
		}
		lines = append(lines, fmt.Sprintf(" %s %d. %s", marker, step, text))
	}
	lines = append(lines, "")
	if res.OK {
		lines = append(lines, styles.StatusOK.Render("deploy completo"))
	} else if res.Detail != "" {
		// O detalhe é a única pista de qual elo quebrou; quebrar em linhas
		// evita perder o final da mensagem no truncamento do painel.
		lines = append(lines, styles.StatusWarn.Render("primeiro elo que falhou:"))
		lines = append(lines, wrap(res.Detail, chartWidth(m.width))...)
	}
	return components.Panel("Pipeline de deploy", m.width, lines)
}

func (m model) renderDocs() string {
	if m.docsContent != "" {
		return components.Panel("docs/", m.width, strings.Split(m.docViewport.View(), "\n"))
	}
	rows := []string{}
	if m.docsInput.Focused() {
		rows = append(rows, m.docsInput.View(), "")
	}
	if len(m.docsResults) == 0 {
		rows = append(rows, styles.StatusMuted.Render("pressione / para buscar em docs/"))
	}
	for i, result := range m.docsResults {
		if i == m.docsCursor {
			rows = append(rows, styles.Selected.Render("▸ "+result))
		} else {
			rows = append(rows, "  "+result)
		}
	}
	return components.Panel("Documentação", m.width, rows)
}

func podPhaseHealth(phase string) string {
	switch phase {
	case "Running", "Succeeded":
		return "Healthy"
	case "Pending":
		return "Progressing"
	default:
		return "Unhealthy"
	}
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	if sha == "" {
		return "-"
	}
	return sha
}

// wrap quebra em palavras para o texto de erro caber no painel.
func wrap(s string, width int) []string {
	if width < 20 {
		width = 20
	}
	words := strings.Fields(s)
	var lines []string
	current := ""
	for _, word := range words {
		if current == "" {
			current = word
			continue
		}
		if len(current)+1+len(word) > width {
			lines = append(lines, current)
			current = word
			continue
		}
		current += " " + word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
