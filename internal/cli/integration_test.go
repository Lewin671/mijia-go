package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Lewin671/mijia-go/internal/cloud"
	"github.com/Lewin671/mijia-go/internal/state"
)

type fakeTransport func(*http.Request) (*http.Response, error)

func (f fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDevicePreviewWriteAndReadBack(t *testing.T) {
	for _, match := range []bool{true, false} {
		t.Run(map[bool]string{true: "verified", false: "mismatch"}[match], func(t *testing.T) {
			dir, e := filepath.EvalSymlinks(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			s := state.Store{Dir: filepath.Join(dir, "state")}
			if e = s.SaveAuth(state.Auth{UserID: "123", CUserID: "fake", Security: "MDEyMzQ1Njc4OWFiY2RlZg==", PassToken: "fake", ServiceToken: "fake"}); e != nil {
				t.Fatal(e)
			}
			writes, reads := 0, 0
			a := app{store: s, timeout: time.Second, newClient: func(s state.Store, d time.Duration) *cloud.Client {
				c := cloud.New(s, d)
				c.HTTP.Transport = fakeTransport(func(r *http.Request) (*http.Response, error) {
					var body string
					switch r.URL.Path {
					case "/app/v2/homeroom/gethome_merged":
						body = `{"code":0,"result":{"homelist":[{"id":101,"uid":"123","name":"Synthetic home","address":"private-address-must-not-be-output"}]}}`
					case "/app/home/home_device_list":
						body = `{"code":0,"result":{"device_info":[{"did":"test-lamp","name":"Synthetic lamp","model":"test.light.v1","spec_type":"urn:miot-spec-v2:device:light:0000A001:test-v1:1","isOnline":true}],"has_more":false}}`
					case "/miot-spec-v2/instance":
						body = `{"type":"urn:miot-spec-v2:device:light:0000A001:test-v1:1","description":"Synthetic lamp","services":[{"iid":2,"properties":[{"iid":1,"type":"urn:miot-spec-v2:property:on:1","format":"bool","access":["read","write"]}]}]}`
					case "/app/miotspec/prop/set":
						writes++
						body = `{"code":0,"result":[{"did":"test-lamp","siid":2,"piid":1,"code":0}]}`
					case "/app/miotspec/prop/get":
						reads++
						if match {
							body = `{"code":0,"result":[{"did":"test-lamp","siid":2,"piid":1,"code":0,"value":true}]}`
						} else {
							body = `{"code":0,"result":[{"did":"test-lamp","siid":2,"piid":1,"code":0,"value":false}]}`
						}
					default:
						t.Fatalf("unexpected request: %s", r.URL.Path)
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
				})
				return c
			}}
			ctx := context.Background()
			base := []string{"device", "set", "--id", "test-lamp", "--property", "on", "--value", "true"}
			preview, e := a.execute(ctx, append(append([]string{}, base...), "--dry-run"))
			if e != nil || writes != 0 {
				t.Fatalf("preview sent write or failed: %v", e)
			}
			if preview.(map[string]any)["policy_allowed"] != false {
				t.Fatal("preview bypassed default policy")
			}
			if _, e = a.execute(ctx, []string{"policy", "allow-device", "--id", "test-lamp"}); e != nil {
				t.Fatal(e)
			}
			got, e := a.execute(ctx, base)
			if e != nil {
				t.Fatal(e)
			}
			result := got.(map[string]any)
			if result["verified"] != match || writes != 1 {
				t.Fatalf("incorrect write outcome: %v, writes=%d", got, writes)
			}
			if !match && reads != 3 {
				t.Fatal("unexpected verification read count")
			}
			devices, e := a.execute(ctx, []string{"devices", "list"})
			if e != nil {
				t.Fatal(e)
			}
			b, _ := json.Marshal(devices)
			if strings.Contains(string(b), "private-address") {
				t.Fatal("private home address leaked")
			}
		})
	}
}
