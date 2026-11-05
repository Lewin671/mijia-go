package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Lewin671/mijia-go/internal/state"
)

func invoke(t *testing.T, dir string, args ...string) (int, map[string]any, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(append([]string{"--config-dir", dir}, args...), &out, &errOut)
	var e map[string]any
	if err := json.Unmarshal(out.Bytes(), &e); err != nil {
		t.Fatalf("not a JSON envelope: %s", out.String())
	}
	return code, e, errOut.String()
}

func TestOfflineCommandContractAndPolicyGate(t *testing.T) {
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	dir = filepath.Join(dir, "state")
	for _, args := range [][]string{{"version"}, {"schema"}, {"doctor"}, {"auth", "status"}, {"policy", "show"}} {
		code, body, _ := invoke(t, dir, args...)
		if code != 0 || body["schema_version"] != "1" || body["ok"] != true {
			t.Fatalf("offline command failed %v", args)
		}
	}
	code, body, _ := invoke(t, dir, "device", "set", "--id", "lamp", "--property", "on", "--value", "true")
	if code != 4 || body["error"].(map[string]any)["code"] != "POLICY_DENIED" {
		t.Fatal("write did not fail closed before any authentication/network")
	}
	code, _, _ = invoke(t, dir, "policy", "allow-device", "--id", "lamp")
	if code != 0 {
		t.Fatal("cannot authorize device")
	}
	code, _, _ = invoke(t, dir, "device", "set", "--id", "lamp", "--property", "on", "--value", "true")
	if code != 3 {
		t.Fatal("missing credentials did not return AUTH_REQUIRED")
	}
	code, _, _ = invoke(t, dir, "policy", "read-only")
	if code != 0 {
		t.Fatal("cannot enable read-only")
	}
	code, _, _ = invoke(t, dir, "device", "action", "--id", "lamp", "--action", "toggle")
	if code != 4 {
		t.Fatal("action bypassed read-only override")
	}
}

func TestCredentialsNeverEnterStatusOrDiagnostics(t *testing.T) {
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	s := state.Store{Dir: filepath.Join(dir, "state")}
	if e = s.SaveAuth(state.Auth{UserID: "private-uid", Security: "private-security", PassToken: "private-pass", ServiceToken: "private-service"}); e != nil {
		t.Fatal(e)
	}
	for _, args := range [][]string{{"auth", "status"}, {"doctor"}} {
		code, body, stderr := invoke(t, s.Dir, args...)
		b, _ := json.Marshal(body)
		if code != 0 || strings.Contains(string(b)+stderr, "private-") {
			t.Fatal("credentials leaked to output")
		}
	}
}

func TestUsageErrorsAreJSONAndNonzero(t *testing.T) {
	for _, args := range [][]string{{"nonsense", "list"}, {"device", "get"}, {"device", "set", "--id", "x", "--property", "on", "--value", "not-json"}, {"auth", "status", "extra"}, {"scenes", "list", "--unknown"}} {
		code, body, _ := invoke(t, t.TempDir(), args...)
		if code != 2 || body["ok"] != false {
			t.Fatalf("bad usage accepted: %v", args)
		}
	}
}
