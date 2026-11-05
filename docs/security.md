# Security and trust boundaries

This project was started to make the code between an AI assistant and a Mi Home account small enough to inspect. It reduces several avoidable risks; it does not eliminate trust in Xiaomi, your AI client, your operating system, or dependencies.

## Network and data flow

| Destination | Purpose | Sensitive data |
|---|---|---|
| `account.xiaomi.com` | QR challenge, polling, token refresh | Sign-in tickets and account refresh credentials |
| `api.mijia.tech` | Mi Home app API | Service token, protocol security material, requested device data |
| `miot-spec.org` | Public capability specifications | Device specification URN; no account token |
| `sts.api.mijia.tech`, `api.io.mi.com`, `sts.api.io.mi.com` | Explicitly allowed sign-in callback destinations | Callback ticket and issued cookies |

Hosts are fixed in `internal/cloud/client.go`. There is no arbitrary `--endpoint` flag and no developer-controlled collection endpoint. HTTPS is mandatory, URLs with user information or explicit ports are rejected, and redirects cannot change the origin. POST redirects are refused. Responses, including decompressed responses, have a size bound.

`api.mijia.tech` comes from the upstream Mi Home app implementation. This project's initial review checked its TLS certificate but did **not** independently establish the domain's legal ownership; a domain-validated certificate does not prove Xiaomi ownership. Do not interpret the hardcoded allowlist as an independent vendor attestation. Verify the destination against trusted Xiaomi material or your own app's traffic before giving it real credentials if this is outside your trust boundary.

The QR is generated locally; no third-party QR image service receives it. QR output is sensitive and belongs in the user's terminal, not issue reports. Network errors and API errors omit raw URLs and server bodies to avoid leaking tickets or tokens.

## Credential storage

Authentication secrets are saved in an explicit private state directory as plaintext JSON. POSIX mode protection is 0700/0600; symlink components are refused, and writes use a private temporary file plus rename. Broad existing file permissions are rejected. This protects against casual access by other OS users; it is **not encryption at rest** and does not protect against malware running as the same user or a compromised AI agent with filesystem access.

Windows uses the user's configuration directory but does not programmatically restrict Windows ACLs. Review that directory's permissions. macOS Keychain/Windows Credential Manager integration is not yet implemented. Avoid synced folders, shared machines, and concurrent state writers.

Credentials are never returned by CLI commands, logged, or intentionally exposed to the Skill. There is no credential-export or DEBUG-auth option. `auth logout` deletes local credentials only. Deleting the file does not revoke the remote account session.

## What an AI sees and can do

Device discovery returns selected identifiers, names, models, specification types, online status, and home IDs, without printing raw cloud responses or street addresses. Queries still expose device metadata/states to the assistant you chose. Device-controlled descriptions may contain hostile text; the Skill treats them as untrusted data.

The underlying account credential may have broad powers. The CLI adds a local device/scene write allowlist, previews, input validation and read-only mode. This policy is not a sandbox: a process with credentials or write access to the policy file can bypass it. Scene authorization is separate because a scene may affect any of its configured devices. Do not allowlist locks, alarms, heaters, or other consequential devices casually.

## Update and build trust

There is no runtime updater or package download. `go install ...@latest` is a convenience install command; for reviewed deployments, use an explicit tag/commit or a fixed source checkout. The repo pins its QR module in `go.mod`/`go.sum`; Go's module checksum mechanism protects integrity against unexpected module content changes, not against malicious code already present in a version.

Release archives include checksums and license notices. Checksums detect corruption and help inspect artifacts; because they are distributed with the release, they are not an independent signature or identity proof. Build from a reviewed commit if release-account compromise is part of your threat model.

## Known gaps

- No real-account or physical-device test has been completed in the initial preview.
- Non-China cloud regions and all model-specific behaviors are unverified.
- No OS keychain, remote-session revocation, interprocess state lock, or process sandbox.
- This is not a safety monitoring system. Never rely on it for emergency response.

To disclose a vulnerability, follow [SECURITY.md](../SECURITY.md).
