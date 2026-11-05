package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"slices"
	"time"
)

func (a app) device(ctx context.Context, cmd string, args []string) (any, error) {
	if !slices.Contains([]string{"inspect", "get", "set", "action"}, cmd) {
		return nil, fail("INVALID_ARGUMENT", "unknown device subcommand")
	}
	var id, prop, value, action, input string
	var dry bool
	verify := true
	if e := flags("device "+cmd, args, func(f *flag.FlagSet) {
		f.StringVar(&id, "id", "", "exact device ID")
		if cmd == "get" || cmd == "set" {
			f.StringVar(&prop, "property", "", "property name or SIID.PIID")
		}
		if cmd == "set" {
			f.StringVar(&value, "value", "", "JSON value")
			f.BoolVar(&verify, "verify", true, "read back readable properties")
		}
		if cmd == "action" {
			f.StringVar(&action, "action", "", "action name or SIID.AIID")
			f.StringVar(&input, "input", "[]", "JSON array of input values")
		}
		if cmd == "set" || cmd == "action" {
			f.BoolVar(&dry, "dry-run", false, "validate and preview without writing")
		}
	}); e != nil {
		return nil, e
	}
	if e := require(id, "id"); e != nil {
		return nil, e
	}
	if cmd == "get" || cmd == "set" {
		if e := require(prop, "property"); e != nil {
			return nil, e
		}
	}
	var v any
	var inputs []any
	if cmd == "set" {
		if e := require(value, "value"); e != nil {
			return nil, e
		}
		if json.Unmarshal([]byte(value), &v) != nil {
			return nil, fail("INVALID_ARGUMENT", "--value must be valid JSON, for example true, 50, or \"auto\"")
		}
	}
	if cmd == "action" {
		if e := require(action, "action"); e != nil {
			return nil, e
		}
		if json.Unmarshal([]byte(input), &inputs) != nil || inputs == nil {
			return nil, fail("INVALID_ARGUMENT", "--input must be a JSON array")
		}
	}
	if (cmd == "set" || cmd == "action") && !dry {
		if e := a.allowed("device", id); e != nil {
			return nil, e
		}
	}
	c, e := a.client()
	if e != nil {
		return nil, e
	}
	d, e := c.FindDevice(ctx, id)
	if e != nil {
		return nil, e
	}
	spec, e := c.Spec(ctx, d.SpecType)
	if e != nil {
		return nil, e
	}
	if cmd == "inspect" {
		return map[string]any{"device": d, "spec": spec}, nil
	}
	if cmd == "action" {
		svc, act, e := spec.Action(action)
		if e != nil {
			return nil, fail("INVALID_ARGUMENT", e.Error())
		}
		in, e := svc.Inputs(act, inputs)
		if e != nil {
			return nil, fail("INVALID_ARGUMENT", e.Error())
		}
		if dry {
			return map[string]any{"dry_run": true, "did": id, "siid": svc.IID, "aiid": act.IID, "in": in, "policy_allowed": a.allowed("device", id) == nil}, nil
		}
		if e := a.allowed("device", id); e != nil {
			return nil, e
		}
		return c.Action(ctx, id, svc.IID, act.IID, in)
	}
	siid, p, e := spec.Property(prop)
	if e != nil {
		return nil, fail("INVALID_ARGUMENT", e.Error())
	}
	if cmd == "get" {
		if !p.CanRead() {
			return nil, fail("INVALID_ARGUMENT", "property is not readable")
		}
		return c.Get(ctx, id, siid, p.IID)
	}
	if !p.CanWrite() {
		return nil, fail("INVALID_ARGUMENT", "property is not writable")
	}
	if e = p.Validate(v); e != nil {
		return nil, fail("INVALID_ARGUMENT", e.Error())
	}
	if dry {
		return map[string]any{"dry_run": true, "did": id, "siid": siid, "piid": p.IID, "value": v, "policy_allowed": a.allowed("device", id) == nil}, nil
	}
	if e := a.allowed("device", id); e != nil {
		return nil, e
	}
	r, e := c.Set(ctx, id, siid, p.IID, v)
	if e != nil {
		return nil, e
	}
	out := map[string]any{"accepted": true, "code": r.Code, "verified": false, "requested": v}
	if !verify || !p.CanRead() {
		return out, nil
	}
	// Read-back failures are reported as a write outcome, not as a reason to repeat the write.
	for attempt := 0; attempt < 3; attempt++ {
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			out["verification_error"] = "verification canceled; do not replay the write"
			return out, nil
		case <-timer.C:
		}
		actual, err := c.Get(ctx, id, siid, p.IID)
		if err != nil {
			out["verification_error"] = "read-back failed; do not replay the write"
			return out, nil
		}
		out["actual"] = actual.Value
		want, _ := json.Marshal(v)
		got, _ := json.Marshal(actual.Value)
		if string(want) == string(got) {
			out["verified"] = true
			return out, nil
		}
	}
	out["verification_error"] = fmt.Sprintf("observed state does not match requested state; inspect device %s before further action", id)
	return out, nil
}
