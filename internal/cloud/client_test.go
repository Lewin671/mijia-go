package cloud

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Lewin671/mijia-go/internal/state"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func testClient(t *testing.T) *Client {
	t.Helper()
	d, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	c := New(state.Store{Dir: filepath.Join(d, "state")}, time.Second)
	c.Auth = state.Auth{UserID: "123", CUserID: "user", PassToken: "private-pass", ServiceToken: "private-service", Security: base64.StdEncoding.EncodeToString([]byte("0123456789abcdef")), UserAgent: "test"}
	return c
}

func TestRedirectAndNetworkErrorRedaction(t *testing.T) {
	c := testClient(t)
	calls := 0
	c.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		res := response("")
		res.StatusCode = 302
		res.Header.Set("Location", "https://evil.example/steal")
		return res, nil
	})
	_, e := c.request(context.Background(), "GET", "https://account.xiaomi.com/?ticket=private-ticket", nil, nil)
	if e == nil || calls != 1 || strings.Contains(e.Error(), "private-ticket") {
		t.Fatal("redirect guard or error redaction failed")
	}
	c.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		t.Fatal("untrusted host reached transport")
		return nil, nil
	})
	for _, raw := range []string{"http://account.xiaomi.com/", "https://account.xiaomi.com.evil.example/", "https://user@account.xiaomi.com/", "https://account.xiaomi.com:8443/"} {
		if _, e = c.request(context.Background(), "GET", raw, nil, nil); e == nil {
			t.Fatal("accepted unsafe destination")
		}
	}
}

func TestReadRefreshButNeverReplayWrite(t *testing.T) {
	for _, read := range []bool{false, true} {
		t.Run(map[bool]string{false: "write", true: "read"}[read], func(t *testing.T) {
			c := testClient(t)
			apiCalls, refresh := 0, 0
			c.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/app/test":
					apiCalls++
					if apiCalls == 1 {
						return response(`{"code":-10030}`), nil
					}
					return response(`{"code":0,"result":{"value":true}}`), nil
				case "/pass/serviceLogin":
					refresh++
					return response(`{"code":0,"location":"https://api.mijia.tech/callback","ssecurity":"MDEyMzQ1Njc4OWFiY2RlZg=="}`), nil
				case "/callback":
					res := response("ok")
					res.Header.Add("Set-Cookie", "serviceToken=refreshed; Path=/; Secure")
					return res, nil
				default:
					t.Fatalf("unexpected request path %s", r.URL.Path)
					return nil, nil
				}
			})
			var dst map[string]any
			e := c.Call(context.Background(), "/test", map[string]any{}, &dst, read)
			if read {
				if e != nil || apiCalls != 2 || refresh != 1 {
					t.Fatalf("read refresh failed: %v", e)
				}
			} else if !errors.Is(e, ErrAuth) || apiCalls != 1 || refresh != 0 {
				t.Fatal("write was replayed")
			}
		})
	}
}

func TestLoginFlowAndCredentialIsolation(t *testing.T) {
	c := testClient(t)
	c.Auth = state.Auth{}
	c.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/pass/serviceLogin":
			return response(`&&&START&&&{"code":70016,"location":"https://account.xiaomi.com/pass/login?sid=mijia"}`), nil
		case "/longPolling/loginUrl":
			return response(`{"code":0,"loginUrl":"https://account.xiaomi.com/pass/qr?ticket=fake","lp":"https://account.xiaomi.com/poll"}`), nil
		case "/poll":
			return response(`{"code":0,"location":"https://api.mijia.tech/callback","ssecurity":"MDEyMzQ1Njc4OWFiY2RlZg==","passToken":"fake-pass","userId":123,"cUserId":"fake-user"}`), nil
		case "/callback":
			res := response("ok")
			res.Header.Set("Set-Cookie", "serviceToken=fake-service; Path=/; Secure")
			return res, nil
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
			return nil, nil
		}
	})
	shown := false
	e := c.Login(context.Background(), func(raw string) error {
		shown = true
		if !strings.Contains(raw, "ticket=fake") {
			t.Fatal("wrong challenge")
		}
		return nil
	})
	if e != nil || !shown {
		t.Fatalf("login failed: %v", e)
	}
	a, e := c.Store.LoadAuth()
	if e != nil || !a.Valid() || a.UserID != "123" {
		t.Fatal("credentials not persisted correctly")
	}
	if _, e = os.Stat(filepath.Join(c.Store.Dir, "auth.json")); e != nil {
		t.Fatal(e)
	}
}

func TestPropertyProtocolAndResponseIdentity(t *testing.T) {
	c := testClient(t)
	c.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("miot-encrypt-algorithm") != "ENCRYPT-RC4" {
			t.Fatal("missing protocol header")
		}
		if e := r.ParseForm(); e != nil {
			t.Fatal(e)
		}
		key, e := signedNonce(c.Auth.Security, r.Form.Get("_nonce"))
		if e != nil {
			t.Fatal(e)
		}
		raw, _ := base64.StdEncoding.DecodeString(r.Form.Get("data"))
		data, e := crypt(key, raw)
		if e != nil || !json.Valid(data) {
			t.Fatal("invalid encrypted request")
		}
		if !strings.Contains(string(data), `"did":"lamp"`) {
			t.Fatal("wrong device in request")
		}
		return response(`{"code":0,"result":[{"did":"other","siid":2,"piid":1,"code":0,"value":true}]}`), nil
	})
	if _, e := c.Get(context.Background(), "lamp", 2, 1); e == nil {
		t.Fatal("accepted another device's result")
	}
}

func TestSpecPathIsNeverUsedAsFilesystemPath(t *testing.T) {
	c := testClient(t)
	c.HTTP.Transport = transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("invalid URN sent to network"); return nil, nil })
	for _, urn := range []string{"../../auth", "/tmp/auth", "urn:miot-spec-v2:device:x:1:../escape:1"} {
		if _, e := c.Spec(context.Background(), urn); e == nil {
			t.Fatal("accepted malformed URN")
		}
	}
}

func TestWriteRedirectNeverReplaysAndCallbackNeedsFreshToken(t *testing.T) {
	c := testClient(t)
	calls := 0
	c.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		res := response("")
		res.StatusCode = 307
		res.Header.Set("Location", "https://api.mijia.tech/second")
		return res, nil
	})
	if _, e := c.request(context.Background(), "POST", "https://api.mijia.tech/first", nil, nil); e == nil || calls != 1 {
		t.Fatal("write redirect was followed")
	}
	u, _ := url.Parse("https://api.mijia.tech/callback")
	c.HTTP.Jar.SetCookies(u, []*http.Cookie{{Name: "serviceToken", Value: "stale", Path: "/"}})
	c.HTTP.Transport = transportFunc(func(*http.Request) (*http.Response, error) { return response("ok"), nil })
	if e := c.callback(context.Background(), u.String()); e == nil {
		t.Fatal("stale service token accepted as callback success")
	}
}
