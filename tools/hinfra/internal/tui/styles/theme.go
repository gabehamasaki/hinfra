package styles

import "github.com/charmbracelet/lipgloss"

// Nenhuma cor deve ser escrita fora deste arquivo: as scenes consomem tokens
// semânticos, então trocar a paleta não exige caçar hex codes pelo código.
var (
	StatusOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	StatusWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	StatusErr   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	StatusMuted = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	Accent      = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	Reserved    = lipgloss.NewStyle().Foreground(lipgloss.Color("61"))
	Label       = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	Header      = lipgloss.NewStyle().Bold(true)
	PanelTitle  = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	Border      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
	Selected    = lipgloss.NewStyle().Background(lipgloss.Color("236")).Bold(true)
	StatusBar   = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252"))
)

// Limiares de saturação: acima de 90% é problema, acima de 75% merece atenção.
const (
	ThresholdWarn = 75.0
	ThresholdCrit = 90.0
)

// ThresholdStyle escolhe a cor pelo percentual de ocupação. Centralizar isso
// garante que gauge, sparkline e área usem o mesmo critério.
func ThresholdStyle(percent float64) lipgloss.Style {
	switch {
	case percent >= ThresholdCrit:
		return StatusErr
	case percent >= ThresholdWarn:
		return StatusWarn
	default:
		return StatusOK
	}
}

func SyncStyle(sync string) lipgloss.Style {
	switch sync {
	case "Synced":
		return StatusOK
	case "OutOfSync":
		return StatusErr
	default:
		return StatusWarn
	}
}

func HealthStyle(health string) lipgloss.Style {
	switch health {
	case "Healthy":
		return StatusOK
	case "Degraded", "Progressing":
		return StatusWarn
	case "Missing", "Unhealthy":
		return StatusErr
	default:
		return StatusMuted
	}
}
