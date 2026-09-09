package tui

import (
	"fmt"
	"strings"

	"github.com/gabehamasaki/infra/tools/hinfra/internal/actions"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/tui/components"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/tui/styles"
)

func (m model) renderNodes() string {
	if len(m.metrics.Nodes) == 0 {
		return components.Panel("Nodes", m.width, []string{
			"", styles.StatusMuted.Render("coletando métricas..."), "",
		})
	}

	cursor := m.nodeCursor
	if cursor >= len(m.metrics.Nodes) {
		cursor = 0
	}
	node := m.metrics.Nodes[cursor]
	history := m.history.Node(node.Name)
	width := chartWidth(m.width)

	stack := newPanelStack(m.width, m.bodyHeight())

	// Com mais de um node, a lista precede o detalhe para o cursor fazer sentido.
	if len(m.metrics.Nodes) > 1 {
		stack.Add("Nodes", m.nodeListRows())
	}
	stack.Add("Node "+node.Name, m.nodeDetailRows(node, width))

	h := chartHeightFor(stack.Remaining(), 2)
	stack.Add("Histórico CPU %", chartBlock(history.CPU.Values(), width, h, 100, "100%", "0%"))
	stack.Add("Histórico Memória %", chartBlock(history.Memory.Values(), width, h, 100, "100%", "0%"))

	return stack.String()
}

func (m model) nodeListRows() []string {
	rows := make([]string, 0, len(m.metrics.Nodes))
	for i, node := range m.metrics.Nodes {
		marker := "  "
		if i == m.nodeCursor {
			marker = styles.Accent.Render("▸ ")
		}
		state := styles.StatusOK.Render("Ready")
		if !node.Ready {
			state = styles.StatusErr.Render("NotReady")
		}
		rows = append(rows, fmt.Sprintf("%s%s  %s  cpu %s  mem %s  disco %s",
			marker,
			components.PadRight(node.Name, 18),
			state,
			components.PadLeft(components.Percent(node.CPUUsagePercent()), 4),
			components.PadLeft(components.Percent(node.MemUsagePercent()), 4),
			components.PadLeft(components.Percent(node.DiskUsagePercent()), 4),
		))
	}
	return rows
}

func (m model) nodeDetailRows(node actions.NodeStat, width int) []string {
	lines := []string{
		detailRow("host", node.Name),
		detailRow("papel", node.Roles),
		detailRow("kubelet", node.KubeletVersion),
		detailRow("so", node.OSImage),
		detailRow("uptime", components.Uptime(node.Uptime)),
		"",
	}

	// A barra empilhada é o quadro real de capacidade: nesta VPS o orçamento de
	// requests satura antes do uso instantâneo.
	lines = append(lines, components.GaugeRows(width,
		components.GaugeSpec{
			Label: "cpu", Percent: node.CPUUsagePercent(),
			Detail: fmt.Sprintf("%s de %s em uso",
				components.Millicores(node.CPUUsageMilli), components.Millicores(node.CPUCapacityMilli)),
		},
		components.GaugeSpec{
			Label: "reservado", Percent: node.CPUUsagePercent(), Reserved: node.CPURequestsPercent(),
			Detail: fmt.Sprintf("%s de %s alocáveis",
				components.Millicores(node.CPURequestsMilli), components.Millicores(node.CPUAllocMilli)),
		},
		components.GaugeSpec{
			Label: "memória", Percent: node.MemUsagePercent(),
			Detail: fmt.Sprintf("%s de %s em uso",
				components.Bytes(node.MemUsageBytes), components.Bytes(node.MemCapacityBytes)),
		},
		components.GaugeSpec{
			Label: "reservado", Percent: node.MemUsagePercent(), Reserved: node.MemRequestsPercent(),
			Detail: fmt.Sprintf("%s de %s alocáveis",
				components.Bytes(node.MemRequestsBytes), components.Bytes(node.MemAllocBytes)),
		},
		components.GaugeSpec{
			Label: "disco", Percent: node.DiskUsagePercent(),
			Detail: fmt.Sprintf("%s de %s em uso",
				components.Bytes(node.DiskUsedBytes), components.Bytes(node.DiskCapacityBytes)),
		},
		components.GaugeSpec{
			Label: "pods", Percent: node.PodsPercent(),
			Detail: fmt.Sprintf("%d de %d agendados", node.PodCount, node.PodCapacity),
		},
	)...)

	lines = append(lines, styles.StatusMuted.Render("█ uso instantâneo   ▒ requests reservados"))
	if len(node.Conditions) > 0 {
		lines = append(lines, styles.StatusWarn.Render("condições ativas: "+strings.Join(node.Conditions, ", ")))
	}
	if node.SummaryErr != "" {
		lines = append(lines, styles.StatusWarn.Render("summary API: "+truncate(node.SummaryErr, width)))
	}
	return lines
}

func (m model) renderStorage() string {
	stack := newPanelStack(m.width, m.bodyHeight())
	width := chartWidth(m.width)

	if len(m.metrics.Nodes) > 0 {
		node := m.metrics.Nodes[0]
		// "resto" é o que não está em imagens de container: volumes locais,
		// logs e o próprio sistema. A summary API não decompõe além disso.
		other := node.DiskUsedBytes - node.ImageFSUsedBytes
		if other < 0 {
			other = 0
		}
		stack.Add("Disco do node "+node.Name, []string{
			components.GaugeRow("total", node.DiskUsagePercent(), width,
				fmt.Sprintf("%s de %s", components.Bytes(node.DiskUsedBytes), components.Bytes(node.DiskCapacityBytes))),
			"",
			detailRow("imagens", components.Bytes(node.ImageFSUsedBytes)),
			detailRow("resto", components.Bytes(other)),
			detailRow("livre", styles.StatusOK.Render(components.Bytes(node.DiskCapacityBytes-node.DiskUsedBytes))),
		})
	}

	if len(m.metrics.PVCs) == 0 {
		stack.Add("PersistentVolumeClaims", []string{
			styles.StatusMuted.Render("nenhum PVC no cluster"),
		})
		return stack.String()
	}

	labels := make([]string, 0, len(m.metrics.PVCs))
	values := make([]float64, 0, len(m.metrics.PVCs))
	rows := make([]string, 0, len(m.metrics.PVCs)+2)
	for _, pvc := range m.metrics.PVCs {
		labels = append(labels, pvc.Namespace+"/"+pvc.Name)
		values = append(values, float64(pvc.CapacityByte))
		status := styles.StatusOK.Render(pvc.Status)
		if pvc.Status != "Bound" {
			status = styles.StatusErr.Render(pvc.Status)
		}
		rows = append(rows, fmt.Sprintf("%s %s %s %s",
			components.PadRight(pvc.Namespace+"/"+pvc.Name, 30),
			components.PadLeft(components.Bytes(pvc.CapacityByte), 9),
			components.PadRight(pvc.StorageClass, 12),
			status))
	}
	rows = append(rows, "", styles.Label.Render(
		fmt.Sprintf("total reservado: %s em %d volumes",
			components.Bytes(m.metrics.TotalPVCByte), len(m.metrics.PVCs))))
	stack.Add("PersistentVolumeClaims", rows)

	stack.Add("Capacidade por volume", components.BarChart(labels, values, width, func(v float64) string {
		return components.Bytes(int64(v))
	}))

	return stack.String()
}
