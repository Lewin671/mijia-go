package cloud

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"
)

type loginReply struct {
	Code     int    `json:"code"`
	Location string `json:"location"`
	Security string `json:"ssecurity"`
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}

func (c *Client) identity() error {
	if c.Auth.DeviceID == "" {
		v, err := randomHex(16)
		if err != nil {
			return err
		}
		c.Auth.DeviceID = v
	}
	if c.Auth.PassO == "" {
		v, err := randomHex(8)
		if err != nil {
			return err
		}
		c.Auth.PassO = v
	}
	if c.Auth.UserAgent == "" {
		id, err := randomHex(20)
		if err != nil {
			return err
		}
		c.Auth.UserAgent = "Android-15-11.0.701-Xiaomi-23046RP50C-OS2.0.212.0.VMYCNXM-" + id + "-CN-" + id + "-" + id + "-SmartHome-MI_APP_STORE-" + id + "|" + id + "|" + c.Auth.PassO + "-64"
	}
	return nil
}

func (c *Client) serviceLogin(ctx context.Context) (loginReply, error) {
	if err := c.identity(); err != nil {
		return loginReply{}, err
	}
	cks := []*http.Cookie{{Name: "deviceId", Value: c.Auth.DeviceID}, {Name: "pass_o", Value: c.Auth.PassO}, {Name: "passToken", Value: c.Auth.PassToken}, {Name: "userId", Value: c.Auth.UserID}, {Name: "cUserId", Value: c.Auth.CUserID}, {Name: "uLocale", Value: "zh_CN"}}
	b, err := c.request(ctx, http.MethodGet, "https://"+AccountHost+"/pass/serviceLogin?_json=true&sid=mijia&_locale=zh_CN", nil, cks)
	if err != nil {
		return loginReply{}, err
	}
	var ret loginReply
	if err = decode(b, &ret); err != nil {
		return ret, err
	}
	return ret, nil
}

func (c *Client) callback(ctx context.Context, raw string) error {
	if !validURL(raw, callbackHosts...) {
		return errors.New("untrusted authentication callback refused")
	}
	// A callback must issue a fresh token; a previously cached cookie is not proof.
	jar, _ := cookiejar.New(nil)
	callbackClient := *c
	h := *c.HTTP
	h.Jar = jar
	callbackClient.HTTP = &h
	if _, err := callbackClient.request(ctx, http.MethodGet, raw, nil, nil); err != nil {
		return err
	}
	u, _ := url.Parse(raw)
	token := ""
	for _, ck := range jar.Cookies(u) {
		if ck.Name == "serviceToken" {
			token = ck.Value
		}
	}
	if token == "" {
		return errors.New("login callback did not supply a service token")
	}
	c.Auth.ServiceToken = token
	return nil
}

func (c *Client) Refresh(ctx context.Context) error {
	ret, err := c.serviceLogin(ctx)
	if err != nil {
		return err
	}
	if ret.Code != 0 || ret.Security == "" {
		return ErrAuth
	}
	if err = c.callback(ctx, ret.Location); err != nil {
		return err
	}
	c.Auth.Security = ret.Security
	return c.Store.SaveAuth(c.Auth)
}

// Login displays a QR challenge through showQR, then waits for the user to scan it.
// Secrets and authentication responses are never passed to an output/logger.
func (c *Client) Login(ctx context.Context, showQR func(string) error) error {
	ret, err := c.serviceLogin(ctx)
	if err != nil {
		return err
	}
	if ret.Code == 0 && ret.Security != "" {
		if err = c.callback(ctx, ret.Location); err != nil {
			return err
		}
		c.Auth.Security = ret.Security
		return c.Store.SaveAuth(c.Auth)
	}
	u, err := url.Parse(ret.Location)
	if err != nil || !validURL(ret.Location, AccountHost) {
		return errors.New("unexpected sign-in response; use the Mi Home app to check the account")
	}
	q := u.Query()
	q.Set("theme", "")
	q.Set("bizDeviceType", "")
	q.Set("_hasLogo", "false")
	q.Set("_qrsize", "240")
	b, err := c.request(ctx, http.MethodGet, "https://"+AccountHost+"/longPolling/loginUrl?"+q.Encode(), nil, nil)
	if err != nil {
		return err
	}
	var challenge struct {
		Code     int    `json:"code"`
		LoginURL string `json:"loginUrl"`
		Poll     string `json:"lp"`
	}
	if err = decode(b, &challenge); err != nil {
		return err
	}
	if challenge.Code != 0 {
		return &APIError{Code: challenge.Code}
	}
	if !validURL(challenge.LoginURL, AccountHost) || !validURL(challenge.Poll, AccountHost) {
		return errors.New("untrusted QR challenge refused")
	}
	if err = showQR(challenge.LoginURL); err != nil {
		return err
	}
	// Long polling is bounded separately from ordinary device requests.
	poll := *c
	h := *c.HTTP
	h.Timeout = 125 * time.Second
	poll.HTTP = &h
	b, err = poll.request(ctx, http.MethodGet, challenge.Poll, nil, nil)
	if err != nil {
		return err
	}
	var result struct {
		Code      int             `json:"code"`
		Location  string          `json:"location"`
		Security  string          `json:"ssecurity"`
		PassToken string          `json:"passToken"`
		UserID    json.RawMessage `json:"userId"`
		CUserID   string          `json:"cUserId"`
	}
	if err = decode(b, &result); err != nil {
		return err
	}
	if result.Code != 0 {
		return &APIError{Code: result.Code}
	}
	if len(result.UserID) == 0 {
		return errors.New("incomplete sign-in response")
	}
	var uid string
	if json.Unmarshal(result.UserID, &uid) != nil {
		var num json.Number
		if json.Unmarshal(result.UserID, &num) != nil {
			return errors.New("invalid user ID")
		}
		uid = num.String()
	}
	c.Auth.UserID = uid
	c.Auth.CUserID = result.CUserID
	c.Auth.Security = result.Security
	c.Auth.PassToken = result.PassToken
	if err = c.callback(ctx, result.Location); err != nil {
		return err
	}
	if !c.Auth.Valid() {
		return errors.New("incomplete sign-in credentials")
	}
	return c.Store.SaveAuth(c.Auth)
}
