package tunnel

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"
)

type Tunnel struct {
	cmd     *exec.Cmd
	Port    int
	keyFile string
}

// Open starts an SSH SOCKS5 proxy (-D) through the remote host using the
// provided private key PEM bytes. The key is written to a temp file for
// the duration of the tunnel and removed on Close.
func Open(ctx context.Context, host string, privKeyPEM []byte, port int) (*Tunnel, error) {
	keyFile, err := os.CreateTemp("", "navsat-key-*")
	if err != nil {
		return nil, fmt.Errorf("create temp key: %w", err)
	}
	keyPath := keyFile.Name()

	if err := os.Chmod(keyPath, 0600); err != nil {
		_ = os.Remove(keyPath)
		return nil, fmt.Errorf("chmod key: %w", err)
	}
	if _, err := keyFile.Write(privKeyPEM); err != nil {
		_ = os.Remove(keyPath)
		return nil, fmt.Errorf("write key: %w", err)
	}
	if err := keyFile.Close(); err != nil {
		_ = os.Remove(keyPath)
		return nil, fmt.Errorf("close key file: %w", err)
	}

	cmd := exec.CommandContext(ctx, "ssh",
		"-D", fmt.Sprintf("%d", port),
		"-N",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ServerAliveInterval=15",
		"-o", "ExitOnForwardFailure=yes",
		"-i", keyPath,
		fmt.Sprintf("ec2-user@%s", host),
	)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(keyPath)
		return nil, fmt.Errorf("start ssh: %w", err)
	}

	t := &Tunnel{cmd: cmd, Port: port, keyFile: keyPath}
	if err := t.waitReady(ctx); err != nil {
		_ = cmd.Process.Kill()
		_ = os.Remove(keyPath)
		return nil, err
	}
	return t, nil
}

// Close kills the SSH process and removes the temporary key file.
func (t *Tunnel) Close() error {
	if t.keyFile != "" {
		_ = os.Remove(t.keyFile)
		t.keyFile = ""
	}
	if t.cmd != nil && t.cmd.Process != nil {
		return t.cmd.Process.Kill()
	}
	return nil
}

func (t *Tunnel) waitReady(ctx context.Context) error {
	addr := fmt.Sprintf("127.0.0.1:%d", t.Port)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("SOCKS5 port %d did not open within 30s", t.Port)
}
