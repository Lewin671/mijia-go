package cli

var Commands = []string{"auth login [--wait 180s]", "auth status", "auth logout", "devices list [--home ID]", "device inspect --id ID", "device get --id ID --property NAME|SIID.PIID", "device set --id ID --property NAME|SIID.PIID --value JSON [--dry-run] [--verify=false]", "device action --id ID --action NAME|SIID.AIID [--input JSON_ARRAY] [--dry-run]", "scenes list", "scene run --id ID --home ID [--dry-run]", "policy show", "policy allow-device --id ID", "policy remove-device --id ID", "policy allow-scene --id ID", "policy remove-scene --id ID", "policy read-only [--enabled=false]", "doctor", "schema", "version"}

const Help = `mijia — Mi Home from your terminal, ready for your AI.

Usage: mijia [--config-dir DIR] [--timeout 15s] [--human] COMMAND

  auth login [--wait 180s]              Scan a QR code with the Mi Home app
  auth status / auth logout            Inspect or remove local credentials
  devices list [--home ID]              List devices, without private addresses
  device inspect --id ID               Discover properties, enums, and actions
  device get --id ID --property NAME    Read a property (or use SIID.PIID)
  device set --id ID --property NAME --value JSON [--dry-run] [--verify=false]
  device action --id ID --action NAME [--input '[]'] [--dry-run]
  scenes list                          List existing manual scenes
  scene run --id ID --home ID [--dry-run]
  policy show                          Show the local write allowlist
  policy allow-device --id ID          Authorize writes to one device
  policy remove-device --id ID         Remove that authorization
  policy allow-scene --id ID           Authorize one manual scene
  policy remove-scene --id ID          Remove that authorization
  policy read-only [--enabled=false]   Override all write authorizations
  doctor                               Offline configuration diagnostics
  schema                               Machine-readable command catalog
  version                              Print build version

JSON is the default. --human pretty-prints the same envelope.
Global flags go BEFORE the command. --value is JSON: true, 50, or '"auto"'.
Writes are denied until explicitly allowed. Dry runs may query the cloud
but never send device writes. auth logout removes local credentials only.
`
