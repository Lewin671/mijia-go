# Compatibility: report what actually works

The initial preview has **no hardware-verified device models**. Offline tests validate implementation invariants, not Xiaomi account acceptance or physical outcomes.

| Environment / feature | Status |
|---|---|
| Offline crypto/sign-in/API fixtures | Automated tests |
| macOS / Linux / Windows binaries, amd64 and arm64 | Cross-build targets; CI results are visible in Actions |
| Real Mi Home account sign-in and refresh | Not yet verified |
| Mainland-China cloud devices | Implemented, awaiting live reports |
| Shared devices outside home discovery | Not guaranteed |
| Non-China regions / LAN control | Not implemented |

## Send a useful report

Use the compatibility issue template and include:

- CLI version, OS/architecture, and Mi Home account region.
- Public model identifier, for example a product's `model` string, rather than your unique `did`.
- Which operation succeeded or failed: sign-in, list, inspect, get, set/read-back, action, or scene.
- Error **code** and a manually redacted description. Do not attach raw logs.
- For control, distinguish cloud acceptance from an observed physical state.

Do not include tokens, QR codes, account IDs, device IDs, MAC/IP addresses, street addresses, serial numbers, or complete cloud responses. Synthetic fixtures are welcome; production captures are not required.

After a reproducible report, add a row with the tested version, model, operations, and the report link. Do not infer support for a whole product family from one model.
