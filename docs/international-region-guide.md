# International DingTalk (`.io`) Guide

This guide explains how to log in to the international DingTalk region and run DWS commands against `*.dingtalk.io` services.

## Region behavior

- `dws auth login --intl` creates or refreshes an international login using the `.io` login, OAuth, and MCP services.
- Omitting `--intl` keeps the existing domestic `.com` behavior.
- `--intl` is a login option, not a global option for business commands. After login, commands such as `contact`, `calendar`, and `doc` derive the region from the selected Token/profile.
- Each new Token records its login region. Switching profiles therefore switches the official DingTalk gateway region automatically.
- `--international` is a compatibility alias. Prefer `--intl` in new scripts.

For the complete Chinese guide, see [DWS 国际版（DingTalk `.io`）使用手册](./international-region-guide.zh-CN.md).

## Check availability

```bash
dws auth login --help
```

The help output must include `--intl` and `--international`.

When validating a source checkout, build it first and use `./dws` so an older binary on `PATH` is not invoked accidentally:

```bash
make build
./dws auth login --help
```

## Log in

Browser login:

```bash
dws auth login --intl
```

Device flow for SSH, containers, and headless environments:

```bash
dws auth login --intl --device
```

User OAuth with custom application credentials:

```bash
dws auth login --intl \
  --client-id <APP_KEY> \
  --client-secret <APP_SECRET>
```

This mode still requires the user to complete OAuth authorization in a browser; it is not a userless `client_credentials` login. The application must be configured on the international developer platform with the required callback and permissions. Never commit an AppSecret to source control or include it in logs.

## Verify the login

```bash
dws auth status --format json
dws profile list --format json
dws contact user get-self
```

The last command is a read-only smoke check. If the organization has not enabled CLI access, an organization administrator must enable it or approve the access request on the international developer platform.

## Use domestic and international profiles together

```bash
# Domestic (.com)
dws auth login

# International (.io)
dws auth login --intl

# Find the stable profile selectors
dws profile list --format json
```

Persistently switch profiles:

```bash
dws profile switch <corpId>:<userId>
```

Toggle back to the previous profile:

```bash
dws profile switch -
```

Select a profile for one command without changing the default:

```bash
dws --profile <corpId>:<userId> contact user get-self
```

Do not add `--intl` to business commands. DWS routes official endpoints from the selected profile's Token region.

## Isolated smoke testing

Use a separate configuration directory to avoid changing the normal `~/.dws` login state:

```bash
DWS_CONFIG_DIR=/tmp/dws-intl-smoke ./dws auth login --intl
DWS_CONFIG_DIR=/tmp/dws-intl-smoke ./dws auth status --format json
DWS_CONFIG_DIR=/tmp/dws-intl-smoke ./dws contact user get-self
```

Use the same `DWS_CONFIG_DIR` for every command. Use `./dws` for a source build and `dws` for an installed release.

## Pre-release overrides (maintainers only)

Normal international users need only `--intl`; they should not set `--pre-url` or `--mcp-url`.

Maintainers can test the pre-release login/MCP pair with:

```bash
dws auth login --intl --pre-url https://pre-login.dingtalk.io
```

A corresponding `pre-mcp.*` URL is also accepted, and DWS derives the paired `pre-login.*` / `pre-mcp.*` bases. `--mcp-url` explicitly overrides the MCP base URL for that login.

Pre-release services may require internal network access or allowlisted accounts. `--pre-url` is intended primarily for the MCP-managed credential flow. Do not combine it with direct custom `--client-id/--client-secret` mode unless the pre-release API contract explicitly supports that combination.

## Troubleshooting

### The browser still opens a `.com` page

1. Run `dws auth login --help` and confirm `--intl` is present.
2. For a source checkout, use `./dws` instead of an older installed binary.
3. Confirm the executed command is `dws auth login --intl`.

### A business command appears to use the wrong region

Run `dws profile list --format json`, then switch with the exact `<corpId>:<userId>` selector or use the global `--profile` option. For a legacy Token created before region metadata existed, reauthorize it with `dws auth login --intl` for an international account or `dws auth login` for a domestic account.

### Login succeeds but the command reports missing permission

This normally means the organization has not enabled CLI access or the application lacks a required permission. It does not by itself indicate a region-routing failure.

### Should I edit `~/.dws/mcp_url` manually?

No. Normal users should establish the login with `dws auth login` or `dws auth login --intl`. DWS then routes official endpoints from the selected Token/profile. Manual configuration is reserved for maintainers who explicitly control the target environment.

## Command reference

| Scenario | Command |
|---|---|
| Domestic browser login | `dws auth login` |
| International browser login | `dws auth login --intl` |
| International device login | `dws auth login --intl --device` |
| Check auth state | `dws auth status --format json` |
| List profiles | `dws profile list --format json` |
| Persistently switch profile | `dws profile switch <corpId>:<userId>` |
| Toggle to previous profile | `dws profile switch -` |
| Select a profile once | `dws --profile <corpId>:<userId> <command>` |
