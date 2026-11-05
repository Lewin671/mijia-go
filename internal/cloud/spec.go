package cloud

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

func shortName(urn string) string {
	p := strings.Split(urn, ":")
	if len(p) > 3 {
		return p[3]
	}
	return urn
}
func has(items []string, v string) bool {
	for _, s := range items {
		if s == v {
			return true
		}
	}
	return false
}

func (s Spec) Property(selector string) (int, Property, error) {
	var siid int
	var found Property
	count := 0
	for _, svc := range s.Services {
		for _, p := range svc.Properties {
			if selector == fmt.Sprintf("%d.%d", svc.IID, p.IID) || selector == shortName(p.Type) {
				siid = svc.IID
				found = p
				count++
			}
		}
	}
	if count == 0 {
		return 0, Property{}, fmt.Errorf("property %q not found; inspect the device first", selector)
	}
	if count > 1 {
		return 0, Property{}, errors.New("property name is ambiguous; use SIID.PIID")
	}
	return siid, found, nil
}
func (s Spec) Action(selector string) (Service, Action, error) {
	var service Service
	var found Action
	count := 0
	for _, svc := range s.Services {
		for _, a := range svc.Actions {
			if selector == fmt.Sprintf("%d.%d", svc.IID, a.IID) || selector == shortName(a.Type) {
				service = svc
				found = a
				count++
			}
		}
	}
	if count == 0 {
		return Service{}, Action{}, fmt.Errorf("action %q not found; inspect the device first", selector)
	}
	if count > 1 {
		return Service{}, Action{}, errors.New("action name is ambiguous; use SIID.AIID")
	}
	return service, found, nil
}

func (p Property) CanRead() bool  { return has(p.Access, "read") }
func (p Property) CanWrite() bool { return has(p.Access, "write") }
func (p Property) Validate(v any) error {
	bad := func() error { return fmt.Errorf("value does not match property format %q", p.Format) }
	switch p.Format {
	case "bool":
		if _, ok := v.(bool); !ok {
			return bad()
		}
	case "string":
		if _, ok := v.(string); !ok {
			return bad()
		}
	case "float", "int8", "int16", "int32", "int64", "uint8", "uint16", "uint32", "uint64":
		n, ok := v.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			return bad()
		}
		if p.Format != "float" {
			if n != math.Trunc(n) || math.Abs(n) > 9007199254740991 {
				return errors.New("integer value must be exact within JSON safe integer range")
			}
			bits, _ := strconv.Atoi(strings.TrimLeft(p.Format, "uint"))
			if strings.HasPrefix(p.Format, "uint") {
				if n < 0 || n >= math.Exp2(float64(bits)) {
					return bad()
				}
			} else {
				lim := math.Exp2(float64(bits - 1))
				if n < -lim || n >= lim {
					return bad()
				}
			}
		}
		if len(p.Range) >= 2 && (n < p.Range[0] || n > p.Range[1]) {
			return errors.New("value is outside the device's declared range")
		}
		if len(p.Range) >= 3 && p.Range[2] > 0 {
			steps := (n - p.Range[0]) / p.Range[2]
			if math.Abs(steps-math.Round(steps)) > 1e-6 {
				return errors.New("value does not match the device's declared step")
			}
		}
	default:
		return errors.New("unsupported property format; refusing to guess")
	}
	if len(p.Values) > 0 {
		want, _ := json.Marshal(v)
		for _, item := range p.Values {
			got, _ := json.Marshal(item.Value)
			if string(want) == string(got) {
				return nil
			}
		}
		return errors.New("value is not in the device's declared enumeration")
	}
	return nil
}

func (s Service) Inputs(a Action, values []any) ([]any, error) {
	if len(values) != len(a.In) {
		return nil, fmt.Errorf("action requires %d input values", len(a.In))
	}
	out := make([]any, 0, len(values))
	for i, piid := range a.In {
		found := false
		for _, p := range s.Properties {
			if p.IID == piid {
				if err := p.Validate(values[i]); err != nil {
					return nil, err
				}
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("action references an unknown input property")
		}
		out = append(out, map[string]any{"piid": piid, "value": values[i]})
	}
	return out, nil
}
