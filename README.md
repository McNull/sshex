# SSHEX

```
┌──────┐ ┌──────┐ ┌──┐┌──┐┌────┐   ┌──┐┌──┐
│   ───┴┐│   ───┴┐│  └┘  ││  ───┐  └┐ └┘ ┌┘
├────┐  │├────┐  ││      ││  ┌──┴─┐┌──  ──┐
│   ─┘  ││   ─┘  ││  ┌┐  ││       ││  ┌┐  │
└───────┘└───────┘└──┘└──┘└───────┘└──┘└──┘
```

Convenient SSH wrapper to simplify adding tunnels and executing commands on the originating machine.

## Install

With Go (1.27.1 or newer):

```sh
go install github.com/mcnull/sshex@latest
```

Or download a release binary:

```sh
curl -fsSL https://raw.githubusercontent.com/McNull/sshex/main/install.sh | sh
```

The installer places the binary in `~/.local/bin` and sets up shell completions
for the detected shell. It adds a single startup line that sources
`~/.local/share/sshex/sshexrc.<shell>`, which keeps the startup file clean.
Set `SSHEX_NO_COMPLETIONS=1` to skip the completions.

### Docker

A Linux image is available for `linux/amd64` and `linux/arm64`:

```sh
docker run --rm -it \
  -v "$HOME/.ssh:/home/sshex/.ssh:ro" \
  -v "$HOME/.config/sshex:/home/sshex/.config/sshex" \
  mcnull/sshex
```

The entrypoint is `sshex`, so pass commands directly.

## Quick start

```sh
# on the origin
$ sshex connect bob@server

# in the remote shell; --client targets the session's origin
$ sshex tunnel add --client 8080
$ sshex exec code .
```

## Terminal UI

Running `sshex` without arguments opens the full-screen terminal interface, as
does `sshex ui`. Outside a session, the Server menu opens Connect, Sessions,
Tunnels, Commands and Update. Inside an sshex session, the Client menu opens the
current session's Tunnels and Commands; its Server entry manages the machine
running the UI as its own origin. The default monochrome theme adapts to light
and dark terminals and respects `NO_COLOR`.

```sh
$ sshex
$ sshex ui
```

## Command Execution

Commands are templates stored on the origin. `sshex exec` runs them locally with the remote context.

### Command Management

```sh
$ sshex command add --alias code 'code --${REMOTE_TARGET_KIND}-uri "vscode-remote://ssh-remote+${REMOTE_HOST}${REMOTE_TARGET}"'
$ sshex command list
$ sshex command edit code
```

On first startup sshex creates the config with disabled `code` and `zed` examples. Enable one with `sshex command enable code` or `sshex command enable zed`; an existing config is never rewritten.

Add, edit, list, enable, disable and remove commands. An `--alias` makes the name available in the remote shell. Only the origin can edit command definitions. `sshex command` manages the local machine; `sshex command --client` lists the current session's origin commands read-only.

### Executing Commands

```sh
$ sshex exec code /srv/app
Executing `code --folder-uri "vscode-remote://ssh-remote+server/srv/app"` ...

$ sshex exec code /srv/app/main.go
Executing `code --file-uri "vscode-remote://ssh-remote+server/srv/app/main.go"` ...
```

Runs on the origin, not the remote. Arguments are passed positionally, so they cannot inject shell code. The first argument is the target; the remote checks whether it is a directory and selects `folder` or `file`.

Templates receive these environment variables:

| Variable | Description |
| --- | --- |
| `${REMOTE_CWD}` | Working directory where `sshex exec` was invoked. |
| `${REMOTE_USER}` | Remote user. |
| `${REMOTE_HOST}` | Remote host (the ssh alias). |
| `${REMOTE_PORT}` | Remote port. |
| `${REMOTE_SESSION}` | Session id. |
| `${REMOTE_ORIGIN}` | Origin machine. |
| `${REMOTE_TARGET}` | Resolved first argument, or the remote working directory when no argument was given. |
| `${REMOTE_TARGET_KIND}` | `folder` if the target is an existing directory, otherwise `file`. |

Positional arguments are available as `${0}` (the command name), `${1}`–`${10}`, and `${@}` (all arguments). They are passed separately and are not interpolated into the command line.

## Tunnel Management

```sh
$ sshex tunnel add 8080            # local 8080 > remote 8080
$ sshex tunnel add 8080:db:5432    # local 8080 > db:5432 via remote
$ sshex tunnel add -R 9000         # remote 9000 > local 9000
$ sshex tunnel list
$ sshex tunnel rm 8080
```

Tunnels belong to the session. Every shell of the session sees them, and they survive an individual shell exiting. `-L` (local forward) is the default; `-R` exposes a local service on the remote. Tunnel commands default to the machine running them; use `--client` inside a remote shell to manage the current session's origin.

## Architecture

```
        origin (local machine)                         remote (ssh01)
 ┌──────────────────────────────────┐        ┌───────────────────────────┐
 │  sshex (local CLI)               │        │  sshex (remote CLI)       │
 │      │ unix socket               │        │      thin HTTP client     │
 │      v                           │        │                           │
 │  sshexd  broker                  │ control│                           │
 │   ├─ REST API (unix, 0600)       │<──────>│                           │
 │   ├─ Session/Tunnel/Command svc  │ ssh -R │                           │
 │   ├─ repositories (in-memory)    │        │                           │
 │   └─ SSH master (system ssh)     │        └────────────┬──────────────┘
 └───────────────┬──────────────────┘                     │
                 │  ssh -L / -R  (user tunnels)           │
                 └────────────────────────────────────────┘
```

### Session Management

A session is the origin↔remote relationship for one `user@host:port`. It owns a controller SSH connection, the control channel, a bearer token and its tunnels. Every `sshex connect` adds one connection; the session lives until the last connection exits, then its controller and tunnels are torn down. Shells send heart beats over the control channel, so a killed client does not leak state.

### Security

- REST API binds a unix socket/loopback only, never a public interface.
- Control channel is a `0600` socket in a `0700` directory; no TCP port on the remote.
- Per-session random bearer tokens delivered in `0600` files, never on the command line.
- Capabilities (`tunnels`, `exec`, `commands`) deny by default; the remote token cannot edit commands.
- Host-key verification stays on (inherited from system `ssh`).
