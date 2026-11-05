// Package state stores account credentials and local device policy.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

type Auth struct {
	UserAgent    string `json:"user_agent"`
	DeviceID     string `json:"device_id"`
	PassO        string `json:"pass_o"`
	UserID       string `json:"user_id"`
	CUserID      string `json:"c_user_id"`
	PassToken    string `json:"pass_token"`
	ServiceToken string `json:"service_token"`
	Security     string `json:"security"`
}

func (a Auth) Valid() bool {
	return a.UserID != "" && a.ServiceToken != "" && a.Security != "" && a.PassToken != ""
}

type Policy struct {
	ReadOnly bool     `json:"read_only"`
	Devices  []string `json:"allowed_devices"`
	Scenes   []string `json:"allowed_scenes"`
}

func (p Policy) Check(kind, id string) error {
	if p.ReadOnly {
		return errors.New("local policy is read-only")
	}
	items := p.Devices
	if kind == "scene" {
		items = p.Scenes
	}
	for _, v := range items {
		if v == id {
			return nil
		}
	}
	return fmt.Errorf("%s %q is not allowed by local policy; ask the user to authorize it", kind, id)
}

type Store struct{ Dir string }

func DefaultDir() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "mijia-go"), nil
}

func safePath(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for p := path; ; p = filepath.Dir(p) {
		st, e := os.Lstat(p)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if e == nil && st.Mode()&os.ModeSymlink != 0 {
			return errors.New("state paths must not contain symlinks")
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return nil
}

func (s Store) read(name string, dst any) error {
	p := filepath.Join(s.Dir, name)
	if err := safePath(p); err != nil {
		return err
	}
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return errors.New("state must be a regular file")
	}
	if runtime.GOOS != "windows" && st.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("state file permissions are too broad; set %s to 0600", name)
	}
	return json.NewDecoder(io.LimitReader(f, 1<<20)).Decode(dst)
}

func (s Store) write(name string, v any) error {
	p := filepath.Join(s.Dir, name)
	if err := safePath(p); err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return err
	}
	if err := os.Chmod(s.Dir, 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.Dir, ".state-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(append(b, '\n'))
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp, p)
}

func (s Store) LoadAuth() (Auth, error) { var a Auth; err := s.read("auth.json", &a); return a, err }
func (s Store) SaveAuth(a Auth) error   { return s.write("auth.json", a) }
func (s Store) LoadPolicy() (Policy, error) {
	p := Policy{Devices: []string{}, Scenes: []string{}}
	err := s.read("policy.json", &p)
	if os.IsNotExist(err) {
		err = nil
	}
	return p, err
}
func (s Store) SavePolicy(p Policy) error { return s.write("policy.json", p) }
func (s Store) Logout() error {
	p := filepath.Join(s.Dir, "auth.json")
	if err := safePath(p); err != nil {
		return err
	}
	err := os.Remove(p)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
