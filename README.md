# navsat

A terminal UI for spinning up a temporary AWS EC2 instance and routing traffic through it via a SOCKS5 proxy. The instance is terminated the moment you quit — no lingering costs.

## Features

- Launches a `t4g.nano` (or any instance type) in your chosen AWS region
- Generates a fresh ed25519 SSH key pair per session — no key management required
- Opens a SOCKS5 proxy tunnel (`ssh -D`) through the instance automatically
- Retries the SSH tunnel up to 3 times (30s apart) while the instance boots
- Terminates the instance, key pair, and security group on exit — even on `ctrl+c`
- Persistent activity log in the TUI so you can see exactly what is happening
- Single binary, no daemon

## Requirements

- Go 1.22+
- AWS credentials configured (`~/.aws/credentials`, environment variables, or SSO)
- `ssh` available in `$PATH`

## Install

```sh
git clone https://github.com/pauldn/navsat
cd navsat
go build -o navsat .
# optionally move to your PATH
mv navsat /usr/local/bin/navsat
```

## AWS credentials

navsat uses the standard AWS credential chain — no extra configuration needed. The idle screen shows which credentials will be used:

| What you see | Source |
|---|---|
| `env vars (AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY)` | Environment variables |
| `my-profile (AWS_PROFILE)` | Named profile via `AWS_PROFILE` env var |
| `[default] profile` | `[default]` in `~/.aws/credentials` or `~/.aws/config` |

For SSO profiles, run `aws sso login --profile <name>` before launching navsat, then set `AWS_PROFILE=<name>`.

## Usage

```sh
navsat
```

### Key bindings

| State      | Key      | Action                          |
|------------|----------|---------------------------------|
| Idle       | `s`      | Launch instance and open tunnel |
| Idle       | `c`      | Open config editor              |
| Idle       | `q`      | Quit                            |
| Config     | `tab/↑↓` | Navigate fields                 |
| Config     | `enter`  | Save and return                 |
| Config     | `esc`    | Cancel and return               |
| Connected  | `s`      | Stop instance (keep CLI open)   |
| Connected  | `q`      | Terminate instance and quit     |
| Error      | `r`      | Return to idle                  |
| Error      | `q`      | Quit                            |

### Using the proxy

Once connected, configure your browser or system to use:

```
SOCKS5  localhost:9000
```

Or export it for CLI tools:

```sh
export https_proxy=socks5h://localhost:9000
export http_proxy=socks5h://localhost:9000
curl https://ifconfig.me   # should show the EC2 instance's IP
```

## Configuration

Config is stored at `~/.config/navsat/config.json` and editable via the `[c]` screen in the TUI.

| Field           | Default     | Description                          |
|-----------------|-------------|--------------------------------------|
| `region`        | `eu-west-2` | AWS region to launch the instance in |
| `instance_type` | `t4g.nano`  | EC2 instance type                    |
| `socks_port`    | `9000`      | Local port for the SOCKS5 proxy      |

## Cost

A `t4g.nano` costs ~$0.0042/hour on-demand in `eu-west-2`. A typical 30-minute session costs less than $0.01. The instance is always terminated on exit.
