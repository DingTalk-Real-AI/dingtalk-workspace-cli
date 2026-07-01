# connect supervision recipes

Keep a `dws dev connect` connector alive across crashes, logout, and reboots —
**without** marrying one process manager. Every recipe here consumes the same
health contract, `dws dev connect status --json` (see PR #543), so they restart
on a *real* health verdict instead of guessing from the process table.

Two layers, pick per host:

| Host | Recipe | Boot persistence | Extra dependency |
|------|--------|------------------|------------------|
| anywhere (default) | `dws dev connect --daemon` | no | none (built-in) |
| macOS | `launchd.dws-connect.plist` | yes | none (OS built-in) |
| Linux | `systemd.dws-connect.service` | yes | none (OS built-in) |
| Windows | NSSM (see below) | yes | NSSM |
| Node teams | `pm2.ecosystem.config.js` | yes | Node + pm2 |

All of them run `connect-watchdog.sh`, which is the "脚本" from the 0701 review:
poll `status --json`, and relaunch when `state` is `down`/`degraded`/`not_running`.

## Why a watchdog on top of a supervisor?

`--daemon`, launchd, systemd, and pm2 all restart on **process death**. The
failure mode the review flagged — a connection that is *alive but deaf* — never
kills the process, so none of them see it. The watchdog closes that by asking
dws for the health verdict, not the OS for the pid.

## The watchdog

```sh
connect-watchdog.sh --client-id <clientId> [--dry-run] -- <launch command...>
```

- `--client-id` — the robot clientId (how the connector is keyed on disk).
- everything after `--` — the command to run when unhealthy.
- reads `dws dev connect status --robot-client-id <id> --json`, relaunches only
  when needed; on `down`/`degraded` it stops the old connector first to avoid the
  single-instance lock. Idempotent, safe to run every few minutes.

## macOS (launchd)

Edit `launchd.dws-connect.plist` (clientId, channel, absolute paths), then:

```sh
cp launchd.dws-connect.plist ~/Library/LaunchAgents/com.dingtalk.dws.connect.plist
launchctl load ~/Library/LaunchAgents/com.dingtalk.dws.connect.plist
```

## Linux (systemd --user)

Edit `systemd.dws-connect.service`, then:

```sh
mkdir -p ~/.config/systemd/user
cp systemd.dws-connect.service ~/.config/systemd/user/dws-connect.service
systemctl --user daemon-reload
systemctl --user enable --now dws-connect.service
loginctl enable-linger "$USER"   # keep running after logout
```

## Windows (NSSM)

`--daemon` is not supported on Windows; run the foreground connector as a
service. Install NSSM, then:

```
nssm install dws-connect "C:\path\to\dws.exe" dev connect --robot-client-id <id> --channel opencode
nssm set dws-connect AppExit Default Restart
nssm start dws-connect
```

## Node teams (pm2)

Only if you already run pm2 — it is **not** a dependency of dws. It supervises
the foreground connector (not `--daemon`, to avoid double supervision):

```sh
pm2 start pm2.ecosystem.config.js
pm2 save && pm2 startup   # boot persistence
```
