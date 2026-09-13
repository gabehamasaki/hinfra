package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/actions"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/argocd"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/config"
	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/tui/store"
)

const (
	minWidth  = 80
	minHeight = 24
	version   = "0.1.0"
)

type scene int

const (
	sceneBoot scene = iota
	sceneOffline
	sceneDashboard
	sceneNodes
	sceneStorage
	sceneAppList
	sceneAppDetail
	sceneLogs
	sceneDeploy
	sceneDocs
	sceneHelp
	sceneConfirm
)

type model struct {
	env    *actions.Env
	width  int
	height int
	scene  scene
	// prevScene permite que help e confirm voltem para onde estavam.
	prevScene scene

	err     string
	notice  string
	loading bool
	watch   bool
	// scroll é a primeira linha visível do corpo. Conteúdo que não cabe rola,
	// em vez de ser omitido: painel escondido é informação perdida em silêncio.
	scroll int

	// history é ponteiro porque o modelo é copiado em cada Update; copiar os
	// ring buffers a cada tecla seria desperdício.
	history *store.History
	metrics actions.ClusterMetrics
	cluster store.ClusterSnapshot

	cwdApp   string
	layer    argocd.Layer
	filter   string
	cursor   int
	apps     []argocd.ApplicationRow
	selected argocd.ApplicationRow
	appSnap  store.AppSnapshot

	nodeCursor int

	logPod      string
	logTail     int
	logPrevious bool

	deploy actions.DeployStatusResult

	docsResults []string
	docsContent string
	docsCursor  int

	filterInput  textinput.Model
	docsInput    textinput.Model
	logViewport  viewport.Model
	docViewport  viewport.Model
	helpViewport viewport.Model

	confirmMsg    string
	confirmAction tea.Cmd
}

func Run(env *actions.Env) error {
	if env.Runtime == nil {
		runtime, err := config.Load()
		if err != nil {
			return err
		}
		env.Runtime = runtime
	}
	program := tea.NewProgram(newModel(env), tea.WithAltScreen())
	_, err := program.Run()
	return err
}

func newModel(env *actions.Env) model {
	filter := textinput.New()
	filter.Placeholder = "filtrar apps"
	filter.Prompt = "/"

	docs := textinput.New()
	docs.Placeholder = "buscar em docs/"
	docs.Prompt = "/"

	m := model{
		env:   env,
		scene: sceneBoot,
		// Watch ligado por padrão: um dashboard de consumo só é útil se os
		// números acompanharem o cluster, e é ele que preenche o histórico.
		watch:        true,
		history:      store.NewHistory(),
		logTail:      100,
		filterInput:  filter,
		docsInput:    docs,
		logViewport:  viewport.New(minWidth, 10),
		docViewport:  viewport.New(minWidth, 10),
		helpViewport: viewport.New(minWidth, 10),
	}
	// Resolver o app do cwd falha fora de um repo de projeto, e isso é normal:
	// o dashboard funciona sem contexto de app.
	if app, err := actions.ResolveApp(env, ""); err == nil {
		m.cwdApp = app.Name
	}
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(checkTailnet(m.env), tick())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeViewports()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tailnetMsg:
		if !msg.ok {
			m.err = msg.err
			m.scene = sceneOffline
			return m, nil
		}
		m.err = ""
		m.scene = sceneDashboard
		m.loading = true
		return m, tea.Batch(fetchMetrics(m.env), fetchCluster(m.env))

	case metricsMsg:
		m.loading = false
		if msg.err != "" {
			m.err = msg.err
			return m, nil
		}
		m.err = ""
		m.metrics = msg.metrics
		m.history.Record(msg.metrics)
		if m.nodeCursor >= len(m.metrics.Nodes) {
			m.nodeCursor = 0
		}
		return m, nil

	case clusterMsg:
		m.cluster = msg.snap
		if msg.snap.Err != nil {
			m.err = msg.snap.Err.Error()
		}
		m.apps = m.filteredApps()
		if m.cursor >= len(m.apps) {
			m.cursor = 0
		}
		return m, nil

	case appMsg:
		m.loading = false
		m.appSnap = msg.snap
		if msg.snap.Err != nil {
			m.err = msg.snap.Err.Error()
		}
		return m, nil

	case logsMsg:
		m.loading = false
		if msg.err != "" {
			m.err = msg.err
			return m, nil
		}
		m.err = ""
		m.logPod = msg.pod
		m.logViewport.SetContent(msg.text)
		m.logViewport.GotoBottom()
		return m, nil

	case deployMsg:
		m.loading = false
		if msg.err != "" {
			m.err = msg.err
			return m, nil
		}
		m.deploy = msg.res
		return m, nil

	case docsMsg:
		m.loading = false
		if msg.err != "" {
			m.err = msg.err
			return m, nil
		}
		m.err = ""
		if msg.content != "" {
			m.docsContent = msg.content
			m.docViewport.SetContent(msg.content)
			m.docViewport.GotoTop()
		} else {
			m.docsResults = msg.results
			m.docsCursor = 0
			m.docsContent = ""
		}
		return m, nil

	case actionDoneMsg:
		m.loading = false
		if msg.err != "" {
			m.err = msg.err
		} else {
			m.notice = msg.message
		}
		return m, nil

	case tickMsg:
		if m.watch && m.scene != sceneOffline && m.scene != sceneBoot {
			return m, tea.Batch(tick(), m.refresh())
		}
		return m, tick()
	}

	return m.forwardToViewport(msg)
}

// resizeViewports reserva as linhas do chrome e da moldura do painel para que
// o conteúdo rolável nunca empurre o rodapé fora da tela.
func (m *model) resizeViewports() {
	height := m.height - m.chromeHeight() - panelOverhead
	if height < 3 {
		height = 3
	}
	width := m.width - 4
	if width < 20 {
		width = 20
	}
	for _, vp := range []*viewport.Model{&m.logViewport, &m.docViewport, &m.helpViewport} {
		vp.Width, vp.Height = width, height
	}
}

func (m model) forwardToViewport(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.scene {
	case sceneLogs:
		m.logViewport, cmd = m.logViewport.Update(msg)
	case sceneDocs:
		m.docViewport, cmd = m.docViewport.Update(msg)
	}
	return m, cmd
}

// refresh busca só o que a scene atual mostra, evitando chamadas inúteis.
func (m model) refresh() tea.Cmd {
	switch m.scene {
	case sceneDashboard, sceneNodes, sceneStorage:
		return tea.Batch(fetchMetrics(m.env), fetchCluster(m.env))
	case sceneAppList:
		return fetchCluster(m.env)
	case sceneAppDetail:
		return fetchApp(m.env, m.selected)
	case sceneLogs:
		return fetchLogs(m.env, m.selectedNamespace(), "", m.logPrevious, m.logTail)
	case sceneDeploy:
		return fetchDeploy(m.env, m.selected.Name)
	default:
		return fetchMetrics(m.env)
	}
}

func (m model) selectedNamespace() string {
	if m.selected.Namespace != "" {
		return m.selected.Namespace
	}
	return m.selected.Name
}

func (m model) filteredApps() []argocd.ApplicationRow {
	return store.FilterApplications(m.cluster.Applications, m.layer, m.filter)
}

func (m model) View() string {
	m = m.withFallbackSize()
	if m.width < minWidth || m.height < minHeight {
		return renderTooSmall(m.width, m.height)
	}
	return m.renderChrome(m.sceneBody())
}

// withFallbackSize cobre pty sem tamanho definido, que nunca emite
// WindowSizeMsg; sem isso a TUI ficaria presa numa tela vazia sem saída.
func (m model) withFallbackSize() model {
	if m.width == 0 || m.height == 0 {
		m.width, m.height = minWidth, minHeight
	}
	return m
}

// sceneBody renderiza só o conteúdo da scene, sem barra de status nem rodapé.
// É função pura do modelo, então o tratamento de teclas pode medi-la para
// limitar a rolagem sem depender de estado deixado pelo último render.
func (m model) sceneBody() string {
	switch m.scene {
	case sceneHelp:
		return m.renderHelp()
	case sceneConfirm:
		return m.renderConfirm()
	case sceneBoot:
		return renderBoot()
	case sceneOffline:
		return m.renderOffline()
	case sceneDashboard:
		return m.renderDashboard()
	case sceneNodes:
		return m.renderNodes()
	case sceneStorage:
		return m.renderStorage()
	case sceneAppList:
		return m.renderAppList()
	case sceneAppDetail:
		return m.renderAppDetail()
	case sceneLogs:
		return m.renderLogs()
	case sceneDeploy:
		return m.renderDeploy()
	case sceneDocs:
		return m.renderDocs()
	default:
		return ""
	}
}

// bodyHeight é o espaço vertical que resta para o corpo depois do chrome.
func (m model) bodyHeight() int {
	return max(m.height-m.chromeHeight(), 1)
}

// maxScroll é a maior primeira-linha-visível que ainda mostra conteúdo.
func (m model) maxScroll() int {
	return max(len(strings.Split(m.sceneBody(), "\n"))-m.bodyHeight(), 0)
}

// scrollBy move a janela visível e trava nos limites do conteúdo, para não
// existirem posições de rolagem que mostram tela vazia.
func (m model) scrollBy(delta int) model {
	m.scroll = min(max(m.scroll+delta, 0), m.maxScroll())
	return m
}
