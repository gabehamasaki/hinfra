package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// handleKey resolve as teclas em três níveis: modais capturam tudo, depois os
// atalhos globais, e só então a scene atual. Sem essa ordem, um "q" digitado
// num campo de busca fecharia o programa.
func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.scene == sceneHelp {
		switch msg.String() {
		case "esc", "q", "?", "enter", "ctrl+c":
			m.scene = m.prevScene
			return m, nil
		}
		var cmd tea.Cmd
		m.helpViewport, cmd = m.helpViewport.Update(msg)
		return m, cmd
	}
	if m.scene == sceneConfirm {
		return m.handleConfirmKey(msg)
	}
	if m.isTypingFilter() {
		return m.handleFilterKey(msg)
	}
	if m.isTypingDocsQuery() {
		return m.handleDocsInputKey(msg)
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		return m, tea.Quit
	case "?":
		m.prevScene = m.scene
		m.scene = sceneHelp
		m.resizeViewports()
		m.helpViewport.SetContent(helpText())
		m.helpViewport.GotoTop()
		return m, nil
	case "r":
		m.notice = ""
		m.loading = true
		return m, m.refresh()
	case "w":
		m.watch = !m.watch
		return m, nil
	case "esc":
		return m.goBack()
	case "1":
		return m.goTo(sceneDashboard), nil
	case "2":
		return m.goTo(sceneNodes), nil
	case "3":
		return m.goTo(sceneStorage), nil
	case "4":
		m = m.goTo(sceneAppList)
		m.layer = ""
		m.apps = m.filteredApps()
		m.cursor = 0
		return m, nil
	}

	return m.handleSceneKey(msg)
}

// goTo troca de scene e zera a rolagem: manter o offset de outra scene deixaria
// a nova aberta no meio, parecendo que o topo não existe.
func (m model) goTo(target scene) model {
	m.scene = target
	m.scroll = 0
	return m
}

// scrollKeys trata a rolagem do corpo. Só vale para scenes sem cursor próprio;
// onde há lista, j/k pertencem à lista.
func (m model) scrollKeys(msg tea.KeyMsg) (model, bool) {
	switch msg.String() {
	case "j", "down":
		return m.scrollBy(1), true
	case "k", "up":
		return m.scrollBy(-1), true
	case "pgdown", "ctrl+d", " ":
		return m.scrollBy(m.bodyHeight() / 2), true
	case "pgup", "ctrl+u":
		return m.scrollBy(-m.bodyHeight() / 2), true
	case "home":
		return m.scrollBy(-m.maxScroll()), true
	case "end":
		return m.scrollBy(m.maxScroll()), true
	}
	return m, false
}

func (m model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		action := m.confirmAction
		m.confirmAction = nil
		m.scene = m.prevScene
		m.loading = true
		return m, action
	default:
		m.confirmAction = nil
		m.scene = m.prevScene
		return m, nil
	}
}

func (m model) isTypingFilter() bool {
	return m.scene == sceneAppList && m.filterInput.Focused()
}

func (m model) isTypingDocsQuery() bool {
	return m.scene == sceneDocs && m.docsInput.Focused()
}

func (m model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filterInput.Blur()
		m.filterInput.SetValue("")
		m.filter = ""
		m.apps = m.filteredApps()
		return m, nil
	case "enter":
		m.filterInput.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.filter = m.filterInput.Value()
	m.apps = m.filteredApps()
	m.cursor = 0
	return m, cmd
}

func (m model) handleDocsInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.docsInput.Blur()
		return m, nil
	case "enter":
		query := m.docsInput.Value()
		m.docsInput.Blur()
		if query == "" {
			return m, nil
		}
		m.loading = true
		return m, fetchDocs(m.env, query, "")
	}
	var cmd tea.Cmd
	m.docsInput, cmd = m.docsInput.Update(msg)
	return m, cmd
}

func (m model) goBack() (tea.Model, tea.Cmd) {
	switch m.scene {
	case sceneAppDetail:
		m.scene = sceneAppList
	case sceneLogs, sceneDeploy:
		m.scene = sceneAppDetail
	case sceneDocs:
		if m.docsContent != "" {
			m.docsContent = ""
			return m, nil
		}
		m.scene = sceneDashboard
	default:
		m.scene = sceneDashboard
	}
	return m, nil
}

func (m model) handleSceneKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.scene {
	case sceneOffline:
		if msg.String() == "enter" {
			return m, checkTailnet(m.env)
		}
	case sceneDashboard:
		switch msg.String() {
		case "d":
			return m.goTo(sceneDocs), nil
		case "g":
			return m.jumpToCwdApp()
		}
		if next, handled := m.scrollKeys(msg); handled {
			return next, nil
		}
	case sceneNodes:
		// j/k rolam o corpo, então trocar de node fica em "n" — num cluster de
		// node único a rolagem é o que se usa toda hora.
		if msg.String() == "n" {
			m.nodeCursor = (m.nodeCursor + 1) % len(m.metrics.Nodes)
			m.scroll = 0
			return m, nil
		}
		if next, handled := m.scrollKeys(msg); handled {
			return next, nil
		}
	case sceneStorage, sceneDeploy:
		if next, handled := m.scrollKeys(msg); handled {
			return next, nil
		}
	case sceneAppList:
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(m.apps)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "/":
			m.filterInput.Focus()
			return m, nil
		case "p":
			m.layer = layerCycle(m.layer)
			m.apps = m.filteredApps()
			m.cursor = 0
		case "enter":
			if len(m.apps) == 0 {
				return m, nil
			}
			m.selected = m.apps[m.cursor]
			m = m.goTo(sceneAppDetail)
			m.loading = true
			return m, fetchApp(m.env, m.selected)
		}
	case sceneAppDetail:
		if next, handled := m.scrollKeys(msg); handled {
			return next, nil
		}
		switch msg.String() {
		case "l":
			m = m.goTo(sceneLogs)
			m.loading = true
			return m, fetchLogs(m.env, m.selectedNamespace(), "", m.logPrevious, m.logTail)
		case "d":
			m = m.goTo(sceneDeploy)
			m.loading = true
			return m, fetchDeploy(m.env, m.selected.Name)
		case "a":
			m.prevScene = m.scene
			m.scene = sceneConfirm
			m.confirmMsg = "Refresh hard no Application " + m.selected.Name + "?"
			m.confirmAction = refreshArgoCD(m.env, m.selected.Name)
			return m, nil
		}
	case sceneLogs:
		switch msg.String() {
		case "p":
			m.logPrevious = !m.logPrevious
			m.loading = true
			return m, fetchLogs(m.env, m.selectedNamespace(), "", m.logPrevious, m.logTail)
		case "t":
			m.logTail = nextTailSize(m.logTail)
			m.loading = true
			return m, fetchLogs(m.env, m.selectedNamespace(), "", m.logPrevious, m.logTail)
		}
	case sceneDocs:
		switch msg.String() {
		case "/":
			m.docsInput.Focus()
			return m, nil
		case "j", "down":
			if m.docsContent == "" && m.docsCursor < len(m.docsResults)-1 {
				m.docsCursor++
			}
		case "k", "up":
			if m.docsContent == "" && m.docsCursor > 0 {
				m.docsCursor--
			}
		case "enter":
			if m.docsContent == "" && m.docsCursor < len(m.docsResults) {
				m.loading = true
				return m, fetchDocs(m.env, "", m.docsResults[m.docsCursor])
			}
		}
	}
	return m.forwardToViewport(msg)
}

func (m model) jumpToCwdApp() (tea.Model, tea.Cmd) {
	if m.cwdApp == "" {
		m.notice = "nenhum app resolvido no diretório atual"
		return m, nil
	}
	for _, app := range m.cluster.Applications {
		if app.Name == m.cwdApp {
			m.selected = app
			m.scene = sceneAppDetail
			m.loading = true
			return m, fetchApp(m.env, app)
		}
	}
	m.notice = "app " + m.cwdApp + " não encontrado no ArgoCD"
	return m, nil
}

func nextTailSize(current int) int {
	switch current {
	case 50:
		return 100
	case 100:
		return 500
	default:
		return 50
	}
}
