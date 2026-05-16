package tui

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	awsutil "github.com/pauldn/navsat/internal/aws"
	"github.com/pauldn/navsat/internal/config"
	"github.com/pauldn/navsat/internal/tunnel"
)

type state int

const (
	stateIdle state = iota
	stateConfig
	stateLaunching
	stateConnected
	stateStopping
	stateError
)

// messages
type (
	// stepMsg carries one log line plus the channel to re-listen on.
	stepMsg struct {
		text string
		ch   <-chan string
	}
	// listenDoneMsg is sent when the step channel is closed.
	listenDoneMsg struct{}
	launchDoneMsg struct {
		inst *awsutil.Instance
		tun  *tunnel.Tunnel
	}
	stopDoneMsg struct{}
	tickMsg     struct{}
	errMsg      struct{ err error }
)

type Model struct {
	ctx    context.Context
	cancel context.CancelFunc

	state   state
	cfg     config.Config
	spinner spinner.Model
	inputs  []textinput.Model
	focused int

	steps    []string // progress checklist, cleared on each transition
	logs     []string // persistent log, never cleared
	instance *awsutil.Instance
	tun      *tunnel.Tunnel
	uptime   time.Duration
	startAt  time.Time
	lastErr  error

	width  int
	height int
}

func New(ctx context.Context, cancel context.CancelFunc) Model {
	cfg, _ := config.Load()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styleWarn

	return Model{
		ctx:     ctx,
		cancel:  cancel,
		state:   stateIdle,
		cfg:     cfg,
		spinner: sp,
	}
}

func (m Model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case stepMsg:
		m.steps = append(m.steps, msg.text)
		m.logs = append(m.logs, msg.text)
		return m, listenCmd(msg.ch)

	case listenDoneMsg:
		return m, nil

	case launchDoneMsg:
		m.instance = msg.inst
		m.tun = msg.tun
		m.state = stateConnected
		m.startAt = time.Now()
		m.steps = nil
		m.logs = append(m.logs, "Connected · "+msg.inst.PublicIP)
		return m, tickCmd()

	case stopDoneMsg:
		m.instance = nil
		m.tun = nil
		m.steps = nil
		m.logs = append(m.logs, "Instance terminated")
		m.state = stateIdle
		return m, nil

	case tickMsg:
		if m.state == stateConnected {
			m.uptime = time.Since(m.startAt).Round(time.Second)
			return m, tickCmd()
		}
		return m, nil

	case errMsg:
		m.lastErr = msg.err
		m.logs = append(m.logs, "Error: "+msg.err.Error())
		m.state = stateError
		return m, nil
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateIdle:
		switch msg.String() {
		case "s":
			m.state = stateLaunching
			m.steps = nil
			return m, tea.Batch(m.spinner.Tick, launchCmd(m.ctx, m.cfg))
		case "c":
			m.state = stateConfig
			m.inputs = buildInputs(m.cfg)
			m.focused = 0
			m.inputs[0].Focus()
			return m, textinput.Blink
		case "q", "ctrl+c":
			m.cancel()
			return m, tea.Quit
		}

	case stateConfig:
		switch msg.String() {
		case "enter":
			if m.focused < len(m.inputs)-1 {
				m.inputs[m.focused].Blur()
				m.focused++
				m.inputs[m.focused].Focus()
				return m, textinput.Blink
			}
			m.cfg = applyInputs(m.cfg, m.inputs)
			_ = config.Save(m.cfg)
			m.state = stateIdle
			return m, nil
		case "esc":
			m.state = stateIdle
			return m, nil
		case "shift+tab", "up":
			if m.focused > 0 {
				m.inputs[m.focused].Blur()
				m.focused--
				m.inputs[m.focused].Focus()
			}
			return m, textinput.Blink
		case "tab", "down":
			if m.focused < len(m.inputs)-1 {
				m.inputs[m.focused].Blur()
				m.focused++
				m.inputs[m.focused].Focus()
			}
			return m, textinput.Blink
		default:
			var cmd tea.Cmd
			m.inputs[m.focused], cmd = m.inputs[m.focused].Update(msg)
			return m, cmd
		}

	case stateConnected:
		switch msg.String() {
		case "s":
			m.state = stateStopping
			m.steps = nil
			if m.tun != nil {
				_ = m.tun.Close()
			}
			return m, tea.Batch(m.spinner.Tick, stopCmd(m.ctx, m.cfg, m.instance))
		case "q", "ctrl+c":
			m.state = stateStopping
			m.steps = nil
			if m.tun != nil {
				_ = m.tun.Close()
			}
			return m, tea.Batch(m.spinner.Tick, stopThenQuit(m.ctx, m.cfg, m.instance))
		}

	case stateStopping:
		// no keys during teardown

	case stateError:
		switch msg.String() {
		case "r":
			m.state = stateIdle
			m.lastErr = nil
			return m, nil
		case "q", "ctrl+c":
			m.cancel()
			return m, tea.Quit
		}

	case stateLaunching:
		if msg.String() == "ctrl+c" {
			// best-effort abort; instance may still be starting
			m.cancel()
			return m, tea.Quit
		}
	}

	return m, nil
}

// commands

// listenCmd reads one line from ch and returns it as a stepMsg.
// When ch is closed it returns listenDoneMsg, stopping the chain.
func listenCmd(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		text, ok := <-ch
		if !ok {
			return listenDoneMsg{}
		}
		return stepMsg{text: text, ch: ch}
	}
}

func launchCmd(ctx context.Context, cfg config.Config) tea.Cmd {
	ch := make(chan string, 32)
	return tea.Batch(
		listenCmd(ch),
		func() tea.Msg {
			step := func(text string) { ch <- text }

			inst, privKeyPEM, err := awsutil.Launch(ctx, cfg.Region, cfg.InstanceType, step)
			if err != nil {
				close(ch)
				return errMsg{err: err}
			}

			ch <- "Opening SSH tunnel"
			const maxRetries = 3
			var tun *tunnel.Tunnel
			var tunErr error
			for attempt := 0; attempt <= maxRetries; attempt++ {
				if attempt > 0 {
					ch <- fmt.Sprintf("SSH not ready, waiting 30s before retry (%d/%d)", attempt, maxRetries)
					select {
					case <-ctx.Done():
						ch <- "Cancelled — cleaning up"
						_ = awsutil.Terminate(context.Background(), cfg.Region, inst, func(text string) { ch <- text })
						close(ch)
						return errMsg{err: ctx.Err()}
					case <-time.After(30 * time.Second):
					}
				}
				tun, tunErr = tunnel.Open(ctx, inst.PublicIP, privKeyPEM, cfg.SOCKSPort)
				if tunErr == nil {
					break
				}
			}
			if tunErr != nil {
				ch <- "Tunnel failed — cleaning up"
				_ = awsutil.Terminate(context.Background(), cfg.Region, inst, func(text string) { ch <- text })
				close(ch)
				return errMsg{err: fmt.Errorf("tunnel: %w", tunErr)}
			}

			close(ch)
			return launchDoneMsg{inst: inst, tun: tun}
		},
	)
}

func stopCmd(ctx context.Context, cfg config.Config, inst *awsutil.Instance) tea.Cmd {
	ch := make(chan string, 16)
	return tea.Batch(
		listenCmd(ch),
		func() tea.Msg {
			step := func(text string) { ch <- text }
			_ = awsutil.Terminate(ctx, cfg.Region, inst, step)
			close(ch)
			return stopDoneMsg{}
		},
	)
}

func stopThenQuit(ctx context.Context, cfg config.Config, inst *awsutil.Instance) tea.Cmd {
	return func() tea.Msg {
		_ = awsutil.Terminate(context.Background(), cfg.Region, inst, func(string) {})
		return tea.Quit()
	}
}

// credentialSource describes which AWS credentials will be used,
// following the SDK's resolution order.
func credentialSource() string {
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" {
		return "env vars (AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY)"
	}
	if p := os.Getenv("AWS_PROFILE"); p != "" {
		return p + " (AWS_PROFILE)"
	}
	return "[default] profile"
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

// config inputs helpers

func buildInputs(cfg config.Config) []textinput.Model {
	fields := []struct {
		label, value string
	}{
		{"Region", cfg.Region},
		{"Instance type", cfg.InstanceType},
		{"SOCKS5 port", fmt.Sprintf("%d", cfg.SOCKSPort)},
	}
	inputs := make([]textinput.Model, len(fields))
	for i, f := range fields {
		t := textinput.New()
		t.Placeholder = f.label
		t.SetValue(f.value)
		inputs[i] = t
	}
	return inputs
}

func applyInputs(cfg config.Config, inputs []textinput.Model) config.Config {
	cfg.Region = inputs[0].Value()
	cfg.InstanceType = inputs[1].Value()
	port := 9000
	if _, err := fmt.Sscanf(inputs[2].Value(), "%d", &port); err != nil {
		port = 9000
	}
	cfg.SOCKSPort = port
	return cfg
}
