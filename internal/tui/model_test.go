package tui

import (
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	awsutil "github.com/pauldn/navsat/internal/aws"
	"github.com/pauldn/navsat/internal/tunnel"
)

func newTestModel() Model {
	ctx, cancel := context.WithCancel(context.Background())
	m := New(ctx, cancel)
	return m
}

func update(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestInitialState(t *testing.T) {
	m := newTestModel()
	if m.state != stateIdle {
		t.Errorf("initial state = %d, want stateIdle", m.state)
	}
}

func TestStepMsgAppended(t *testing.T) {
	m := newTestModel()
	m = update(m, stepMsg{text: "Creating security group", ch: nil})
	m = update(m, stepMsg{text: "Launching instance", ch: nil})
	if len(m.steps) != 2 {
		t.Errorf("steps len = %d, want 2", len(m.steps))
	}
	if m.steps[0] != "Creating security group" {
		t.Errorf("steps[0] = %q", m.steps[0])
	}
	if len(m.logs) != 2 {
		t.Errorf("logs len = %d, want 2", len(m.logs))
	}
}

func TestLaunchDoneTransition(t *testing.T) {
	m := newTestModel()
	m.state = stateLaunching
	m.steps = []string{"step one"}

	inst := &awsutil.Instance{ID: "i-abc", PublicIP: "1.2.3.4", SGId: "sg-xyz"}
	tun := &tunnel.Tunnel{Port: 9000}
	m = update(m, launchDoneMsg{inst: inst, tun: tun})

	if m.state != stateConnected {
		t.Errorf("state = %d, want stateConnected", m.state)
	}
	if m.instance != inst {
		t.Error("instance not set")
	}
	if m.tun != tun {
		t.Error("tunnel not set")
	}
	if len(m.steps) != 0 {
		t.Errorf("steps should be cleared, got %d", len(m.steps))
	}
	if len(m.logs) == 0 {
		t.Error("logs should contain connected entry")
	}
}

func TestLogsPersistedAcrossTransitions(t *testing.T) {
	m := newTestModel()
	m = update(m, stepMsg{text: "Creating security group", ch: nil})
	m = update(m, stepMsg{text: "Launching instance", ch: nil})

	inst := &awsutil.Instance{ID: "i-abc", PublicIP: "1.2.3.4", SGId: "sg-xyz"}
	m = update(m, launchDoneMsg{inst: inst, tun: &tunnel.Tunnel{Port: 9000}})

	// steps cleared but logs must still have all entries
	if len(m.steps) != 0 {
		t.Errorf("steps should be cleared, got %d", len(m.steps))
	}
	if len(m.logs) < 3 { // 2 steps + 1 connected entry
		t.Errorf("logs len = %d, want at least 3", len(m.logs))
	}
}

func TestErrMsgLogged(t *testing.T) {
	m := newTestModel()
	m = update(m, errMsg{err: errors.New("launch failed")})

	if len(m.logs) == 0 {
		t.Error("error should be appended to logs")
	}
}

func TestStopDoneTransition(t *testing.T) {
	m := newTestModel()
	m.state = stateStopping
	m.instance = &awsutil.Instance{ID: "i-abc"}
	m.tun = &tunnel.Tunnel{Port: 9000}

	m = update(m, stopDoneMsg{})

	if m.state != stateIdle {
		t.Errorf("state = %d, want stateIdle", m.state)
	}
	if m.instance != nil {
		t.Error("instance should be nil after stop")
	}
	if m.tun != nil {
		t.Error("tunnel should be nil after stop")
	}
}

func TestErrMsgTransition(t *testing.T) {
	m := newTestModel()
	m.state = stateLaunching
	sentinel := errors.New("launch failed")

	m = update(m, errMsg{err: sentinel})

	if m.state != stateError {
		t.Errorf("state = %d, want stateError", m.state)
	}
	if m.lastErr != sentinel {
		t.Errorf("lastErr = %v, want %v", m.lastErr, sentinel)
	}
}

func TestKeyQuitFromIdle(t *testing.T) {
	m := newTestModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected a cmd, got nil")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestKeyErrorReturnToIdle(t *testing.T) {
	m := newTestModel()
	m.state = stateError
	m.lastErr = errors.New("some error")

	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})

	if m.state != stateIdle {
		t.Errorf("state = %d, want stateIdle", m.state)
	}
	if m.lastErr != nil {
		t.Errorf("lastErr should be cleared, got %v", m.lastErr)
	}
}

func TestTickUpdatesUptime(t *testing.T) {
	m := newTestModel()
	m.state = stateConnected

	m = update(m, tickMsg{})

	if m.uptime == 0 {
		t.Error("uptime should be non-zero after tick")
	}
}
