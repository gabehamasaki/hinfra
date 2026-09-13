package components

import (
	"strings"

	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/tui/styles"
)

// eighths vai de 1/8 a 8/8 de altura; index 0 representa célula vazia.
var eighths = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// Sparkline condensa a série numa única linha. Útil no cabeçalho de um painel,
// onde só interessa a forma da curva (subindo, estável, com pico).
func Sparkline(values []float64, width int, max float64) string {
	if width <= 0 {
		return ""
	}
	series := fitSeries(values, width)
	if max <= 0 {
		max = seriesMax(series)
	}
	if max <= 0 {
		return styles.StatusMuted.Render(strings.Repeat("▁", len(series)))
	}
	var b strings.Builder
	for _, v := range series {
		level := int(v / max * 8)
		if level < 0 {
			level = 0
		}
		if level > 8 {
			level = 8
		}
		if level == 0 && v > 0 {
			level = 1
		}
		b.WriteString(styles.ThresholdStyle(v / max * 100).Render(string(eighths[level])))
	}
	return b.String()
}

// AreaChart devolve `height` linhas formando um gráfico de área. É o gráfico
// "de verdade" do dashboard: mostra tendência com resolução vertical, coisa que
// uma sparkline de uma linha não consegue.
func AreaChart(values []float64, width, height int, max float64) []string {
	if width <= 0 || height <= 0 {
		return nil
	}
	series := fitSeries(values, width)
	if max <= 0 {
		max = seriesMax(series)
	}
	if max <= 0 {
		max = 1
	}

	rows := make([]string, height)
	for row := 0; row < height; row++ {
		var b strings.Builder
		// Linhas são desenhadas de cima para baixo; a base da coluna é a última.
		rowsFromBottom := float64(height - row - 1)
		for _, v := range series {
			cellHeight := v/max*float64(height) - rowsFromBottom
			level := int(cellHeight * 8)
			if level < 0 {
				level = 0
			}
			if level > 8 {
				level = 8
			}
			if level == 0 {
				b.WriteString(" ")
				continue
			}
			b.WriteString(styles.ThresholdStyle(v / max * 100).Render(string(eighths[level])))
		}
		rows[row] = b.String()
	}
	return rows
}

// ChartPlaceholder ocupa exatamente a altura do gráfico com uma mensagem. Sem
// isso o layout mudaria de altura entre o primeiro carregamento e o estado com
// histórico, e os painéis pulariam na tela.
func ChartPlaceholder(width, height int, message string) []string {
	rows := make([]string, height)
	for i := range rows {
		if i == height/2 {
			rows[i] = styles.StatusMuted.Render(center(message, width))
		}
	}
	return rows
}

func center(s string, width int) string {
	padding := width - len([]rune(s))
	if padding <= 0 {
		return s
	}
	return strings.Repeat(" ", padding/2) + s
}

// BarChart compara itens numa escala comum, sempre relativa ao maior valor.
func BarChart(labels []string, values []float64, width int, format func(float64) string) []string {
	if len(labels) == 0 {
		return nil
	}
	labelWidth := 0
	for _, l := range labels {
		if len(l) > labelWidth {
			labelWidth = len(l)
		}
	}
	if labelWidth > 28 {
		labelWidth = 28
	}
	max := seriesMax(values)
	if max <= 0 {
		max = 1
	}
	barWidth := width - labelWidth - 12
	if barWidth < 6 {
		barWidth = 6
	}

	rows := make([]string, 0, len(labels))
	for i, label := range labels {
		if i >= len(values) {
			break
		}
		if len(label) > labelWidth {
			label = label[:labelWidth-1] + "…"
		}
		filled := int(values[i] / max * float64(barWidth))
		if filled == 0 && values[i] > 0 {
			filled = 1
		}
		bar := styles.Accent.Render(strings.Repeat("▬", filled)) +
			styles.StatusMuted.Render(strings.Repeat("·", barWidth-filled))
		detail := ""
		if format != nil {
			detail = format(values[i])
		}
		rows = append(rows, styles.Label.Render(padRight(label, labelWidth))+" "+bar+" "+padLeft(detail, 9))
	}
	return rows
}

// fitSeries recorta a série para caber na largura, mantendo as amostras mais
// recentes, e alinha à direita quando há menos dados que colunas.
func fitSeries(values []float64, width int) []float64 {
	if len(values) == 0 {
		return make([]float64, width)
	}
	if len(values) >= width {
		return values[len(values)-width:]
	}
	out := make([]float64, width)
	copy(out[width-len(values):], values)
	return out
}

func seriesMax(values []float64) float64 {
	max := 0.0
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	return max
}
