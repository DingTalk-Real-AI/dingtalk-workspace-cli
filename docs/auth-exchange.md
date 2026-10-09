# External authorization-code login

`dws auth exchange` logs in with a code issued by another device or service.
It does not require an existing supervisor profile and does not request a new
code. The code must belong to the selected authorization application.

```sh
dws auth exchange --code-stdin --client-id default --format json
dws auth exchange --code-stdin --client-id <dwsClientId> --format json
```

The host writes the code to stdin and closes stdin. `--code <code>` is also
supported; stdin avoids exposing the code in process arguments. The two input
options are mutually exclusive, and stdin is limited to 64 KiB.

## Application selection

| Input | Behavior |
| --- | --- |
| `--client-id default` | Discover the current service environment's official application, then exchange using server-managed credentials. |
| `--client-id <id>` | Use that application. A local secret is reused only when the configured application ID matches; otherwise use server-managed credentials. |
| Omitted | Use the configured application, or discover the official default if no application is configured. |

External exchange reads application configuration from its supplied config
directory without using the process-wide app credential cache. Malformed or
unreadable configuration fails before exchanging the code. Direct credential
provenance records the selected secret's actual source (`flag`, `app`, `env`,
or `default`), rather than marking every direct login as `flag`.

Existing direct OAuth ClientID/ClientSecret configuration remains usable. An
explicit `--client-secret` selects direct OAuth for the chosen concrete ID;
it conflicts with `--client-id default`. Application selection happens before
the code is consumed. Exchange does not retry a code against another app or
switch transports after a failure. Custom applications must be supported by
the authorization service; passing an arbitrary ID does not authorize it.

There is no `--mcp` flag or alias. Internal callers of the previous test branch
must replace it with `--client-id default`. The internal token `Source=mcp`
metadata remains supported because refresh uses it to select the endpoint.
The persisted client ID is always the actual ID, never the string `default`.

## Identity and output

Before persisting a token, exchange queries the current user's identity using
the new token. It requires exactly one complete `corpId:userId` identity in
the token's organization. It does not fill missing user IDs from an older
profile or trust a caller-provided ID as the actual identity.

`--expected-corp-id` and `--expected-user-id` are optional assertions. The
existing hidden `--uid` option also acts as an expected user ID; it no longer
overwrites the online identity. Conflicting assertions fail before exchange.

Successful JSON has `success: true` and a `data` object containing:

- `status: profile_saved`, `dwsProfile`, `corpId`, `userId`, `clientId`;
- `expiresAt`, `currentProfile`, `isCurrentProfile`, `useOnce`.

Tokens and codes are never returned. In an empty sandbox, the first profile
becomes current. Existing default profiles are preserved, including concurrent
changes observed under the persistence lock. Repeat login updates the exact
identity slot. Refresh uses the application's persisted identity metadata.

Direct-login ClientSecret writes participate in the same locked snapshot and
rollback as Token/Profile writes. An unreadable existing secret fails before
code consumption; the authoritative snapshot is read again under the write lock.
If saving fails, an existing secret is restored, or a newly created secret is
removed. Rollback errors are surfaced; this is local failure recovery, not a
rollback of the server's consumption of a one-time code. Editions with a custom
token-saving hook do not support this direct-credential transaction and reject
it before exchange; ordinary managed exchange remains available.

`--dry-run` validates arguments and describes the steps without reading stdin,
discovering applications, exchanging codes, or writing credentials.

## Three entry points

| Command | Use when |
| --- | --- |
| `auth exchange` | The host already obtained a code and supplies it to the employee's sandbox. |
| `dingtalk-tag manage login` | A logged-in supervisor uses `agentUuid` to request authorization and save the employee profile. |
| `dingtalk-tag connect` | Connect a published local employee to DSH or another supported local Agent. |

Like other authentication controls, `auth exchange` is public in CLI help but
explicitly excluded from the stable Agent Schema catalog. Its reviewed exact
exclusion is in `internal/cli/schema_command_exclusions.go`.

## Local verification

```sh
make build
python3 scripts/dev/test-auth-exchange-local.py --binary ./dws
```

The smoke test starts a localhost OAuth/MCP fixture, uses isolated encrypted
credential storage, and launches the real binary in separate processes. It
checks code exchange, request-scoped identity authentication, profile readback,
refresh, then normal `contact user get-self` execution with both the default and
explicit exact profile. It also checks dry-run, rejected flags, and incomplete identities. It does not use
production credentials or establish that a particular HSF-issued code or
custom application is authorized by the production backend.

Performance reports redact both `--code VALUE` and `--code=VALUE`. The binary
regression covers success, failure, and dry-run for both forms and stdin,
checking stdout, stderr, and performance reports with fake credentials only.
The runner invokes the binary directly and does not change security policy or
use architecture launch adapters.
