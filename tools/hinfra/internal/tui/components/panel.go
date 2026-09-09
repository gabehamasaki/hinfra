package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/tui/styles"
)

// Panel envolve conteúdo numa moldura com título, para o dashboard ter regiões
// visualmente separadas em vez de uma parede de texto.
func Panel(title string, width int, lines []string) string {
	inner := width - 4
	if inner < 10 {
		inner = 10
	}
	body := make([]string, 0, len(lines))
	for _, line := range lines {
		body = append(body, truncateStyled(line, inner))
	}
	content := styles.PanelTitle.Render(title) + "\n" + strings.Join(body, "\n")
	return styles.Border.Width(width - 2).Render(content)
}

// SideBySide junta painéis na horizontal quando há largura; o chamador decide
// quando empilhar (terminal estreito).
func SideBySide(blocks ...string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, blocks...)
}

// EqualizeLines iguala o número de linhas dos blocos com linhas em branco.
// Painéis lado a lado com conteúdos de alturas diferentes fecham a borda em
// alturas diferentes, o que parece defeito de renderização.
func EqualizeLines(blocks ...*[]string) {
	tallest := 0
	for _, block := range blocks {
		if len(*block) > tallest {
			tallest = len(*block)
		}
	}
	for _, block := range blocks {
		for len(*block) < tallest {
			*block = append(*block, "")
		}
	}
}

// truncateStyled corta respeitando a largura visível, não o número de bytes —
// necessário porque as linhas já vêm com códigos ANSI de cor.
func truncateStyled(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	var b strings.Builder
	visible := 0
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			b.WriteRune(r)
			continue
		}
		if inEscape {
			b.WriteRune(r)
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		if visible >= width-1 {
			b.WriteString("…\x1b[0m")
			break
		}
		b.WriteRune(r)
		visible++
	}
	return b.String()
}
