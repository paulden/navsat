package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	content := lipgloss.JoinVertical(lipgloss.Left,
		m.renderHeader(),
		m.renderBody(),
		m.renderLogStrip(),
		m.renderFooter(),
	)

	return styleBorder.
		Width(m.contentWidth()).
		Render(content)
}

func (m Model) renderHeader() string {
	title := styleTitle.Render("navsat")
	pill := statusPill(m.state)
	gap := m.contentWidth() - lipgloss.Width(title) - lipgloss.Width(pill) - 2
	if gap < 1 {
		gap = 1
	}
	return title + strings.Repeat(" ", gap) + pill
}

func (m Model) renderBody() string {
	switch m.state {
	case stateIdle:
		return m.viewIdle()
	case stateConfig:
		return m.viewConfig()
	case stateLaunching:
		return m.viewProgress("Launching")
	case stateConnected:
		return m.viewConnected()
	case stateStopping:
		return m.viewProgress("Stopping")
	case stateError:
		return m.viewError()
	}
	return ""
}

func (m Model) viewIdle() string {
	lines := []string{
		"",
		row("Credentials", credentialSource()),
		row("Region", m.cfg.Region),
		row("Instance", m.cfg.InstanceType),
		row("SOCKS5", fmt.Sprintf("localhost:%d", m.cfg.SOCKSPort)),
		"",
	}
	return strings.Join(lines, "\n")
}

func (m Model) viewConfig() string {
	var b strings.Builder
	labels := []string{"Region", "Instance type", "SOCKS5 port"}
	b.WriteString("\n")
	for i, input := range m.inputs {
		label := styleKey.Render(fmt.Sprintf("  %-16s", labels[i]))
		b.WriteString(label + input.View() + "\n")
	}
	b.WriteString("\n")
	b.WriteString(styleMuted.Render("  tab/↑↓ navigate  enter save  esc cancel"))
	return b.String()
}

func (m Model) viewProgress(title string) string {
	var b strings.Builder
	b.WriteString("\n")
	fmt.Fprintf(&b, "  %s %s\n\n", m.spinner.View(), styleWarn.Render(title))
	for i, step := range m.steps {
		if i < len(m.steps)-1 {
			fmt.Fprintf(&b, "  %s %s\n", styleSuccess.Render("✓"), step)
		} else {
			fmt.Fprintf(&b, "  %s %s\n", m.spinner.View(), step)
		}
	}
	b.WriteString("\n")
	return b.String()
}

func (m Model) viewConnected() string {
	inst := m.instance
	lines := []string{
		"",
		row("Instance", inst.ID),
		row("Public IP", inst.PublicIP),
		row("SOCKS5", fmt.Sprintf("socks5://localhost:%d", m.cfg.SOCKSPort)),
		row("Region", m.cfg.Region),
		row("Uptime", m.uptime.String()),
		"",
		styleMuted.Render(fmt.Sprintf("  export https_proxy=socks5h://localhost:%d", m.cfg.SOCKSPort)),
		"",
	}
	return strings.Join(lines, "\n")
}

func (m Model) viewError() string {
	return fmt.Sprintf("\n  %s\n\n  %s\n\n",
		styleError.Render("Error:"),
		m.lastErr.Error(),
	)
}

func (m Model) renderFooter() string {
	var keys string
	switch m.state {
	case stateIdle:
		keys = key("s", "start") + "  " + key("c", "config") + "  " + key("q", "quit")
	case stateConnected:
		keys = key("s", "stop") + "  " + key("q", "quit")
	case stateError:
		keys = key("r", "retry") + "  " + key("q", "quit")
	case stateLaunching, stateStopping:
		keys = styleMuted.Render("please wait…")
	}
	return styleFooter.Width(m.contentWidth()).Render(keys)
}

func (m Model) renderLogStrip() string {
	divider := styleMuted.Render(strings.Repeat("─", m.contentWidth()-2))

	var lines []string
	if len(m.logs) == 0 {
		lines = []string{styleMuted.Render("  — no activity —")}
	} else {
		tail := m.logs
		if len(tail) > 6 {
			tail = tail[len(tail)-6:]
		}
		for _, l := range tail {
			lines = append(lines, styleMuted.Render("  "+l))
		}
	}

	return "\n" + divider + "\n" + strings.Join(lines, "\n") + "\n"
}

func row(label, value string) string {
	return fmt.Sprintf("  %s  %s",
		styleKey.Render(fmt.Sprintf("%-12s", label)),
		styleValue.Render(value),
	)
}

func key(k, label string) string {
	return styleKey.Render("["+k+"]") + " " + styleMuted.Render(label)
}

func (m Model) contentWidth() int {
	if m.width > 10 {
		w := m.width - 4
		if w > 72 {
			return 72
		}
		return w
	}
	return 68
}
