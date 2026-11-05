# Architecture

```text
User / AI assistant
       │
       │  skills/mijia/SKILL.md: intent, discovery, outcome handling
       ▼
  cmd/mijia → internal/cli ── local policy ── internal/state
                    │
                    ▼
              internal/cloud
              ├── auth.go    QR challenge, callback, refresh
              ├── crypto.go  nonce, protocol signature, RC4-drop1024
              ├── client.go  HTTPS boundary, limits, retry policy
              ├── devices.go inventory, properties, scenes, actions
              └── spec.go    discovery, types, ranges, action inputs
                    │
                    ▼
              Mi Home cloud / MIoT specifications
```

The Skill contains workflow knowledge, not credentials or a second implementation. The CLI has deterministic authorization and validation. The cloud package knows the protocol but does not decide what the user authorized. A writable cloud method must be called only through a CLI path that enforces policy.

There is no daemon, HTTP listener, MCP server, runtime Python dependency, model API, or telemetry pipeline. The one external runtime module only renders QR codes. It has no transitive Go-module dependencies.

## Protocol provenance

The Mi Home app protocol was studied from Do1e/mijia-api at commit `2cc61f2e4ae21d9e3736e450c968bbe53c87ebd1`, together with its credited micloud work. The project intentionally uses GPL-3.0-or-later and preserves attribution rather than claiming a clean-room implementation.

The wire algorithm combines a random nonce and a minute timestamp with the account's security material through SHA-256; request parameters are encrypted with RC4 after discarding 1,024 keystream bytes, then signed in the protocol's defined order. SHA-1/RC4 are compatibility requirements of this existing API, **not** recommended primitives for a new protocol. HTTPS certificate verification remains enabled.

## Validation strategy

- Crypto vector generated independently with Python's standard hash functions and a standalone RC4 implementation.
- Synthetic transports exercise sign-in, credential refresh, request encoding, response identity, redirects, and write replay prevention without contacting Xiaomi.
- Storage tests check restrictive permissions, atomic replacement, and symlink rejection.
- CLI tests check JSON, exit codes, token non-disclosure, and policy denial before authentication/network access.
- No test loads real credentials or addresses physical devices. See the [compatibility matrix](compatibility.md) for the separate live-validation status.
