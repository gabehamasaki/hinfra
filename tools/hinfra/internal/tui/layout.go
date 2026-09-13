package tui

import (
	"strings"

	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/tui/components"
)

// panelStack empilha painéis e acompanha quanto da tela já foi consumido. O
// orçamento serve para dimensionar gráficos, não para descartar painéis:
// conteúdo que não cabe rola, porque painel omitido é informação que
// desaparece sem o usuário saber que existia.
type panelStack struct {
	width     int
	remaining int
	blocks    []string
}

func newPanelStack(width, height int) *panelStack {
	return &panelStack{width: width, remaining: height}
}

func (s *panelStack) Add(title string, lines []string) {
	s.remaining -= panelOverhead + len(lines)
	s.blocks = append(s.blocks, components.Panel(title, s.width, lines))
}

// Remaining pode ficar negativo quando o conteúdo passa da tela; quem
// dimensiona gráfico deve tratar isso como "use a altura mínima".
func (s *panelStack) Remaining() int { return s.remaining }

// Blocos adjacentes são unidos por "\n" porque cada painel já vem sem quebra
// final, então a junção não consome linha extra do orçamento.
func (s *panelStack) String() string { return strings.Join(s.blocks, "\n") }

// chartPanelCost é o custo em linhas de um painel que contém gráfico de altura
// h: moldura, as linhas do gráfico e a linha de eixo.
func chartPanelCost(h int) int { return panelOverhead + h + 1 }

// chartHeightFor divide o espaço restante entre n painéis de gráfico. Nunca
// devolve menos que minChartHeight: um gráfico apertado ainda mostra tendência,
// e o que passar da tela fica acessível por rolagem.
func chartHeightFor(remaining, n int) int {
	if n <= 0 {
		return 0
	}
	return clampChartHeight((remaining - n*(panelOverhead+1)) / n)
}

func clampChartHeight(h int) int {
	return min(max(h, minChartHeight), maxChartHeight)
}
