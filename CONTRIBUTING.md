# Contributing

Thanks for helping make Mi Home easier to inspect and automate.

## Start small

The most useful early contributions are a redacted compatibility report, a synthetic protocol regression test, an accessibility improvement to terminal sign-in, or a small fix with a clear before/after example. For new authentication methods, regions, or a new runtime dependency, open an issue describing the design first.

## Develop

Use Go 1.26 or newer. Clone the repository and run:

```sh
go test -race ./...
go vet ./...
go mod verify
go build -trimpath -o bin/mijia ./cmd/mijia
./bin/mijia schema
```

Ordinary tests are offline. Do not add tests that automatically sign in or operate physical devices. Fixtures must be synthetic and contain no credentials, account/device IDs, addresses, or production captures.

Read [AGENTS.md](AGENTS.md) and [the architecture](docs/architecture.md) before changing the wire protocol. Keep JSON/exit-code compatibility, validate parameters against device specs, enforce policy before writes, and never replay writes automatically. Keep English/Chinese quick starts consistent. Run `gofmt` on Go files.

## Pull requests

Explain the concrete problem, resulting behavior, and validation. Link a compatibility report when relevant. Do not claim hardware support based solely on a successful build or a mocked test. Keep unrelated refactoring separate so the change is easy to audit.

Contributions are accepted under GPL-3.0-or-later. Preserve third-party notices. No contributor license agreement is required.

Report vulnerabilities privately using [SECURITY.md](SECURITY.md).
