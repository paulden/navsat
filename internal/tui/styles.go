package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorAccent  = lipgloss.Color("#7C3AED")
	colorMuted   = lipgloss.Color("#6B7280")
	colorSuccess = lipgloss.Color("#10B981")
	colorWarn    = lipgloss.Color("#F59E0B")
	colorError   = lipgloss.Color("#EF4444")
	colorText    = lipgloss.Color("#F9FAFB")

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	styleMuted = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleSuccess = lipgloss.NewStyle().
			Foreground(colorSuccess)

	styleWarn = lipgloss.NewStyle().
			Foreground(colorWarn)

	styleError = lipgloss.NewStyle().
			Foreground(colorError)

	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(0, 1)

	styleKey = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	styleValue = lipgloss.NewStyle().
			Foreground(colorText)

	styleFooter = lipgloss.NewStyle().
			Foreground(colorMuted).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorMuted).
			PaddingTop(1).
			MarginTop(1)
)

func statusPill(s state) string {
	switch s {
	case stateIdle:
		return styleMuted.Render("● Idle")
	case stateLaunching:
		return styleWarn.Render("◌ Launching")
	case stateConnected:
		return styleSuccess.Render("● Connected")
	case stateStopping:
		return styleWarn.Render("◌ Stopping")
	case stateError:
		return styleError.Render("✗ Error")
	}
	return ""
}
