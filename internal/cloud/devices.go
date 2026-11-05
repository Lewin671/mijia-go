package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type Home struct {
	ID   string      `json:"id"`
	UID  json.Number `json:"uid"`
	Name string      `json:"name"`
}
type Device struct {
	ID       string `json:"did"`
	Name     string `json:"name"`
	Model    string `json:"model"`
	SpecType string `json:"spec_type"`
	Online   bool   `json:"isOnline"`
	HomeID   string `json:"home_id,omitempty"`
}
type Scene struct {
	ID     string `json:"scene_id"`
	Name   string `json:"name"`
	HomeID string `json:"home_id"`
}

// The app endpoints return identifiers as either JSON strings or JSON numbers.
// Keep their exact decimal representation; never round an account/device ID.
func wireID(b json.RawMessage) (string, error) {
	var s string
	if json.Unmarshal(b, &s) == nil {
		if s == "" {
			return "", errors.New("empty cloud identifier")
		}
		return s, nil
	}
	var n json.Number
	if json.Unmarshal(b, &n) != nil || n.String() == "" || strings.ContainsAny(n.String(), ".eE+-") {
		return "", errors.New("invalid identifier in cloud response")
	}
	if _, err := strconv.ParseUint(n.String(), 10, 64); err != nil {
		return "", errors.New("invalid numeric cloud identifier")
	}
	return n.String(), nil
}
func (h *Home) UnmarshalJSON(b []byte) error {
	type alias Home
	v := struct {
		*alias
		ID json.RawMessage `json:"id"`
	}{alias: (*alias)(h)}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	id, err := wireID(v.ID)
	h.ID = id
	return err
}
func (d *Device) UnmarshalJSON(b []byte) error {
	type alias Device
	v := struct {
		*alias
		ID json.RawMessage `json:"did"`
	}{alias: (*alias)(d)}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	id, err := wireID(v.ID)
	d.ID = id
	return err
}
func (s *Scene) UnmarshalJSON(b []byte) error {
	type alias Scene
	v := struct {
		*alias
		ID json.RawMessage `json:"scene_id"`
	}{alias: (*alias)(s)}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	id, err := wireID(v.ID)
	s.ID = id
	return err
}

func (c *Client) Homes(ctx context.Context) ([]Home, error) {
	var r struct {
		Homes []Home `json:"homelist"`
	}
	err := c.Call(ctx, "/v2/homeroom/gethome_merged", map[string]any{"fg": true, "fetch_share": true, "fetch_share_dev": true, "limit": 300, "app_ver": 7, "plat_form": 0}, &r, true)
	return r.Homes, err
}

func (c *Client) Devices(ctx context.Context, homeID string) ([]Device, error) {
	homes, err := c.Homes(ctx)
	if err != nil {
		return nil, err
	}
	devices := []Device{}
	for _, h := range homes {
		if homeID != "" && h.ID != homeID {
			continue
		}
		start := ""
		seen := map[string]bool{}
		for page := 0; ; page++ {
			if page >= 100 {
				return nil, errors.New("device pagination exceeds limit")
			}
			var r struct {
				Devices []Device `json:"device_info"`
				More    bool     `json:"has_more"`
				MaxDID  string   `json:"max_did"`
			}
			err = c.Call(ctx, "/home/home_device_list", map[string]any{"home_owner": h.UID, "home_id": numberID(h.ID), "limit": 200, "start_did": start, "get_split_device": true, "support_smart_home": true, "get_cariot_device": true, "get_third_device": true}, &r, true)
			if err != nil {
				return nil, err
			}
			for _, d := range r.Devices {
				d.HomeID = h.ID
				devices = append(devices, d)
			}
			if !r.More {
				break
			}
			if r.MaxDID == "" || seen[r.MaxDID] {
				return nil, errors.New("invalid device pagination cursor")
			}
			start = r.MaxDID
			seen[start] = true
		}
	}
	return devices, nil
}

func numberID(s string) any {
	n, e := strconv.ParseInt(s, 10, 64)
	if e == nil {
		return n
	}
	return s
}

func (c *Client) FindDevice(ctx context.Context, id string) (Device, error) {
	ds, err := c.Devices(ctx, "")
	if err != nil {
		return Device{}, err
	}
	for _, d := range ds {
		if d.ID == id {
			return d, nil
		}
	}
	return Device{}, fmt.Errorf("device %q not found in selected account homes", id)
}

func (c *Client) Scenes(ctx context.Context) ([]Scene, error) {
	homes, err := c.Homes(ctx)
	if err != nil {
		return nil, err
	}
	out := []Scene{}
	for _, h := range homes {
		var r struct {
			Scenes []Scene `json:"manual_scene_info_list"`
		}
		err = c.Call(ctx, "/appgateway/miot/appsceneservice/AppSceneService/GetSimpleSceneList", map[string]any{"app_version": 12, "get_type": 2, "home_id": h.ID, "owner_uid": h.UID}, &r, true)
		if err != nil {
			return nil, err
		}
		for _, s := range r.Scenes {
			s.HomeID = h.ID
			out = append(out, s)
		}
	}
	return out, nil
}

func (c *Client) RunScene(ctx context.Context, id, home string) (any, error) {
	homes, err := c.Homes(ctx)
	if err != nil {
		return nil, err
	}
	var owner json.Number
	found := false
	for _, h := range homes {
		if h.ID == home {
			owner = h.UID
			found = true
		}
	}
	if !found {
		return nil, errors.New("scene home not found")
	}
	var out any
	err = c.Call(ctx, "/appgateway/miot/appsceneservice/AppSceneService/NewRunScene", map[string]any{"scene_id": id, "scene_type": 2, "phone_id": "null", "home_id": home, "owner_uid": owner}, &out, false)
	if err == nil {
		if accepted, ok := out.(bool); ok && !accepted {
			return nil, errors.New("cloud did not accept the scene")
		}
		if obj, ok := out.(map[string]any); ok {
			if code, ok := obj["code"].(float64); ok && code != 0 {
				return nil, &APIError{Code: int(code)}
			}
		}
	}
	return out, err
}

type Property struct {
	IID         int       `json:"iid"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Format      string    `json:"format"`
	Access      []string  `json:"access"`
	Range       []float64 `json:"value-range,omitempty"`
	Values      []struct {
		Value       any    `json:"value"`
		Description string `json:"description"`
	} `json:"value-list,omitempty"`
}
type Action struct {
	IID         int    `json:"iid"`
	Type        string `json:"type"`
	Description string `json:"description"`
	In          []int  `json:"in"`
}
type Service struct {
	IID         int        `json:"iid"`
	Type        string     `json:"type"`
	Description string     `json:"description"`
	Properties  []Property `json:"properties"`
	Actions     []Action   `json:"actions"`
}
type Spec struct {
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Services    []Service `json:"services"`
}

var validURN = regexp.MustCompile(`^urn:[a-zA-Z0-9_-]+:device:[a-zA-Z0-9_-]+:[a-zA-Z0-9_-]+:[a-zA-Z0-9_-]+:[0-9]+$`)

func (c *Client) Spec(ctx context.Context, urn string) (Spec, error) {
	if !validURN.MatchString(urn) {
		return Spec{}, errors.New("invalid device specification URN")
	}
	b, err := c.request(ctx, http.MethodGet, "https://"+SpecHost+"/miot-spec-v2/instance?type="+url.QueryEscape(urn), nil, nil)
	if err != nil {
		return Spec{}, err
	}
	var s Spec
	if err = decode(b, &s); err != nil {
		return s, err
	}
	if len(s.Services) == 0 || s.Type != urn {
		return Spec{}, errors.New("specification response does not match requested device")
	}
	return s, nil
}

type Result struct {
	DID   string `json:"did"`
	SIID  int    `json:"siid"`
	PIID  int    `json:"piid,omitempty"`
	Code  int    `json:"code"`
	Value any    `json:"value,omitempty"`
}

func (r *Result) UnmarshalJSON(b []byte) error {
	type alias Result
	v := struct {
		*alias
		Code *int `json:"code"`
	}{alias: (*alias)(r)}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.Code == nil {
		return errors.New("property response has no status code")
	}
	r.Code = *v.Code
	return nil
}

func (c *Client) Get(ctx context.Context, id string, siid, piid int) (Result, error) {
	var r []Result
	err := c.Call(ctx, "/miotspec/prop/get", map[string]any{"params": []any{map[string]any{"did": id, "siid": siid, "piid": piid}}, "datasource": 1}, &r, true)
	if err != nil {
		return Result{}, err
	}
	if len(r) != 1 {
		return Result{}, errors.New("unexpected property response count")
	}
	if r[0].DID != id || r[0].SIID != siid || r[0].PIID != piid {
		return Result{}, errors.New("property response does not match requested device")
	}
	if r[0].Code != 0 {
		return r[0], &APIError{Code: r[0].Code}
	}
	return r[0], nil
}
func (c *Client) Set(ctx context.Context, id string, siid, piid int, value any) (Result, error) {
	var r []Result
	err := c.Call(ctx, "/miotspec/prop/set", map[string]any{"params": []any{map[string]any{"did": id, "siid": siid, "piid": piid, "value": value}}}, &r, false)
	if err != nil {
		return Result{}, err
	}
	if len(r) != 1 {
		return Result{}, errors.New("unexpected property response count")
	}
	if r[0].DID != id || r[0].SIID != siid || r[0].PIID != piid {
		return Result{}, errors.New("write response does not match requested device; inspect state before retrying")
	}
	if r[0].Code != 0 && r[0].Code != 1 {
		return r[0], &APIError{Code: r[0].Code}
	}
	return r[0], nil
}
func (c *Client) Action(ctx context.Context, id string, siid, aiid int, input []any) (any, error) {
	var r struct {
		Code *int `json:"code"`
		Out  any  `json:"out"`
	}
	err := c.Call(ctx, "/miotspec/action", map[string]any{"params": map[string]any{"did": id, "siid": siid, "aiid": aiid, "in": input}}, &r, false)
	if err != nil {
		return nil, err
	}
	if r.Code == nil {
		return nil, errors.New("action response has no status code")
	}
	if *r.Code != 0 && *r.Code != 1 {
		return nil, &APIError{Code: *r.Code}
	}
	return map[string]any{"accepted": true, "verified": false, "code": *r.Code, "out": r.Out}, nil
}
