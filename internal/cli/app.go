// Package cli provides a deterministic command surface for humans and agents.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"time"

	"github.com/Lewin671/mijia-go/internal/cloud"
	"github.com/Lewin671/mijia-go/internal/state"
	qrcode "github.com/skip2/go-qrcode"
)

var Version = "dev"

type fault struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *fault) Error() string    { return e.Message }
func fail(code, msg string) error { return &fault{code, msg} }

type envelope struct {
	SchemaVersion string `json:"schema_version"`
	OK            bool   `json:"ok"`
	Data          any    `json:"data,omitempty"`
	Error         *fault `json:"error,omitempty"`
}

func output(w io.Writer, human bool, v envelope) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if human {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(v)
}

func Run(args []string, out, errOut io.Writer) int {
	dir, err := state.DefaultDir()
	if err != nil {
		fmt.Fprintln(errOut, "cannot resolve configuration directory")
		return 1
	}
	fs := flag.NewFlagSet("mijia", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&dir, "config-dir", dir, "credential and policy directory")
	timeout := fs.Duration("timeout", 15*time.Second, "per-request timeout")
	human := fs.Bool("human", false, "pretty-print JSON")
	jsonOutput := fs.Bool("json", true, "JSON output (default)")
	if len(args) == 0 || slices.Contains([]string{"help", "--help", "-h"}, args[0]) {
		fmt.Fprint(out, Help)
		return 0
	}
	if err = fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(out, Help)
			return 0
		}
		_ = output(out, *human, envelope{"1", false, nil, &fault{"INVALID_ARGUMENT", "invalid global flags; run mijia help"}})
		return 2
	}
	if !*jsonOutput {
		_ = output(out, *human, envelope{"1", false, nil, &fault{"INVALID_ARGUMENT", "JSON is the output format; use --human to pretty-print"}})
		return 2
	}
	if *timeout < time.Second || *timeout > 120*time.Second {
		_ = output(out, *human, envelope{"1", false, nil, &fault{"INVALID_ARGUMENT", "timeout must be between 1s and 120s"}})
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	a := app{store: state.Store{Dir: dir}, timeout: *timeout, errOut: errOut}
	data, err := a.execute(ctx, fs.Args())
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(out, Help)
		return 0
	}
	if err != nil {
		f := &fault{"ERROR", err.Error()}
		exit := 1
		var specific *fault
		var api *cloud.APIError
		switch {
		case errors.As(err, &specific):
			f = specific
			if f.Code == "INVALID_ARGUMENT" {
				exit = 2
			} else if f.Code == "POLICY_DENIED" {
				exit = 4
			}
		case errors.Is(err, cloud.ErrAuth) || errors.Is(err, os.ErrNotExist):
			f = &fault{"AUTH_REQUIRED", "sign in interactively with mijia auth login"}
			exit = 3
		case errors.As(err, &api):
			f = &fault{"CLOUD_ERROR", err.Error()}
			exit = 5
		case errors.Is(err, context.Canceled):
			f = &fault{"CANCELED", "operation canceled"}
			exit = 130
		}
		if output(out, *human, envelope{"1", false, nil, f}) != nil {
			return 1
		}
		return exit
	}
	if output(out, *human, envelope{"1", true, data, nil}) != nil {
		return 1
	}
	return 0
}

type app struct {
	store     state.Store
	timeout   time.Duration
	errOut    io.Writer
	newClient func(state.Store, time.Duration) *cloud.Client
}

func (a app) freshClient() *cloud.Client {
	if a.newClient != nil {
		return a.newClient(a.store, a.timeout)
	}
	return cloud.New(a.store, a.timeout)
}
func (a app) client() (*cloud.Client, error) { c := a.freshClient(); return c, c.Load() }
func flags(name string, args []string, define func(*flag.FlagSet)) error {
	f := flag.NewFlagSet(name, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	define(f)
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return fail("INVALID_ARGUMENT", "invalid "+name+" flags; run mijia help")
	}
	if f.NArg() != 0 {
		return fail("INVALID_ARGUMENT", "unexpected positional argument")
	}
	return nil
}
func require(value, name string) error {
	if value == "" {
		return fail("INVALID_ARGUMENT", "--"+name+" is required")
	}
	return nil
}
func (a app) allowed(kind, id string) error {
	p, e := a.store.LoadPolicy()
	if e != nil {
		return e
	}
	if e = p.Check(kind, id); e != nil {
		return fail("POLICY_DENIED", e.Error())
	}
	return nil
}

func (a app) execute(ctx context.Context, args []string) (any, error) {
	if len(args) == 0 {
		return nil, fail("INVALID_ARGUMENT", "command required; run mijia help")
	}
	if args[0] == "version" && len(args) == 1 {
		return map[string]string{"version": Version}, nil
	}
	if args[0] == "schema" && len(args) == 1 {
		return map[string]any{"schema_version": "1", "commands": Commands, "output": "JSON envelope: schema_version, ok, data | error{code,message}", "global_flags": []string{"--config-dir", "--timeout", "--human", "--json"}, "exit_codes": map[string]int{"success": 0, "error": 1, "invalid_argument": 2, "auth_required": 3, "policy_denied": 4, "cloud_error": 5, "canceled": 130}}, nil
	}
	if args[0] == "doctor" && len(args) == 1 {
		_, authErr := a.store.LoadAuth()
		p, err := a.store.LoadPolicy()
		if err != nil {
			return nil, err
		}
		return map[string]any{"version": Version, "config_dir": a.store.Dir, "credentials_readable": authErr == nil, "allowed_devices": len(p.Devices), "read_only": p.ReadOnly, "network_checked": false, "hosts": []string{cloud.AccountHost, cloud.APIHost, cloud.SpecHost}}, nil
	}
	if len(args) < 2 {
		return nil, fail("INVALID_ARGUMENT", "subcommand required; run mijia help")
	}
	group, cmd, rest := args[0], args[1], args[2:]
	switch group {
	case "auth":
		return a.auth(ctx, cmd, rest)
	case "policy":
		return a.policy(cmd, rest)
	case "devices":
		if cmd != "list" {
			break
		}
		var home string
		if e := flags("devices list", rest, func(f *flag.FlagSet) { f.StringVar(&home, "home", "", "home ID") }); e != nil {
			return nil, e
		}
		c, e := a.client()
		if e != nil {
			return nil, e
		}
		return c.Devices(ctx, home)
	case "device":
		return a.device(ctx, cmd, rest)
	case "scenes":
		if cmd != "list" {
			break
		}
		if e := flags("scenes list", rest, func(f *flag.FlagSet) {}); e != nil {
			return nil, e
		}
		c, e := a.client()
		if e != nil {
			return nil, e
		}
		return c.Scenes(ctx)
	case "scene":
		if cmd != "run" {
			break
		}
		var id, home string
		var dry bool
		if e := flags("scene run", rest, func(f *flag.FlagSet) {
			f.StringVar(&id, "id", "", "scene ID")
			f.StringVar(&home, "home", "", "home ID")
			f.BoolVar(&dry, "dry-run", false, "preview without sending a write")
		}); e != nil {
			return nil, e
		}
		if e := require(id, "id"); e != nil {
			return nil, e
		}
		if e := require(home, "home"); e != nil {
			return nil, e
		}
		if dry {
			return map[string]any{"dry_run": true, "scene_id": id, "home_id": home, "policy_allowed": a.allowed("scene", id) == nil}, nil
		}
		if e := a.allowed("scene", id); e != nil {
			return nil, e
		}
		c, e := a.client()
		if e != nil {
			return nil, e
		}
		// Bind the requested scene to its actual home before executing.
		ss, e := c.Scenes(ctx)
		if e != nil {
			return nil, e
		}
		found := false
		for _, s := range ss {
			if s.ID == id && s.HomeID == home {
				found = true
			}
		}
		if !found {
			return nil, fail("INVALID_ARGUMENT", "scene ID and home ID do not match")
		}
		if e := a.allowed("scene", id); e != nil {
			return nil, e
		}
		result, e := c.RunScene(ctx, id, home)
		if e != nil {
			return nil, e
		}
		return map[string]any{"accepted": true, "verified": false, "result": result}, nil
	}
	return nil, fail("INVALID_ARGUMENT", "unknown command; run mijia help")
}

func (a app) auth(ctx context.Context, cmd string, args []string) (any, error) {
	switch cmd {
	case "status":
		if e := flags("auth status", args, func(f *flag.FlagSet) {}); e != nil {
			return nil, e
		}
		auth, e := a.store.LoadAuth()
		if os.IsNotExist(e) {
			return map[string]bool{"signed_in": false, "cloud_checked": false}, nil
		}
		if e != nil {
			return nil, e
		}
		return map[string]bool{"signed_in": auth.Valid(), "cloud_checked": false}, nil
	case "logout":
		if e := flags("auth logout", args, func(f *flag.FlagSet) {}); e != nil {
			return nil, e
		}
		if e := a.store.Logout(); e != nil {
			return nil, e
		}
		return map[string]any{"signed_in": false, "remote_session_revoked": false}, nil
	case "login":
		var wait time.Duration
		if e := flags("auth login", args, func(f *flag.FlagSet) {
			f.DurationVar(&wait, "wait", 180*time.Second, "maximum interactive sign-in duration")
		}); e != nil {
			return nil, e
		}
		if wait < time.Second || wait > 5*time.Minute {
			return nil, fail("INVALID_ARGUMENT", "wait must be between 1s and 5m")
		}
		c := a.freshClient()
		old, e := a.store.LoadAuth()
		if e == nil {
			c.Auth = old
		} else if !os.IsNotExist(e) {
			return nil, e
		}
		ctx, cancel := context.WithTimeout(ctx, wait)
		defer cancel()
		e = c.Login(ctx, func(raw string) error {
			qr, e := qrcode.New(raw, qrcode.Medium)
			if e != nil {
				return errors.New("cannot encode sign-in QR")
			}
			fmt.Fprintln(a.errOut, "Scan with the Mi Home app. This QR is a private sign-in challenge; do not share it.")
			fmt.Fprintln(a.errOut, qr.ToSmallString(false))
			return nil
		})
		if e != nil {
			return nil, e
		}
		return map[string]bool{"signed_in": true}, nil
	}
	return nil, fail("INVALID_ARGUMENT", "unknown auth subcommand")
}

func (a app) policy(cmd string, args []string) (any, error) {
	var id string
	var enabled bool
	if !slices.Contains([]string{"show", "allow-device", "remove-device", "allow-scene", "remove-scene", "read-only"}, cmd) {
		return nil, fail("INVALID_ARGUMENT", "unknown policy subcommand")
	}
	if e := flags("policy "+cmd, args, func(f *flag.FlagSet) {
		if strings.Contains(cmd, "device") || strings.Contains(cmd, "scene") {
			f.StringVar(&id, "id", "", "exact ID")
		}
		if cmd == "read-only" {
			f.BoolVar(&enabled, "enabled", true, "enable read-only mode")
		}
	}); e != nil {
		return nil, e
	}
	p, e := a.store.LoadPolicy()
	if e != nil {
		return nil, e
	}
	if cmd == "show" {
		return p, nil
	}
	if cmd == "read-only" {
		p.ReadOnly = enabled
	} else {
		if e = require(id, "id"); e != nil {
			return nil, e
		}
		items := &p.Devices
		if strings.Contains(cmd, "scene") {
			items = &p.Scenes
		}
		if strings.HasPrefix(cmd, "allow-") {
			if !slices.Contains(*items, id) {
				*items = append(*items, id)
			}
		} else {
			*items = slices.DeleteFunc(*items, func(s string) bool { return s == id })
		}
	}
	if e = a.store.SavePolicy(p); e != nil {
		return nil, e
	}
	return p, nil
}
