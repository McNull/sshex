# SSHEX

```
┌──────┐ ┌──────┐ ┌──┐┌──┐┌────┐   ┌──┐┌──┐
│   ───┴┐│   ───┴┐│  └┘  ││  ───┐  └┐ └┘ ┌┘
├────┐  │├────┐  ││      ││  ┌──┴─┐┌──  ──┐
│   ─┘  ││   ─┘  ││  ┌┐  ││       ││  ┌┐  │
└───────┘└───────┘└──┘└──┘└───────┘└──┘└──┘
```

Convenient SSH wrapper to simplify adding tunnels and executing commands on the originating machine.

## Quick start

```sh
# on the origin
$ sshex connect bob@server

# in the remote shell
$ sshex tunnel add 8080
$ sshex exec code .
```

## Command Execution

Commands are templates stored on the origin. `sshex exec` runs them locally with the remote context.

### Command Management

```sh
$ sshex command add --alias code 'code --folder-uri "vscode-remote://ssh-remote+${REMOTE_HOST}${@:-${REMOTE_CWD}}"'
$ sshex command list
$ sshex command edit code
```

Add, edit, list, enable, disable and remove commands. An `--alias` makes the name available in the remote shell. Only the origin can edit command definitions.

### Executing Commands

```sh
$ sshex exec code /srv/app
Executing `code --folder-uri "vscode-remote://ssh-remote+server/srv/app"` ...
```

Runs on the origin, not the remote. Arguments are passed positionally, so they cannot inject shell code. Templates can use `${REMOTE_CWD}`, `${REMOTE_USER}`, `${REMOTE_HOST}`, `${REMOTE_PORT}`, `${REMOTE_SESSION}`, `${REMOTE_ORIGIN}` and `${@}`.

## Tunnel Management

```sh
$ sshex tunnel add 8080            # local 8080 > remote 8080
$ sshex tunnel add 8080:db:5432    # local 8080 > db:5432 via remote
$ sshex tunnel add -R 9000         # remote 9000 > local 9000
$ sshex tunnel list
$ sshex tunnel rm 8080
```

Tunnels belong to the session. Every shell of the session sees them, and they survive an individual shell exiting. `-L` (local forward) is the default; `-R` exposes a local service on the remote.

## Architecture

```
        origin (local machine)                         remote (ssh01)
 ┌──────────────────────────────────┐        ┌───────────────────────────┐
 │  sshex (local CLI)               │        │  sshex (remote CLI)       │
 │      │ unix socket               │        │      thin HTTP client     │
 │      v                           │        │            │              │
 │  sshexd  broker                  │ control│            │              │
 │   ├─ REST API (unix, 0600)       │<──────>│            │              │
 │   ├─ Session/Tunnel/Command svc  │ ssh -R │            │              │
 │   ├─ repositories (in-memory)    │        │            │              │
 │   └─ SSH master (system ssh)     │        └────────────┼──────────────┘
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
