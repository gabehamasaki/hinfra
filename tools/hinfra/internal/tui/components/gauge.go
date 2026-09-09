package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/tui/styles"
)

const (
	gaugeFilled   = "█"
	gaugeReserved = "▒"
	gaugeEmpty    = "░"

	// labelWidth cabe "reservado", o maior rótulo usado nos painéis.
	labelWidth = 9
	pctWidth   = 4
	minBar     = 8
)

// Gauge desenha uma barra de ocupação com cor definida pelo próprio percentual,
// para o olho achar o recurso saturado antes de ler o número.
func Gauge(percent float64, width int) string {
	if width < 3 {
		width = 3
	}
	percent = clampPercent(percent)
	filled := cellsFor(percent, width)
	return styles.ThresholdStyle(percent).Render(strings.Repeat(gaugeFilled, filled)) +
		styles.StatusMuted.Render(strings.Repeat(gaugeEmpty, width-filled))
}

// StackedGauge sobrepõe duas séries na mesma barra: uso real e requests
// reservados. Nesta infra o orçamento de requests satura antes do uso real,
// então ver os dois juntos é o que evita decisão errada de capacidade.
func StackedGauge(usedPercent, reservedPercent float64, width int) string {
	if width < 4 {
		width = 4
	}
	usedPercent = clampPercent(usedPercent)
	reservedPercent = clampPercent(reservedPercent)
	usedCells := cellsFor(usedPercent, width)
	reservedCells := cellsFor(reservedPercent, width)

	var b strings.Builder
	for i := 0; i < width; i++ {
		switch {
		case i < usedCells:
			b.WriteString(styles.ThresholdStyle(usedPercent).Render(gaugeFilled))
		case i < reservedCells:
			b.WriteString(styles.Reserved.Render(gaugeReserved))
		default:
			b.WriteString(styles.StatusMuted.Render(gaugeEmpty))
		}
	}
	return b.String()
}

// GaugeSpec descreve uma linha de gauge. Reserved > 0 pede a barra empilhada.
type GaugeSpec struct {
	Label    string
	Percent  float64
	Reserved float64
	Detail   string
}

// GaugeRows renderiza um grupo com uma única largura de barra para todas as
// linhas. Dimensionar cada linha isoladamente faz as barras terminarem em
// colunas diferentes, e o painel passa a parecer desalinhado.
func GaugeRows(totalWidth int, specs ...GaugeSpec) []string {
	detailWidth := 0
	for _, spec := range specs {
		if w := lipgloss.Width(spec.Detail); w > detailWidth {
			detailWidth = w
		}
	}

	overhead := labelWidth + 1 + pctWidth
	if detailWidth > 0 {
		overhead += 2 + detailWidth
	}
	barWidth := totalWidth - overhead
	if barWidth < minBar {
		barWidth = minBar
	}

	rows := make([]string, 0, len(specs))
	for _, spec := range specs {
		bar := Gauge(spec.Percent, barWidth)
		// O percentual mostrado acompanha a série dominante da barra.
		shown := spec.Percent
		if spec.Reserved > 0 {
			bar = StackedGauge(spec.Percent, spec.Reserved, barWidth)
			shown = spec.Reserved
		}
		row := styles.Label.Render(padRight(spec.Label, labelWidth)) + " " + bar + " " +
			styles.ThresholdStyle(clampPercent(shown)).Render(padLeft(Percent(shown), pctWidth))
		if detailWidth > 0 {
			row += "  " + styles.StatusMuted.Render(padRight(spec.Detail, detailWidth))
		}
		rows = append(rows, row)
	}
	return rows
}

// GaugeRow é o atalho para uma linha só.
func GaugeRow(label string, percent float64, totalWidth int, detail string) string {
	return GaugeRows(totalWidth, GaugeSpec{Label: label, Percent: percent, Detail: detail})[0]
}

// cellsFor evita que um valor pequeno mas diferente de zero desapareça.
func cellsFor(percent float64, width int) int {
	cells := int(percent / 100 * float64(width))
	if cells == 0 && percent > 0 {
		cells = 1
	}
	if cells > width {
		cells = width
	}
	return cells
}

func clampPercent(p float64) float64 {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func padRight(s string, width int) string {
	if lipgloss.Width(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-lipgloss.Width(s))
}

func padLeft(s string, width int) string {
	if lipgloss.Width(s) >= width {
		return s
	}
	return strings.Repeat(" ", width-lipgloss.Width(s)) + s
}

// PadRight e PadLeft são reexportados para as scenes alinharem tabelas.
func PadRight(s string, width int) string { return padRight(s, width) }
func PadLeft(s string, width int) string  { return padLeft(s, width) }
