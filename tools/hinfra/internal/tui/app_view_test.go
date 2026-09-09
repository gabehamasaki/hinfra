package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/actions"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/argocd"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/config"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/tui/store"
)

// nodeFixture reproduz a VPS real: 2 vCPU, 8 GiB, disco de 96 GiB.
func nodeFixture() actions.NodeStat {
	return actions.NodeStat{
		Name:              "srv1957194",
		Ready:             true,
		Roles:             "control-plane",
		KubeletVersion:    "v1.36.4+k3s1",
		OSImage:           "Ubuntu 24.04",
		CPUCapacityMilli:  2000,
		CPUAllocMilli:     2000,
		CPUUsageMilli:     196,
		CPURequestsMilli:  820,
		MemCapacityBytes:  8326631424,
		MemAllocBytes:     8326631424,
		MemUsageBytes:     3150176256,
		MemRequestsBytes:  2147483648,
		DiskCapacityBytes: 102888095744,
		DiskUsedBytes:     7535067136,
		ImageFSUsedBytes:  3760676864,
		NetRxBytes:        48000000000,
		NetTxBytes:        13000000000,
		PodCount:          22,
		PodCapacity:       110,
		Uptime:            93 * time.Hour,
	}
}

func dashboardModel(width, height int) model {
	m := model{
		width: width, height: height, scene: sceneDashboard,
		env:     &actions.Env{Runtime: &config.Runtime{InfraRepo: "/infra"}},
		history: store.NewHistory(),
		metrics: actions.ClusterMetrics{
			FetchedAt: time.Now(),
			Nodes:     []actions.NodeStat{nodeFixture()},
			TopPods: []actions.WorkloadStat{
				{Namespace: "argocd", Name: "argocd-application-controller-0", CPUMilli: 45, MemoryBytes: 200 << 20},
				{Namespace: "kube-system", Name: "metrics-server-abc", CPUMilli: 18, MemoryBytes: 60 << 20},
			},
			PVCs: []actions.StorageStat{
				{Namespace: "rustfs", Name: "rustfs-data", Status: "Bound", CapacityByte: 5 << 30, StorageClass: "local-path"},
				{Namespace: "postgres", Name: "postgres-1", Status: "Bound", CapacityByte: 4 << 30, StorageClass: "local-path"},
			},
			TotalPVCByte: 9 << 30,
		},
		cluster: store.ClusterSnapshot{
			Applications: []argocd.ApplicationRow{
				{Name: "site", Sync: "Synced", Health: "Healthy", Layer: argocd.LayerProjects, Namespace: "site"},
				{Name: "valkey", Sync: "OutOfSync", Health: "Degraded", Layer: argocd.LayerData, Namespace: "valkey"},
			},
		},
	}
	// Duas coletas para as séries terem dados e uma taxa de rede calculável.
	m.history.Record(m.metrics)
	later := m.metrics
	later.FetchedAt = m.metrics.FetchedAt.Add(5 * time.Second)
	later.Nodes[0].NetRxBytes += 6 << 20
	m.history.Record(later)
	return m
}

func TestDashboardMostraTodosOsPaineisDeRecurso(t *testing.T) {
	out := dashboardModel(120, 40).View()
	for _, want := range []string{"CPU", "MEMÓRIA", "DISCO", "REDE"} {
		if !strings.Contains(out, want) {
			t.Errorf("dashboard não contém o painel %q", want)
		}
	}
}

func TestDashboardMostraNumerosDeCapacidade(t *testing.T) {
	out := stripANSI(dashboardModel(120, 44).View())
	// 196m de 2000m = 10%; 3.0GiB de 7.76GiB = 38%
	for _, want := range []string{"196m", "10%", "38%", "7.02GiB"} {
		if !strings.Contains(out, want) {
			t.Errorf("dashboard não mostra %q\n---\n%s", want, out)
		}
	}
}

func TestDashboardMostraRequestsSeparadoDoUso(t *testing.T) {
	out := stripANSI(dashboardModel(120, 40).View())
	if !strings.Contains(out, "requests") {
		t.Error("dashboard deveria distinguir uso real de requests reservados")
	}
	// 820m de 2000m alocáveis = 41%
	if !strings.Contains(out, "41%") {
		t.Errorf("esperava 41%% de requests de CPU\n---\n%s", out)
	}
}

func TestDashboardDesenhaGraficoDeArea(t *testing.T) {
	out := dashboardModel(120, 40).View()
	blocks := "▁▂▃▄▅▆▇█"
	found := false
	for _, r := range blocks {
		if strings.ContainsRune(out, r) {
			found = true
			break
		}
	}
	if !found {
		t.Error("dashboard não desenhou nenhum caractere de gráfico")
	}
}

func TestDashboardUsaGradeDuplaQuandoHaLargura(t *testing.T) {
	out := stripANSI(dashboardModel(140, 40).View())
	if lineWith(out, "CPU") != lineWith(out, "MEMÓRIA") {
		t.Error("em 140 colunas CPU e MEMÓRIA deveriam ficar na mesma linha")
	}
	if lineWith(out, "DISCO") != lineWith(out, "REDE") {
		t.Error("em 140 colunas DISCO e REDE deveriam ficar na mesma linha")
	}
}

func TestDashboardUsaPainelCompactoEmTerminalEstreito(t *testing.T) {
	out := stripANSI(dashboardModel(84, 40).View())
	if !strings.Contains(out, "RECURSOS") {
		t.Errorf("em 84 colunas esperava o painel compacto\n---\n%s", out)
	}
	if strings.Contains(out, "MEMÓRIA") {
		t.Error("o painel compacto não deveria abrir painéis separados por recurso")
	}
	// O ponto do modo compacto é não truncar nenhum número de recurso.
	for _, line := range panelLines(out, "RECURSOS") {
		if strings.Contains(line, "…") {
			t.Errorf("painel compacto truncou a linha %q", line)
		}
	}
}

func TestGaugesDoPainelTerminamNaMesmaColuna(t *testing.T) {
	for _, size := range [][2]int{{84, 40}, {120, 44}, {160, 50}} {
		m := dashboardModel(size[0], size[1])
		title := "RECURSOS"
		if size[0] >= wideLayoutMin {
			title = "CPU"
		}
		var columns []int
		for _, line := range panelLines(stripANSI(m.View()), title) {
			if !strings.Contains(line, "█") {
				continue
			}
			if column := runeColumn(line, "%"); column > 0 {
				columns = append(columns, column)
			}
		}
		if len(columns) < 2 {
			t.Fatalf("%dx%d: esperava ao menos 2 gauges no painel %q", size[0], size[1], title)
		}
		for _, column := range columns[1:] {
			if column != columns[0] {
				t.Errorf("%dx%d: gauges terminam em colunas diferentes: %v", size[0], size[1], columns)
				break
			}
		}
	}
}

func TestGraficoMostraAvisoEnquantoAcumulaHistorico(t *testing.T) {
	m := dashboardModel(120, 44)
	// Uma amostra só não dá para desenhar tendência.
	m.history = store.NewHistory()
	m.history.Record(m.metrics)
	out := stripANSI(m.View())
	if !strings.Contains(out, "acumulando histórico") {
		t.Errorf("esperava aviso em vez de gráfico com uma amostra\n---\n%s", out)
	}
}

// runeColumn devolve a coluna visível da substring. Índice em bytes não serve:
// os blocos do gauge e as letras acentuadas ocupam vários bytes cada.
func runeColumn(line, needle string) int {
	index := strings.Index(line, needle)
	if index < 0 {
		return -1
	}
	return len([]rune(line[:index]))
}

// panelLines devolve as linhas do painel cujo título contém a substring, sem as
// bordas laterais.
func panelLines(out, title string) []string {
	lines := strings.Split(out, "\n")
	start := -1
	for i, line := range lines {
		if strings.Contains(line, title) && strings.HasPrefix(strings.TrimSpace(line), "│") {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil
	}
	var body []string
	for _, line := range lines[start:] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "╰") {
			break
		}
		body = append(body, strings.Trim(trimmed, "│ "))
	}
	return body
}

func TestDashboardNaoEstouraAlturaDoTerminal(t *testing.T) {
	for _, width := range []int{80, 84, 100, 120, 160} {
		for height := 24; height <= 60; height++ {
			out := dashboardModel(width, height).View()
			if got := len(strings.Split(out, "\n")); got > height {
				t.Errorf("%dx%d: view usou %d linhas", width, height, got)
			}
		}
	}
}

func TestDashboardNaoEstouraAlturaComAvisoEErro(t *testing.T) {
	for height := 24; height <= 40; height++ {
		m := dashboardModel(120, height)
		m.notice = "refresh disparado"
		m.err = "argocd indisponível"
		out := m.View()
		if got := len(strings.Split(out, "\n")); got > height {
			t.Errorf("altura %d com aviso e erro: view usou %d linhas", height, got)
		}
	}
}

func TestNenhumaSceneEstouraAAlturaDoTerminal(t *testing.T) {
	scenes := map[string]scene{
		"dashboard": sceneDashboard,
		"nodes":     sceneNodes,
		"storage":   sceneStorage,
		"apps":      sceneAppList,
		"detalhe":   sceneAppDetail,
		"deploy":    sceneDeploy,
		"docs":      sceneDocs,
		"ajuda":     sceneHelp,
	}
	for name, target := range scenes {
		for _, width := range []int{80, 120, 160} {
			for height := 24; height <= 50; height += 2 {
				m := dashboardModel(width, height)
				m.scene = target
				m.apps = m.cluster.Applications
				m.selected = m.cluster.Applications[0]
				out := m.View()
				if got := len(strings.Split(out, "\n")); got > height {
					t.Errorf("%s em %dx%d: view usou %d linhas", name, width, height, got)
				}
			}
		}
	}
}

// Regressão: painéis eram omitidos quando não cabiam, então em telas menores
// informação desaparecia sem sinal nenhum. Agora tudo é renderizado e o que
// passa da tela fica acessível por rolagem.
func TestTodosOsPaineisAparecemEmQualquerTamanho(t *testing.T) {
	for _, width := range []int{80, 100, 120, 160} {
		for _, height := range []int{24, 30, 40, 60} {
			m := dashboardModel(width, height)
			body := stripANSI(m.sceneBody())
			for _, want := range []string{"TOP PODS", "APPLICATIONS"} {
				if !strings.Contains(body, want) {
					t.Errorf("%dx%d: painel %q ausente do corpo", width, height, want)
				}
			}
		}
	}
}

func TestNodesSempreRenderizaOsDoisHistoricos(t *testing.T) {
	for _, height := range []int{24, 30, 44, 60} {
		m := dashboardModel(120, height)
		m.scene = sceneNodes
		body := stripANSI(m.sceneBody())
		for _, want := range []string{"Histórico CPU", "Histórico Memória"} {
			if !strings.Contains(body, want) {
				t.Errorf("altura %d: painel %q ausente", height, want)
			}
		}
	}
}

func TestLayoutLimitaAlturaDoGrafico(t *testing.T) {
	layout := dashboardModel(120, 120).dashboardLayout()
	if layout.chartHeight > maxChartHeight {
		t.Errorf("chartHeight = %d, esperava no máximo %d", layout.chartHeight, maxChartHeight)
	}
	// Numa tela baixa o gráfico encolhe, mas nunca abaixo do mínimo legível.
	if h := dashboardModel(120, 24).dashboardLayout().chartHeight; h != minChartHeight {
		t.Errorf("chartHeight = %d em 24 linhas, esperava o mínimo %d", h, minChartHeight)
	}
}

func TestRolagemRevelaOConteudoQuePassaDaTela(t *testing.T) {
	m := dashboardModel(120, 24)
	total := len(strings.Split(m.sceneBody(), "\n"))
	if total <= m.bodyHeight() {
		t.Fatalf("cenário inválido: corpo com %d linhas cabe em %d", total, m.bodyHeight())
	}

	topo := stripANSI(m.View())
	if !strings.Contains(topo, "CPU") {
		t.Error("no topo esperava ver o painel de CPU")
	}
	if strings.Contains(topo, "APPLICATIONS") {
		t.Error("APPLICATIONS não deveria estar visível antes de rolar")
	}

	fim := stripANSI(m.scrollBy(m.maxScroll()).View())
	if !strings.Contains(fim, "APPLICATIONS") {
		t.Errorf("depois de rolar até o fim esperava ver APPLICATIONS\n---\n%s", fim)
	}
}

func TestRolagemTravaNosLimites(t *testing.T) {
	m := dashboardModel(120, 24)
	if got := m.scrollBy(-50).scroll; got != 0 {
		t.Errorf("rolar para cima no topo deu scroll=%d, esperava 0", got)
	}
	limite := m.maxScroll()
	if got := m.scrollBy(10_000).scroll; got != limite {
		t.Errorf("rolar além do fim deu scroll=%d, esperava %d", got, limite)
	}
}

// Regressão: barra de status e rodapé passavam da largura em terminais
// estreitos. A linha quebrada roubava uma linha do corpo e empurrava o topo
// para fora da tela.
func TestChromeNuncaPassaDaLargura(t *testing.T) {
	scenes := []scene{sceneDashboard, sceneNodes, sceneStorage, sceneAppList, sceneAppDetail}
	for _, target := range scenes {
		for width := 80; width <= 200; width += 4 {
			m := dashboardModel(width, 30)
			m.scene = target
			m.apps = m.cluster.Applications
			m.selected = m.cluster.Applications[0]

			lines := strings.Split(m.View(), "\n")
			for i, line := range lines {
				if got := lipgloss.Width(line); got > width {
					t.Fatalf("scene %d em %d colunas: linha %d tem largura %d\n%q",
						target, width, i, got, stripANSI(line))
				}
			}
		}
	}
}

func TestChromeMantemAtalhosEssenciaisNoEstreito(t *testing.T) {
	for _, width := range []int{80, 84, 100} {
		lines := strings.Split(stripANSI(dashboardModel(width, 30).View()), "\n")
		footer := lines[len(lines)-1]
		for _, want := range []string{"q sair", "? ajuda"} {
			if !strings.Contains(footer, want) {
				t.Errorf("rodapé em %d colunas perdeu %q: %q", width, want, footer)
			}
		}
	}
}

func TestIndicadorDeRolagemApareceSoQuandoNecessario(t *testing.T) {
	apertado := stripANSI(dashboardModel(120, 24).View())
	if !strings.Contains(apertado, "↕") {
		t.Error("esperava indicador de rolagem quando o corpo não cabe")
	}
	folgado := stripANSI(dashboardModel(120, 60).View())
	if strings.Contains(folgado, "↕") {
		t.Error("sem conteúdo excedente o indicador não deveria aparecer")
	}
}

func TestTrocarDeSceneZeraARolagem(t *testing.T) {
	m := dashboardModel(120, 24).scrollBy(5)
	if m.scroll == 0 {
		t.Fatal("cenário inválido: rolagem não avançou")
	}
	if got := m.goTo(sceneNodes).scroll; got != 0 {
		t.Errorf("scroll = %d após trocar de scene, esperava 0", got)
	}
}

func TestWatchComecaLigado(t *testing.T) {
	m := newModel(&actions.Env{Runtime: &config.Runtime{InfraRepo: "/infra"}})
	if !m.watch {
		t.Error("watch deveria começar ligado para o dashboard acompanhar o cluster")
	}
}

func TestDashboardSemMetricasNaoQuebra(t *testing.T) {
	m := model{
		width: 100, height: 30, scene: sceneDashboard,
		env:     &actions.Env{Runtime: &config.Runtime{}},
		history: store.NewHistory(),
	}
	out := m.View()
	if !strings.Contains(out, "coletando") {
		t.Errorf("esperava estado de carregamento, veio:\n%s", out)
	}
}

func TestApplicationsPanelDestacaAppComProblema(t *testing.T) {
	out := stripANSI(dashboardModel(120, 44).View())
	if !strings.Contains(out, "valkey") {
		t.Errorf("app OutOfSync deveria aparecer no painel de applications\n---\n%s", out)
	}
	if !strings.Contains(out, "healthy 1/2") {
		t.Errorf("esperava contagem healthy 1/2\n---\n%s", out)
	}
}

func TestNodesSceneMostraBarraEmpilhada(t *testing.T) {
	m := dashboardModel(120, 40)
	m.scene = sceneNodes
	out := stripANSI(m.View())
	if !strings.Contains(out, "reservado") {
		t.Errorf("scene de nodes deveria explicar a barra empilhada\n---\n%s", out)
	}
	if !strings.Contains(out, "v1.36.4+k3s1") {
		t.Error("scene de nodes deveria mostrar a versão do kubelet")
	}
}

func TestStorageSceneListaPVCs(t *testing.T) {
	m := dashboardModel(120, 40)
	m.scene = sceneStorage
	out := stripANSI(m.View())
	for _, want := range []string{"rustfs/rustfs-data", "postgres/postgres-1", "local-path"} {
		if !strings.Contains(out, want) {
			t.Errorf("scene de storage não mostra %q\n---\n%s", want, out)
		}
	}
}

func TestOfflineViewOrientaSobreTailnet(t *testing.T) {
	m := model{width: 100, height: 30, scene: sceneOffline, err: "tailscale down", history: store.NewHistory()}
	out := stripANSI(m.View())
	if !strings.Contains(out, "tailnet") || !strings.Contains(out, "tailscale up") {
		t.Errorf("view offline deveria orientar sobre a tailnet\n---\n%s", out)
	}
}

func TestViewRecusaTerminalPequeno(t *testing.T) {
	m := model{width: 40, height: 10, scene: sceneDashboard, history: store.NewHistory()}
	if !strings.Contains(m.View(), "pequeno demais") {
		t.Error("esperava aviso de terminal pequeno")
	}
}

func TestStatusBarMostraVersaoENode(t *testing.T) {
	out := stripANSI(dashboardModel(120, 40).View())
	if !strings.Contains(out, "hinfra v0.1.0") {
		t.Error("barra de status deveria mostrar a versão")
	}
	if !strings.Contains(out, "srv1957194") {
		t.Error("barra de status deveria mostrar o node")
	}
	if !strings.Contains(out, "3d21h") {
		t.Error("barra de status deveria mostrar o uptime")
	}
}

func TestDeployPipelineMarcaElosAnterioresComoOk(t *testing.T) {
	m := dashboardModel(120, 30)
	m.scene = sceneDeploy
	m.selected = argocd.ApplicationRow{Name: "site"}
	m.deploy = actions.DeployStatusResult{Step: 3, OK: false, Detail: "sync=OutOfSync", SHA: "abcdef1234567890"}
	out := stripANSI(m.View())
	if !strings.Contains(out, "✓ 1.") || !strings.Contains(out, "✓ 2.") {
		t.Errorf("elos 1 e 2 deveriam estar marcados como ok\n---\n%s", out)
	}
	if !strings.Contains(out, "✗ 3.") {
		t.Errorf("elo 3 deveria estar marcado como falha\n---\n%s", out)
	}
	if !strings.Contains(out, "abcdef123456") {
		t.Error("esperava o SHA abreviado")
	}
}

func TestLayerCycleVoltaParaTodas(t *testing.T) {
	layer := argocd.Layer("")
	seen := []argocd.Layer{}
	for i := 0; i < 4; i++ {
		layer = layerCycle(layer)
		seen = append(seen, layer)
	}
	want := []argocd.Layer{argocd.LayerProjects, argocd.LayerData, argocd.LayerPlatform, ""}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("ciclo = %v, esperava %v", seen, want)
		}
	}
}

func TestWrapQuebraSemCortarPalavra(t *testing.T) {
	lines := wrap("pod nao roda a imagem com o SHA esperado pelo repositorio infra", 25)
	for _, line := range lines {
		if len(line) > 25 {
			t.Errorf("linha excede 25 colunas: %q", line)
		}
	}
	if strings.Join(lines, " ") != "pod nao roda a imagem com o SHA esperado pelo repositorio infra" {
		t.Errorf("wrap perdeu conteúdo: %v", lines)
	}
}

// lineWith devolve o índice da primeira linha que contém a substring, ou -1.
func lineWith(out, needle string) int {
	for i, line := range strings.Split(out, "\n") {
		if strings.Contains(line, needle) {
			return i
		}
	}
	return -1
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
