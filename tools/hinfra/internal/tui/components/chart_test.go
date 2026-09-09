package components

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestFitSeriesMantemAmostrasRecentes(t *testing.T) {
	got := fitSeries([]float64{1, 2, 3, 4, 5}, 3)
	if len(got) != 3 || got[0] != 3 || got[2] != 5 {
		t.Fatalf("fitSeries = %v, esperava as 3 últimas amostras", got)
	}
}

func TestFitSeriesAlinhaADireitaQuandoFaltamDados(t *testing.T) {
	got := fitSeries([]float64{7, 8}, 5)
	if len(got) != 5 {
		t.Fatalf("largura = %d, esperava 5", len(got))
	}
	if got[0] != 0 || got[3] != 7 || got[4] != 8 {
		t.Fatalf("fitSeries = %v, esperava zeros à esquerda e dados à direita", got)
	}
}

func TestAreaChartRespeitaDimensoes(t *testing.T) {
	rows := AreaChart([]float64{0, 50, 100}, 3, 4, 100)
	if len(rows) != 4 {
		t.Fatalf("linhas = %d, esperava 4", len(rows))
	}
	for i, row := range rows {
		if w := lipgloss.Width(row); w != 3 {
			t.Errorf("linha %d tem largura %d, esperava 3", i, w)
		}
	}
}

func TestAreaChartValorMaximoPreencheTodasAsLinhas(t *testing.T) {
	rows := AreaChart([]float64{100}, 1, 3, 100)
	for i, row := range rows {
		if strings.TrimSpace(stripANSI(row)) == "" {
			t.Errorf("linha %d vazia; valor máximo deveria preencher a coluna toda", i)
		}
	}
}

func TestAreaChartValorZeroDeixaColunaVazia(t *testing.T) {
	rows := AreaChart([]float64{0}, 1, 3, 100)
	for i, row := range rows {
		if strings.TrimSpace(stripANSI(row)) != "" {
			t.Errorf("linha %d deveria estar vazia para valor 0, veio %q", i, row)
		}
	}
}

func TestGaugeLarguraConstante(t *testing.T) {
	for _, p := range []float64{0, 1, 42, 99.9, 100, 150} {
		if w := lipgloss.Width(Gauge(p, 20)); w != 20 {
			t.Errorf("Gauge(%v) largura = %d, esperava 20", p, w)
		}
	}
}

func TestGaugeValorPequenoAindaMostraUmaCelula(t *testing.T) {
	out := stripANSI(Gauge(0.4, 20))
	if !strings.HasPrefix(out, gaugeFilled) {
		t.Errorf("Gauge(0.4) = %q, esperava ao menos 1 célula preenchida", out)
	}
}

func TestStackedGaugeMostraUsoEReserva(t *testing.T) {
	out := stripANSI(StackedGauge(25, 75, 20))
	if len(out) == 0 {
		t.Fatal("saída vazia")
	}
	if strings.Count(out, gaugeFilled) != 5 {
		t.Errorf("células de uso = %d, esperava 5 (25%% de 20)", strings.Count(out, gaugeFilled))
	}
	if strings.Count(out, "▒") != 10 {
		t.Errorf("células reservadas = %d, esperava 10", strings.Count(out, "▒"))
	}
}

func TestBytes(t *testing.T) {
	cases := map[int64]string{
		0:            "0B",
		512:          "512B",
		1024:         "1.00KiB",
		7535067136:   "7.02GiB",
		102888095744: "95.8GiB",
	}
	for in, want := range cases {
		if got := Bytes(in); got != want {
			t.Errorf("Bytes(%d) = %q, esperava %q", in, got, want)
		}
	}
}

func TestMillicores(t *testing.T) {
	if got := Millicores(196); got != "196m" {
		t.Errorf("Millicores(196) = %q", got)
	}
	if got := Millicores(2000); got != "2.00" {
		t.Errorf("Millicores(2000) = %q", got)
	}
}

func TestUptime(t *testing.T) {
	if got := Uptime(93*time.Hour + 30*time.Minute); got != "3d21h" {
		t.Errorf("Uptime = %q, esperava 3d21h", got)
	}
	if got := Uptime(0); got != "-" {
		t.Errorf("Uptime(0) = %q, esperava -", got)
	}
}

func TestTruncateStyledPreservaLarguraVisivel(t *testing.T) {
	styled := lipgloss.NewStyle().Bold(true).Render("abcdefghij")
	out := truncateStyled(styled, 5)
	if w := lipgloss.Width(out); w > 5 {
		t.Errorf("largura = %d, esperava no máximo 5", w)
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
