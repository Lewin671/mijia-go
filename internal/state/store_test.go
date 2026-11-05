package state

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testStore(t *testing.T) Store {
	t.Helper()
	d, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return Store{Dir: filepath.Join(d, "account")}
}

func TestCredentialRoundTripAndPermissions(t *testing.T) {
	s := testStore(t)
	a := Auth{UserID: "123", PassToken: "private", ServiceToken: "secret", Security: "key"}
	if e := s.SaveAuth(a); e != nil {
		t.Fatal(e)
	}
	got, e := s.LoadAuth()
	if e != nil || got != a {
		t.Fatalf("round trip failed: %v", e)
	}
	if runtime.GOOS != "windows" {
		for p, want := range map[string]os.FileMode{s.Dir: 0700, filepath.Join(s.Dir, "auth.json"): 0600} {
			st, e := os.Stat(p)
			if e != nil || st.Mode().Perm() != want {
				t.Fatalf("bad permissions on %s", p)
			}
		}
	}
	a.ServiceToken = "rotated"
	if e := s.SaveAuth(a); e != nil {
		t.Fatal(e)
	}
	got, e = s.LoadAuth()
	if e != nil || got != a {
		t.Fatal("atomic replacement failed")
	}
	if e := s.Logout(); e != nil {
		t.Fatal(e)
	}
	if _, e := s.LoadAuth(); !os.IsNotExist(e) {
		t.Fatal("logout did not remove local credentials")
	}
}

func TestRejectSymlinkAndBroadPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission/symlink test")
	}
	s := testStore(t)
	if e := s.SaveAuth(Auth{}); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(s.Dir, "auth.json")
	if e := os.Chmod(p, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := s.LoadAuth(); e == nil {
		t.Fatal("accepted broad credential permissions")
	}
	outside := filepath.Join(filepath.Dir(s.Dir), "outside")
	if e := os.WriteFile(outside, []byte("keep"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Remove(p); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(outside, p); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveAuth(Auth{}); e == nil {
		t.Fatal("followed symlink on save")
	}
	if _, e := s.LoadAuth(); e == nil {
		t.Fatal("followed symlink on read")
	}
	if e := s.Logout(); e == nil {
		t.Fatal("accepted symlink on logout")
	}
	b, _ := os.ReadFile(outside)
	if string(b) != "keep" {
		t.Fatal("modified symlink target")
	}
}

func TestPolicyDefaultsAndReadOnly(t *testing.T) {
	s := testStore(t)
	p, e := s.LoadPolicy()
	if e != nil {
		t.Fatal(e)
	}
	if p.Check("device", "lamp") == nil {
		t.Fatal("new account permits writes")
	}
	p.Devices = []string{"lamp"}
	if p.Check("device", "lamp") != nil || p.Check("device", "other") == nil || p.Check("scene", "lamp") == nil {
		t.Fatal("policy scope incorrect")
	}
	p.ReadOnly = true
	if p.Check("device", "lamp") == nil {
		t.Fatal("read-only override ignored")
	}
	if e = s.SavePolicy(p); e != nil {
		t.Fatal(e)
	}
	p, e = s.LoadPolicy()
	if e != nil || !p.ReadOnly {
		t.Fatal("policy not persisted")
	}
}
