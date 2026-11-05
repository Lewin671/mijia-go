# Working on mijia-go

This is a Go CLI and an Agent Skill for Mi Home. There is no MCP server.

- Keep runtime dependencies small and pinned. Core HTTP, crypto, storage, and CLI use the Go standard library; QR rendering is the only external module.
- Read `docs/architecture.md` and `docs/security.md` before changing authentication or the wire protocol.
- Preserve the `schema_version: "1"` JSON envelope, documented exit codes, and exact device-ID selection.
- Never log or include credentials, cookies, sign-in tickets, raw authentication responses, or private account data in errors or test fixtures. Never commit real device data.
- Keep write authorization in the CLI's policy boundary. Preview may make reads but must not make writes. Never automatically replay a write after errors or timeouts.
- Do not log into an account or control real hardware without the user's explicit instruction. Default tests must use offline fixtures and fake transports.
- Validation: `go test -race ./...`, `go vet ./...`, `go mod verify`, and the Skill validator where available. No benchmarks are needed for ordinary changes.
- Keep English and Chinese quick starts consistent. Label synthetic examples and unverified compatibility honestly.
- Preserve protocol provenance and GPL/MIT notices. Do not add telemetry, auto-update downloads, shell installers, or remote-code execution.
