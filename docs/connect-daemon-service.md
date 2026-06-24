# Running the connector as a 7x24 service

`dws devapp robot connect` keeps a DingTalk robot wired to a local agent over a
Stream long-connection. By default it runs in the foreground and dies when the
terminal closes. For an unattended "digital employee" you have two options.

## Option A: built-in daemon (recommended for a quick start)

```bash
# Detach into a background supervisor that restarts the connector if it crashes.
dws devapp robot connect --daemon \
  --channel claudecode \
  --robot-client-id <clientId> --robot-client-secret <clientSecret>

# Inspect / stop it.
dws devapp robot connect status --robot-client-id <clientId>
dws devapp robot connect stop   --robot-client-id <clientId>
```

- The parent prints the daemon pid and the log path, then exits.
- A supervisor process (POSIX `setsid`, detached from the terminal) keeps a
  worker connector alive, restarting it with exponential backoff (1s..60s, up to
  10 consecutive fast failures) when it exits abnormally.
- The single-instance lock (one connector per robot per machine) is reused, so a
  duplicate daemon refuses to start.
- Logs go to `~/.dws/connect/<clientId>/daemon.log` with size-based rotation
  (5 MB x 2 backups), and the pid file lives at
  `~/.dws/connect/<clientId>/daemon.pid`.
- The daemon does NOT survive a reboot. For that, use Option B.

> Windows: `--daemon` is not supported (no `setsid` / POSIX signal stop). Use a
> Windows service wrapper around the foreground command instead.

## Option B: OS service manager (survives reboot)

Use the foreground command (NOT `--daemon`) and let the OS supervise and
restart it. This is the most robust way to get boot-time auto-start.

### macOS — launchd

Save as `~/Library/LaunchAgents/com.dingtalk.dws.connect.plist`, edit the paths
and credentials, then `launchctl load -w <path>`.

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.dingtalk.dws.connect</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/dws</string>
    <string>devapp</string>
    <string>robot</string>
    <string>connect</string>
    <string>--channel</string>
    <string>claudecode</string>
    <string>--robot-client-id</string>
    <string>REPLACE_CLIENT_ID</string>
    <string>--robot-client-secret</string>
    <string>REPLACE_CLIENT_SECRET</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ThrottleInterval</key>
  <integer>10</integer>
  <key>StandardOutPath</key>
  <string>/tmp/dws-connect.out.log</string>
  <key>StandardErrorPath</key>
  <string>/tmp/dws-connect.err.log</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>/usr/local/bin:/usr/bin:/bin</string>
  </dict>
</dict>
</plist>
```

`KeepAlive=true` makes launchd restart the connector if it exits; the connector
itself relies on the single-instance lock to avoid duplicates.

### Linux — systemd (user service)

Save as `~/.config/systemd/user/dws-connect.service`, edit paths/credentials,
then:

```bash
systemctl --user daemon-reload
systemctl --user enable --now dws-connect.service
# allow it to keep running after logout:
loginctl enable-linger "$USER"
```

```ini
[Unit]
Description=DWS DingTalk robot connector
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/dws devapp robot connect \
  --channel claudecode \
  --robot-client-id REPLACE_CLIENT_ID \
  --robot-client-secret REPLACE_CLIENT_SECRET
Restart=always
RestartSec=5
# Optional hardening:
# NoNewPrivileges=true
# PrivateTmp=true

[Install]
WantedBy=default.target
```

`Restart=always` + `RestartSec` gives crash recovery; systemd captures stdout/
stderr into the journal (`journalctl --user -u dws-connect -f`).

## Which to choose

- Just need it to outlive the terminal and self-heal on crash → `--daemon`.
- Need it to come back after a reboot, with the OS owning the lifecycle → use
  launchd / systemd with the foreground command.
