package tui

import (
	"fmt"
	"strings"

	"github.com/gabehamasaki/infra/tools/hinfra/internal/actions"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/tui/components"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/tui/store"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/tui/styles"
)

const (
	// wideLayoutMin é a largura a partir da qual dois painéis cabem lado a lado
	// sem esmagar os gráficos.
	wideLayoutMin = 100

	// Custos de altura usados no orçamento do layout.
	panelOverhead  = 3 // borda de cima, título, borda de baixo
	resourceFixed  = 4 // 2 gauges + linha em branco + eixo do gráfico
	topPodsLines   = 8
	appsLines      = 5
	compactLines   = 8
	minChartHeight = 3
	// maxChartHeight impede que uma tela alta gere um gráfico gigante e quase
	// vazio: acima de ~8 linhas a curva não fica mais legível, só mais alta.
	maxChartHeight = 8
)

// dashboardLayout dimensiona o dashboard para a tela. Todos os painéis são
// sempre renderizados; o que o orçamento decide é só a altura dos gráficos.
type dashboardLayout struct {
	wide        bool
	panelWidth  int
	chartHeight int
}

func (m model) dashboardLayout() dashboardLayout {
	layout := dashboardLayout{wide: m.width >= wideLayoutMin, panelWidth: m.width}
	if !layout.wide {
		// Sem largura para duas colunas, um painel compacto de gauges informa
		// mais que quatro painéis com gráficos truncados.
		return layout
	}

	layout.panelWidth = m.width / 2
	const rows = 2
	spare := m.bodyHeight() - topPodsLines - appsLines - rows*(panelOverhead+resourceFixed)
	layout.chartHeight = clampChartHeight(spare / rows)
	return layout
}

// chromeHeight conta as linhas que renderChrome adiciona ao corpo.
func (m model) chromeHeight() int {
	lines := 2 // barra de status e rodapé
	if m.notice != "" {
		lines++
	}
	if m.err != "" && m.scene != sceneOffline {
		lines++
	}
	return lines
}

func (m model) renderDashboard() string {
	if len(m.metrics.Nodes) == 0 {
		return components.Panel("Dashboard", m.width, []string{
			"",
			styles.StatusMuted.Render("coletando métricas do cluster..."),
			"",
		})
	}

	node := m.metrics.Nodes[0]
	history := m.history.Node(node.Name)
	layout := m.dashboardLayout()

	var b strings.Builder
	if layout.wide {
		cpuLines := m.cpuPanel(node, history, layout)
		memoryLines := m.memoryPanel(node, history, layout)
		diskLines := m.diskPanel(node, history, layout)
		networkLines := m.networkPanel(node, history, layout)
		components.EqualizeLines(&cpuLines, &memoryLines)
		components.EqualizeLines(&diskLines, &networkLines)

		b.WriteString(components.SideBySide(
			components.Panel("CPU", layout.panelWidth, cpuLines),
			components.Panel("MEMÓRIA", layout.panelWidth, memoryLines)))
		b.WriteString("\n")
		b.WriteString(components.SideBySide(
			components.Panel("DISCO", layout.panelWidth, diskLines),
			components.Panel("REDE", layout.panelWidth, networkLines)))
	} else {
		b.WriteString(components.Panel("RECURSOS · "+node.Name, m.width,
			m.compactPanel(node, history, layout)))
	}

	b.WriteString("\n")
	b.WriteString(components.Panel("TOP PODS · CPU", m.width, m.topPodsPanel(m.width)))
	b.WriteString("\n")
	b.WriteString(components.Panel("APPLICATIONS", m.width, m.applicationsPanel()))

	return b.String()
}

// compactPanel é a versão de terminal estreito: só gauges e as taxas de rede,
// sem gráficos, para caber sem truncar nenhum número.
func (m model) compactPanel(node actions.NodeStat, history *store.NodeHistory, layout dashboardLayout) []string {
	width := chartWidth(m.width)
	lines := components.GaugeRows(width,
		components.GaugeSpec{
			Label: "cpu", Percent: node.CPUUsagePercent(),
			Detail: fmt.Sprintf("%s de %s", components.Millicores(node.CPUUsageMilli), components.Millicores(node.CPUCapacityMilli)),
		},
		components.GaugeSpec{
			Label: "reservado", Percent: node.CPUUsagePercent(), Reserved: node.CPURequestsPercent(),
			Detail: fmt.Sprintf("%s reservado", components.Millicores(node.CPURequestsMilli)),
		},
		components.GaugeSpec{
			Label: "memória", Percent: node.MemUsagePercent(),
			Detail: fmt.Sprintf("%s de %s", components.Bytes(node.MemUsageBytes), components.Bytes(node.MemCapacityBytes)),
		},
		components.GaugeSpec{
			Label: "reservado", Percent: node.MemUsagePercent(), Reserved: node.MemRequestsPercent(),
			Detail: fmt.Sprintf("%s reservado", components.Bytes(node.MemRequestsBytes)),
		},
		components.GaugeSpec{
			Label: "disco", Percent: node.DiskUsagePercent(),
			Detail: fmt.Sprintf("%s de %s", components.Bytes(node.DiskUsedBytes), components.Bytes(node.DiskCapacityBytes)),
		},
		components.GaugeSpec{
			Label: "pods", Percent: node.PodsPercent(),
			Detail: fmt.Sprintf("%d de %d", node.PodCount, node.PodCapacity),
		},
	)
	return append(lines, "",
		detailRow("rede", fmt.Sprintf("↓ %s   ↑ %s",
			components.Rate(history.NetRx.Last()), components.Rate(history.NetTx.Last()))))
}

// chartWidth desconta a moldura e o padding do painel.
func chartWidth(panelWidth int) int {
	width := panelWidth - 6
	if width < 12 {
		width = 12
	}
	return width
}

func (m model) cpuPanel(node actions.NodeStat, history *store.NodeHistory, layout dashboardLayout) []string {
	width := chartWidth(layout.panelWidth)
	lines := components.GaugeRows(width,
		components.GaugeSpec{
			Label: "uso", Percent: node.CPUUsagePercent(),
			Detail: fmt.Sprintf("%s de %s", components.Millicores(node.CPUUsageMilli), components.Millicores(node.CPUCapacityMilli)),
		},
		components.GaugeSpec{
			Label: "requests", Percent: node.CPUUsagePercent(), Reserved: node.CPURequestsPercent(),
			Detail: fmt.Sprintf("%s reservado", components.Millicores(node.CPURequestsMilli)),
		},
	)
	lines = append(lines, "")
	return append(lines, chartBlock(history.CPU.Values(), width, layout.chartHeight, 100, "100%", "0%")...)
}

func (m model) memoryPanel(node actions.NodeStat, history *store.NodeHistory, layout dashboardLayout) []string {
	width := chartWidth(layout.panelWidth)
	lines := components.GaugeRows(width,
		components.GaugeSpec{
			Label: "uso", Percent: node.MemUsagePercent(),
			Detail: fmt.Sprintf("%s de %s", components.Bytes(node.MemUsageBytes), components.Bytes(node.MemCapacityBytes)),
		},
		components.GaugeSpec{
			Label: "requests", Percent: node.MemUsagePercent(), Reserved: node.MemRequestsPercent(),
			Detail: fmt.Sprintf("%s reservado", components.Bytes(node.MemRequestsBytes)),
		},
	)
	lines = append(lines, "")
	return append(lines, chartBlock(history.Memory.Values(), width, layout.chartHeight, 100,
		components.Bytes(node.MemCapacityBytes), "0")...)
}

func (m model) diskPanel(node actions.NodeStat, history *store.NodeHistory, layout dashboardLayout) []string {
	width := chartWidth(layout.panelWidth)
	lines := []string{
		components.GaugeRow("raiz", node.DiskUsagePercent(), width,
			fmt.Sprintf("%s de %s", components.Bytes(node.DiskUsedBytes), components.Bytes(node.DiskCapacityBytes))),
		detailRow("imagens", components.Bytes(node.ImageFSUsedBytes)+" em imagens de container"),
		detailRow("pvcs", fmt.Sprintf("%s em %d volumes", components.Bytes(m.metrics.TotalPVCByte), len(m.metrics.PVCs))),
		detailRow("livre", components.Bytes(node.DiskCapacityBytes-node.DiskUsedBytes)),
		"",
	}
	// O painel de disco tem 2 linhas fixas a mais que o de CPU; encurtar o
	// gráfico mantém as duas fileiras de painéis com a mesma altura.
	return append(lines, chartBlock(history.Disk.Values(), width, max(layout.chartHeight-2, 1), 100, "100%", "0%")...)
}

func (m model) networkPanel(node actions.NodeStat, history *store.NodeHistory, layout dashboardLayout) []string {
	width := chartWidth(layout.panelWidth)
	sparkWidth := width - 22
	if sparkWidth < 8 {
		sparkWidth = 8
	}
	// Escala compartilhada entre rx e tx: sem isso, um tx pequeno pareceria
	// tão alto quanto um rx grande.
	scale := maxOf(history.NetRx.Values(), history.NetTx.Values())

	lines := []string{
		styles.Label.Render(components.PadRight("↓ rx", 6)) +
			components.PadLeft(components.Rate(history.NetRx.Last()), 11) + "  " +
			components.Sparkline(history.NetRx.Values(), sparkWidth, scale),
		styles.Label.Render(components.PadRight("↑ tx", 6)) +
			components.PadLeft(components.Rate(history.NetTx.Last()), 11) + "  " +
			components.Sparkline(history.NetTx.Values(), sparkWidth, scale),
		"",
		detailRow("total", fmt.Sprintf("↓ %s   ↑ %s",
			components.Bytes(node.NetRxBytes), components.Bytes(node.NetTxBytes))),
		"",
		components.GaugeRow("pods", node.PodsPercent(), width,
			fmt.Sprintf("%d de %d", node.PodCount, node.PodCapacity)),
	}
	// NotReady e pressure conditions aparecem na barra de status: incluí-las
	// aqui variaria a altura do painel e estouraria o orçamento do layout.
	return lines
}

func (m model) topPodsPanel(width int) []string {
	if len(m.metrics.TopPods) == 0 {
		return []string{styles.StatusMuted.Render("metrics-server não retornou dados de pod")}
	}
	limit := len(m.metrics.TopPods)
	if limit > 5 {
		limit = 5
	}
	labels := make([]string, 0, limit)
	values := make([]float64, 0, limit)
	for _, pod := range m.metrics.TopPods[:limit] {
		labels = append(labels, pod.Namespace+"/"+pod.Name)
		values = append(values, float64(pod.CPUMilli))
	}
	return components.BarChart(labels, values, width-6, func(v float64) string {
		return components.Millicores(int64(v))
	})
}

func (m model) applicationsPanel() []string {
	totals := m.appTotals()
	if totals.Total == 0 {
		return []string{styles.StatusMuted.Render("nenhum Application encontrado")}
	}
	lines := []string{
		fmt.Sprintf("%s  %s  %s",
			styles.StatusOK.Render(fmt.Sprintf("synced %d/%d", totals.Synced, totals.Total)),
			styles.StatusOK.Render(fmt.Sprintf("healthy %d/%d", totals.Healthy, totals.Total)),
			styles.StatusMuted.Render(fmt.Sprintf("projects %d · data %d · platform %d",
				totals.ByLayer["projects"], totals.ByLayer["data"], totals.ByLayer["platform"]))),
	}
	for _, app := range totals.Problems {
		lines = append(lines, styles.StatusWarn.Render(fmt.Sprintf("  ! %-24s sync=%s health=%s",
			app.Name, app.Sync, app.Health)))
	}
	return lines
}

func (m model) appTotals() store.ApplicationTotals {
	return store.SummarizeApplications(m.cluster.Applications)
}

// detailRow alinha as linhas de texto puro com a coluna das barras.
func detailRow(label, value string) string {
	return styles.Label.Render(components.PadRight(label, 10)) + styles.StatusMuted.Render(value)
}

// minSamplesForChart evita desenhar uma área com uma amostra só, que aparece
// como uma coluna solitária e sugere um gráfico quebrado.
const minSamplesForChart = 3

// chartBlock devolve o gráfico de área com os rótulos de escala embaixo, ou um
// aviso de mesma altura enquanto o histórico não tem amostras suficientes.
func chartBlock(values []float64, width, height int, max float64, topLabel, bottomLabel string) []string {
	var rows []string
	if len(values) < minSamplesForChart {
		rows = components.ChartPlaceholder(width, height, "acumulando histórico...")
	} else {
		rows = components.AreaChart(values, width, height, max)
	}
	axis := styles.StatusMuted.Render(components.PadRight(bottomLabel, width-len(topLabel)) + topLabel)
	return append(rows, axis)
}

func maxOf(series ...[]float64) float64 {
	max := 0.0
	for _, values := range series {
		for _, v := range values {
			if v > max {
				max = v
			}
		}
	}
	return max
}
