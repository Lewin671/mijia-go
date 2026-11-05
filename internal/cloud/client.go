// Package cloud implements the Xiaomi Mi Home app cloud protocol.
package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/Lewin671/mijia-go/internal/state"
)

const maxBody int64 = 8 << 20
const APIHost = "api.mijia.tech"
const AccountHost = "account.xiaomi.com"
const SpecHost = "miot-spec.org"

var callbackHosts = []string{AccountHost, APIHost, "sts.api.mijia.tech", "api.io.mi.com", "sts.api.io.mi.com"}

var ErrAuth = errors.New("authentication expired or missing; run mijia auth login")

type APIError struct{ Code int }

func (e *APIError) Error() string { return fmt.Sprintf("Xiaomi API returned code %d", e.Code) }

type Client struct {
	HTTP  *http.Client
	Auth  state.Auth
	Store state.Store
}

func New(store state.Store, timeout time.Duration) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{Store: store, HTTP: &http.Client{Timeout: timeout, Jar: jar, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if via[0].Method != http.MethodGet && via[0].Method != http.MethodHead {
			return errors.New("write redirects refused")
		}
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		// Never carry credentials across origins or downgrade HTTPS.
		if req.URL.Scheme != "https" || req.URL.Host != via[0].URL.Host {
			return errors.New("cross-origin redirect refused")
		}
		return nil
	}}}
}

func validURL(raw string, hosts ...string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" {
		return false
	}
	for _, h := range hosts {
		if u.Host == h {
			return true
		}
	}
	return false
}

func (c *Client) request(ctx context.Context, method, raw string, values url.Values, cookies []*http.Cookie) ([]byte, error) {
	if !validURL(raw, append(callbackHosts, SpecHost)...) {
		return nil, errors.New("untrusted network destination refused")
	}
	body := ""
	if values != nil {
		body = values.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, raw, strings.NewReader(body))
	if err != nil {
		return nil, errors.New("cannot construct request")
	}
	req.Header.Set("User-Agent", c.Auth.UserAgent)
	if req.URL.Host == APIHost {
		req.Header.Set("Accept-Encoding", "identity")
		req.Header.Set("miot-accept-encoding", "GZIP")
		req.Header.Set("miot-encrypt-algorithm", "ENCRYPT-RC4")
		req.Header.Set("x-xiaomi-protocal-flag-cli", "PROTOCAL-HTTP2")
	}
	if values != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// url.Error includes URLs, which can contain login tickets. Never return it.
		return nil, errors.New("network request failed or timed out; check connectivity")
	}
	defer res.Body.Close()
	if res.StatusCode == 401 || res.StatusCode == 403 {
		return nil, ErrAuth
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote server returned HTTP %d", res.StatusCode)
	}
	return readBounded(res.Body, maxBody)
}

func decode(body []byte, dst any) error {
	body = []byte(strings.TrimPrefix(string(body), "&&&START&&&"))
	if err := json.Unmarshal(body, dst); err != nil {
		return errors.New("remote server returned invalid JSON")
	}
	return nil
}

func (c *Client) Load() error {
	a, err := c.Store.LoadAuth()
	if err != nil {
		return fmt.Errorf("cannot load credentials: %w", err)
	}
	if !a.Valid() {
		return ErrAuth
	}
	c.Auth = a
	return nil
}

func (c *Client) callOnce(ctx context.Context, path string, data any, dst any) error {
	if !c.Auth.Valid() {
		return ErrAuth
	}
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	n, err := nonce()
	if err != nil {
		return err
	}
	params, key, err := encryptRequest(path, string(b), c.Auth.Security, n)
	if err != nil {
		return err
	}
	cookies := []*http.Cookie{{Name: "cUserId", Value: c.Auth.CUserID}, {Name: "serviceToken", Value: c.Auth.ServiceToken}, {Name: "yetAnotherServiceToken", Value: c.Auth.ServiceToken}, {Name: "locale", Value: "zh_CN"}, {Name: "countryCode", Value: "CN"}, {Name: "channel", Value: "MI_APP_STORE"}, {Name: "PassportDeviceId", Value: c.Auth.DeviceID}}
	ret, err := c.request(ctx, http.MethodPost, "https://"+APIHost+"/app"+path, params, cookies)
	if err != nil {
		return err
	}
	if !json.Valid(ret) {
		ret, err = decryptResponse(key, ret)
		if err != nil {
			return err
		}
	}
	var envelope struct {
		Code   *int            `json:"code"`
		Result json.RawMessage `json:"result"`
	}
	if err := decode(ret, &envelope); err != nil {
		return err
	}
	if envelope.Code == nil {
		return errors.New("cloud response has no status code")
	}
	if *envelope.Code == -10030 || *envelope.Code == -3 {
		return ErrAuth
	}
	if *envelope.Code != 0 {
		return &APIError{Code: *envelope.Code}
	}
	if len(envelope.Result) == 0 {
		return errors.New("cloud response has no result")
	}
	return decode(envelope.Result, dst)
}

// Call retries only reads after refreshing credentials. Writes are never replayed.
func (c *Client) Call(ctx context.Context, path string, data any, dst any, readOnly bool) error {
	err := c.callOnce(ctx, path, data, dst)
	if !readOnly || !errors.Is(err, ErrAuth) {
		return err
	}
	if e := c.Refresh(ctx); e != nil {
		return e
	}
	return c.callOnce(ctx, path, data, dst)
}
