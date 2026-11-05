# CLI contract

The executable is `mijia`. Global flags must precede the command:

```sh
mijia --config-dir /path/to/private/state --timeout 20s --human devices list
```

`--timeout` is a per-request timeout (1–120 seconds, default 15s), not an overall command deadline. Multi-page discovery may make several requests. Ctrl-C cancels an operation. QR polling has a separate 125-second request bound; `auth login --wait 180s` bounds the whole interactive sign-in (maximum 5 minutes).

## Output

Normal commands return exactly one JSON envelope on stdout. `--human` pretty-prints that same envelope. `--json` is an optional explicit spelling of the default. Help is human-readable text; QR prompts and the QR itself appear only on stderr.

```json
{"schema_version":"1","ok":true,"data":{"signed_in":false,"cloud_checked":false}}
```

```json
{"schema_version":"1","ok":false,"error":{"code":"POLICY_DENIED","message":"device is not allowed by local policy"}}
```

These are synthetic examples. Fields in `data` depend on the command. See [output.schema.json](output.schema.json) for the envelope schema; `mijia schema` enumerates commands offline. Clients should tolerate additional fields and inspect `error.code`, not parse prose messages.

| Exit | Meaning |
|---|---|
| 0 | Command completed; inspect the outcome fields below |
| 1 | Network, storage, response validation, or other failure |
| 2 | Invalid arguments or unsupported value |
| 3 | Missing or expired authentication |
| 4 | Local policy denied the write |
| 5 | Xiaomi API returned an error code |
| 130 | Operation canceled |

Authentication errors never trigger interactive login automatically. Read calls may refresh credentials and retry once on an expired-token response. Device writes, actions, and scenes are never automatically replayed. POST redirects are refused as well.

## Devices

```sh
mijia devices list [--home HOME_ID]
mijia device inspect --id DEVICE_ID
mijia device get --id DEVICE_ID --property NAME
mijia device set --id DEVICE_ID --property NAME --value JSON [--dry-run] [--verify=false]
mijia device action --id DEVICE_ID --action NAME [--input '[]'] [--dry-run]
```

`--id` is always an exact device ID. Names are shown for discovery, never used to select a device for a write. Property and action names come from their MIoT URNs. If a short name occurs in multiple services, use `SIID.PIID` for properties or `SIID.AIID` for actions. Do not confuse a device's user-assigned name with a property name.

`--value` must be JSON, with the right type, enum, range, and step. Examples: `true`, `50`, `'"auto"'`. A CLI parameter is not interpreted as shell code. Quote string values according to your shell. Integer values beyond JSON's exact numeric range are deliberately refused.

Action `--input` is an array of values in the order declared by the action's `in` specification. The CLI checks the referenced input properties and builds the MIoT input structure. Unknown formats are refused rather than guessed.

`--dry-run` on a device command may make discovery/specification reads, but never writes. It reports the planned identifiers, values, and current policy decision. No credentials are needed for the entirely local `scene run --dry-run` preview; actual execution validates the scene/home relationship against the cloud first.

### Accepted is not verified

- A successful property write returns `accepted: true`, its cloud `code`, and `verified: false` initially. Xiaomi code 1 is treated as accepted/pending, not observed success.
- Unless `--verify=false` is used, readable properties are checked up to three times, 500ms apart. Only a matching read-back produces `verified: true`.
- A mismatch, read error, or cancellation after the write returns `verified: false` with `verification_error`. Exit 0 means the write was accepted; it does **not** prove the requested state was reached.
- Actions and scenes cannot be verified generically. They report acceptance and never claim a physical outcome. After a transport error, the physical outcome may be unknown: read before deciding whether to try again.

## Scenes and policy

```sh
mijia scenes list
mijia scene run --id SCENE_ID --home HOME_ID [--dry-run]
mijia policy show
mijia policy allow-device --id DEVICE_ID
mijia policy remove-device --id DEVICE_ID
mijia policy allow-scene --id SCENE_ID
mijia policy remove-scene --id SCENE_ID
mijia policy read-only [--enabled=false]
```

New state directories have no write authorizations. Device and scene allowlists are independent; a scene may control devices outside the device allowlist. Read-only mode overrides both. These commands change **local** policy, not Xiaomi-side account scopes. Anyone who can change the policy file or run a different API client with the credentials can bypass it. It is an accidental-action guard, not an OS security sandbox.

## Credentials

`mijia auth login` is interactive. `auth status` only checks local completeness, without sending a request. `auth logout` only deletes the local credential file; it does not revoke remote sessions. Review sessions/authorizations through your Xiaomi account when revocation is needed.

The default state location follows Go's `os.UserConfigDir()` plus `mijia-go`: `~/Library/Application Support/mijia-go` on macOS, `$XDG_CONFIG_HOME/mijia-go` or `~/.config/mijia-go` on Linux, and `%AppData%\mijia-go` on Windows. `--config-dir` overrides it. Do not use a synced/public directory.

Files contain long-lived account secrets in plaintext. On POSIX systems directories are set to 0700, files to 0600, and broad file permissions and symlink paths are refused. Windows file ACLs are not managed by the CLI. Atomic replacement protects against partial saves, but the store is designed for one writer at a time. Avoid simultaneous login/policy edits from multiple processes.
