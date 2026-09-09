package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/tui/components"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/tui/styles"
)

// renderChrome envolve o conteúdo da scene com barra de status no topo e
// atalhos no rodapé, as duas regiões que nunca mudam de lugar.
func (m model) renderChrome(body string) string {
	visible, indicator := m.clipBody(body)

	var b strings.Builder
	b.WriteString(m.renderStatusBar())
	b.WriteString("\n")
	b.WriteString(visible)
	b.WriteString("\n")
	if m.notice != "" {
		b.WriteString(styles.Accent.Render("→ "+m.notice) + "\n")
	}
	if m.err != "" && m.scene != sceneOffline {
		b.WriteString(styles.StatusErr.Render("! "+truncate(m.err, m.width-2)) + "\n")
	}
	b.WriteString(m.renderFooter(indicator))
	return b.String()
}

// clipBody devolve a janela visível do corpo e um indicador de posição. Recortar
// aqui garante que a view nunca passe da altura do terminal, qualquer que seja o
// tamanho do conteúdo da scene.
func (m model) clipBody(body string) (string, string) {
	lines := strings.Split(body, "\n")
	height := m.bodyHeight()
	if len(lines) <= height {
		return body, ""
	}

	start := min(max(m.scroll, 0), len(lines)-height)
	end := start + height
	indicator := fmt.Sprintf("↕ %d-%d de %d", start+1, end, len(lines))
	return strings.Join(lines[start:end], "\n"), indicator
}

func (m model) renderStatusBar() string {
	left := styles.Header.Render(" hinfra v" + version)

	connection := styles.StatusOK.Render("● tailnet")
	if m.scene == sceneOffline {
		connection = styles.StatusErr.Render("○ offline")
	}

	// Ordem = prioridade: o que fica no fim é descartado primeiro quando a
	// largura aperta.
	segments := []string{connection}
	var kubelet, uptime string
	if len(m.metrics.Nodes) > 0 {
		node := m.metrics.Nodes[0]
		name := node.Name
		if !node.Ready {
			name = styles.StatusErr.Render(node.Name + " NotReady")
		} else if len(node.Conditions) > 0 {
			name = styles.StatusWarn.Render(node.Name + " " + strings.Join(node.Conditions, ","))
		}
		segments = append(segments, name)
		kubelet = node.KubeletVersion
		uptime = "up " + components.Uptime(node.Uptime)
	}
	if totals := m.appTotals(); totals.Total > 0 {
		style := styles.StatusOK
		if len(totals.Problems) > 0 {
			style = styles.StatusWarn
		}
		segments = append(segments, style.Render(fmt.Sprintf("apps %d/%d", totals.Healthy, totals.Total)))
	}
	if !m.metrics.FetchedAt.IsZero() {
		// Clamp em zero porque skew de relógio produziria idade negativa.
		age := max(int(time.Since(m.metrics.FetchedAt).Seconds()), 0)
		label := fmt.Sprintf("%ds", age)
		if age > 60 {
			label = styles.StatusWarn.Render(label + " stale")
		}
		segments = append(segments, label)
	}
	if m.loading {
		segments = append(segments, styles.StatusMuted.Render("⟳"))
	}
	if m.watch {
		segments = append(segments, styles.Accent.Render("WATCH"))
	}
	segments = append(segments, kubelet, uptime)
	if m.cwdApp != "" {
		segments = append(segments, styles.Accent.Render("cwd:"+m.cwdApp))
	}

	available := m.width - lipgloss.Width(left) - 2
	return left + "  " + joinWithin(available, styles.StatusMuted.Render(" · "), segments)
}

func (m model) renderFooter(scrollIndicator string) string {
	perScene := map[scene]string{
		sceneDashboard: "j/k rola  d docs  g cwd",
		sceneNodes:     "j/k rola  n próximo node",
		sceneStorage:   "j/k rola",
		sceneAppList:   "j/k  enter detalhe  / filtro  p camada",
		sceneAppDetail: "j/k rola  l logs  d deploy  a refresh  esc volta",
		sceneLogs:      "j/k rola  p previous  t tail  esc volta",
		sceneDeploy:    "esc volta",
		sceneDocs:      "/ buscar  enter abrir  esc volta",
		sceneOffline:   "enter tentar de novo",
	}
	watch := "w watch"
	if m.watch {
		watch = "w watch on"
	}
	nav := "1 dash  2 nodes  3 storage  4 apps"
	data := "r refresh  " + watch + "  ? ajuda  q sair"

	reserved := 1
	if scrollIndicator != "" {
		reserved += lipgloss.Width(scrollIndicator) + 2
	}
	available := m.width - reserved

	// A ordem de exibição é sempre a mesma; o que muda é quanto se descarta
	// quando falta largura. Descartar da esquerda mudaria de lugar os atalhos
	// que o usuário já decorou.
	body := data
	for _, candidate := range [][]string{
		{nav, perScene[m.scene], data},
		{nav, data},
		{data},
	} {
		if line := joinAll("  │  ", candidate); lipgloss.Width(line) <= available {
			body = line
			break
		}
	}

	footer := styles.StatusMuted.Render(" " + body)
	if scrollIndicator == "" {
		return footer
	}
	// O indicador vai à direita, longe dos atalhos: ali ele lê como "tem mais
	// conteúdo fora da tela" em vez de virar outro atalho.
	gap := max(m.width-lipgloss.Width(footer)-lipgloss.Width(scrollIndicator), 1)
	return footer + strings.Repeat(" ", gap) + styles.Accent.Render(scrollIndicator)
}

// joinAll junta os segmentos não vazios, sem deixar separador solto quando a
// scene não tem dica própria.
func joinAll(separator string, segments []string) string {
	var kept []string
	for _, segment := range segments {
		if segment != "" {
			kept = append(kept, segment)
		}
	}
	return strings.Join(kept, separator)
}

// joinWithin junta os segmentos que couberem na largura, na ordem dada. Os
// primeiros são os mais importantes; o resto é descartado em vez de deixar a
// linha quebrar, porque uma linha de chrome que quebra rouba uma linha do corpo
// e o conteúdo do topo sai da tela.
func joinWithin(width int, separator string, segments []string) string {
	separatorWidth := lipgloss.Width(separator)
	var kept []string
	used := 0
	for _, segment := range segments {
		if segment == "" {
			continue
		}
		cost := lipgloss.Width(segment)
		if len(kept) > 0 {
			cost += separatorWidth
		}
		if used+cost > width {
			break
		}
		used += cost
		kept = append(kept, segment)
	}
	return strings.Join(kept, separator)
}

func (m model) renderConfirm() string {
	return components.Panel("Confirmar", m.width, []string{
		"",
		m.confirmMsg,
		"",
		styles.Accent.Render("[y]") + " confirmar    " + styles.StatusMuted.Render("[n/esc]") + " cancelar",
		"",
	})
}

func renderBoot() string {
	return "\n  " + styles.StatusMuted.Render("validando config, kubeconfig e tailnet...")
}

func (m model) renderOffline() string {
	return components.Panel("Sem acesso ao cluster", m.width, []string{
		"",
		styles.StatusErr.Render(m.err),
		"",
		"O cluster só responde dentro da tailnet. Verifique:",
		"  " + styles.Label.Render("tailscale up"),
		"  " + styles.Label.Render("hinfra doctor"),
		"",
		styles.Accent.Render("[enter]") + " tentar de novo    " + styles.StatusMuted.Render("[q]") + " sair",
		"",
	})
}

// helpText é maior que muitos terminais, por isso a ajuda rola em vez de ser
// cortada — atalho escondido é atalho que não existe.
func helpText() string {
	sections := []string{
		styles.PanelTitle.Render("Navegação"),
		"  1        dashboard com gráficos de CPU, memória, disco e rede",
		"  2        detalhe por node",
		"  3        armazenamento (disco do node e PVCs)",
		"  4        applications do ArgoCD",
		"  esc      volta um nível",
		"",
		styles.PanelTitle.Render("Rolagem"),
		"  j/k      rola linha por linha (onde não há lista)",
		"  pgup/pgdn, ctrl+u/ctrl+d   rola meia tela",
		"  home/end rola ao topo ou ao fim",
		styles.StatusMuted.Render("  Nada é omitido por falta de espaço: o indicador ↕ no rodapé"),
		styles.StatusMuted.Render("  mostra a posição quando há mais conteúdo do que caberia."),
		"",
		styles.PanelTitle.Render("Dados"),
		"  r        recarrega a scene atual",
		"  w        watch: recarrega a cada 5s (começa ligado)",
		"  n        próximo node (scene 2)",
		"",
		styles.PanelTitle.Render("Apps"),
		"  j/k      move o cursor",
		"  enter    abre o detalhe",
		"  /        filtra por nome",
		"  p        alterna camada (projects/data/platform)",
		"  g        pula para o app do diretório atual",
		"  l        logs do pod",
		"  d        pipeline de deploy (4 elos)",
		"  a        refresh hard no ArgoCD (pede confirmação)",
		"",
		styles.PanelTitle.Render("Logs"),
		"  p        alterna --previous (CrashLoopBackOff)",
		"  t        alterna tail entre 50, 100 e 500 linhas",
		"",
		styles.StatusMuted.Render("Mutações destrutivas (restart, rollback) ficam na CLI:"),
		styles.StatusMuted.Render("  hinfra restart --app <nome>"),
		styles.StatusMuted.Render("  hinfra rollback --app <nome> --confirm"),
	}
	return strings.Join(sections, "\n")
}

func (m model) renderHelp() string {
	// View recebe o modelo por valor, então dimensionar aqui é seguro e garante
	// que a ajuda caiba mesmo sem um resize anterior. SetContent preserva a
	// posição de rolagem, então re-aplicá-lo a cada frame não atrapalha.
	m.helpViewport.Width = max(m.width-4, 20)
	m.helpViewport.Height = max(m.height-m.chromeHeight()-panelOverhead, 3)
	m.helpViewport.SetContent(helpText())
	return components.Panel("Ajuda · j/k rola · esc fecha", m.width,
		strings.Split(m.helpViewport.View(), "\n"))
}

func renderTooSmall(width, height int) string {
	return styles.StatusErr.Render(fmt.Sprintf(
		"terminal %dx%d é pequeno demais\nmínimo: %dx%d",
		width, height, minWidth, minHeight,
	))
}

func truncate(s string, width int) string {
	if width <= 1 || len(s) <= width {
		return s
	}
	return s[:width-1] + "…"
}
