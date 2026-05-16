# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
go build -o navsat .          # build binary
go build ./...                # compile-check all packages
go vet ./...                  # static analysis
go test ./...                 # run all tests
golangci-lint run             # lint (run after every change before committing)
go run .                      # run without building
```

The project uses `asdf` for Go version management — `.tool-versions` pins `golang 1.26.3`.

## Architecture

navsat is a single-binary TUI that creates an ephemeral EC2 instance and routes traffic through it via SOCKS5. **Cost safety is the top priority**: the instance must be terminated whenever the CLI exits, including on signals.

### Lifecycle and shutdown contract

`main.go` owns a `context.Context` + `cancel` pair that is passed into the TUI model. Two shutdown paths exist:

1. **Clean exit** (user presses `q` from connected state): the bubbletea `stopThenQuit` command calls `awsutil.Terminate` using `context.Background()` (not the model's ctx, which may already be cancelled), then returns `tea.Quit()`.
2. **Signal/force exit** (`SIGINT`/`SIGTERM`): `main.go` catches the signal and calls `cancel()`. This propagates into any in-flight AWS waiter or tunnel open, unblocking them so the process can exit. The instance may not be cleanly terminated in this path — this is a known trade-off for `ctrl+c` during launch.

When adding new teardown logic, always use `context.Background()` for the actual AWS terminate call (not the model ctx), so cancellation doesn't abort the cleanup.

### TUI state machine (`internal/tui/model.go`)

The `Model` cycles through six states: `stateIdle → stateLaunching → stateConnected → stateStopping → stateIdle`. `stateConfig` and `stateError` are side branches from idle.

- **Messages** (`stepMsg`, `launchDoneMsg`, `stopDoneMsg`, `tickMsg`, `errMsg`) are the only way state transitions happen — no direct mutation outside `Update`.
- The `steps []string` slice accumulates the progress checklist during launching/stopping and is reset to `nil` on each transition. The `logs []string` slice is never cleared and feeds the persistent log strip at the bottom of every view.
- **Step streaming**: `launchCmd` and `stopCmd` create a buffered `chan string` and use `tea.Batch` to run the worker goroutine alongside a `listenCmd`. Each step sent to the channel is delivered to `Update` as a `stepMsg`, which fires `listenCmd` again to receive the next one. The chain ends when the channel is closed (`listenDoneMsg`).
- `launchCmd` retries `tunnel.Open` up to 3 times with 30s waits between attempts. On any failure path the channel is kept open until after `awsutil.Terminate` completes so cleanup steps appear in the log.

### AWS package (`internal/aws/ec2.go`)

`Launch` and `Terminate` are the only public functions. Each call creates its own AWS SDK client from the default config chain (env vars, `~/.aws/credentials`, instance metadata). The `StepFunc` callback is called before each blocking operation to feed progress text back to the TUI.

`Launch` returns `(*Instance, []byte, error)` — the second value is a PEM-encoded ed25519 private key to be passed directly to `tunnel.Open`. No key files are written to disk by the AWS package.

Every resource created has a matching cleanup. Cleanup functions in error paths use `context.Background()` (not the passed ctx) so a cancellation doesn't abort teardown. Resources created per session:
- Security group named `navsat-<unix-timestamp>`
- EC2 key pair named `navsat-<unix-timestamp>` (imported from a generated ed25519 key)
- Instance tagged `ManagedBy=navsat`

The AMI lookup always resolves the latest Amazon Linux 2 AMI at launch time. Architecture is detected from the instance type name: families ending in `g` (e.g. `t4g`, `m6g`) and `a1` map to `arm64`; everything else maps to `x86_64`.

### Tunnel package (`internal/tunnel/ssh.go`)

Wraps `ssh -D <port> -N` as a subprocess. `Open` accepts PEM-encoded private key bytes, writes them to a `os.CreateTemp` file with mode `0600`, and passes the path to `ssh -i`. The temp file is removed on `Close` or if `waitReady` fails.

`waitReady` polls `localhost:<port>` every 500ms for up to 30 seconds to confirm the SOCKS5 port is accepting connections. The SSH user is hardcoded to `ec2-user` (Amazon Linux 2 default).

`Close` kills the SSH process and removes the temp key file; it does not wait for graceful exit.

### Config (`internal/config/config.go`)

Loaded from `~/.config/navsat/config.json` at startup with `Default()` values as fallback. Saved immediately when the user confirms the config screen. Fields: `region`, `instance_type`, `socks_port`. No key or credential fields — AWS credentials are resolved entirely from the environment.

The idle screen displays which credential source will be used (env vars, `AWS_PROFILE`, or `[default]` profile) via `credentialSource()` in `model.go`, which checks env vars in priority order.
