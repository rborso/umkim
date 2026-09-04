package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rborso/umkim/cmd/umkim/internal/config"
)

type Client struct {
	apiURL     string
	authURL    string
	token      string
	devMode    bool
	http       *http.Client
	cookiePath string
}

func New(cfg config.File) (*Client, error) {
	jar, _ := cookiejar.New(nil)
	c := &Client{
		apiURL:  strings.TrimRight(cfg.APIURL, "/"),
		authURL: strings.TrimRight(cfg.AuthURL, "/"),
		token:   strings.TrimSpace(cfg.Token),
		devMode: cfg.DevMode,
		http: &http.Client{
			Timeout: 120 * time.Second,
			Jar:     jar,
		},
	}
	dir, err := config.Dir()
	if err == nil {
		c.cookiePath = filepath.Join(dir, "cookies.json")
		_ = c.loadCookies(jar)
	}
	return c, nil
}

func (c *Client) APIURL() string  { return c.apiURL }
func (c *Client) AuthURL() string { return c.authURL }

func (c *Client) Login(username, password string) (map[string]interface{}, error) {
	if c.authURL == "" {
		return nil, fmt.Errorf("auth URL not configured (set UMKIM_AUTH_URL)")
	}
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req, err := http.NewRequest(http.MethodPost, c.authURL+"/login", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("login failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	_ = c.saveCookies()
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Logout() error {
	if c.authURL == "" {
		return nil
	}
	req, _ := http.NewRequest(http.MethodPost, c.authURL+"/logout", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	_ = c.saveCookies()
	return nil
}

func (c *Client) Whoami() (map[string]interface{}, error) {
	if c.authURL == "" {
		return nil, fmt.Errorf("auth URL not configured")
	}
	return c.authJSON(http.MethodGet, "/me", nil)
}

func (c *Client) Get(path string) ([]byte, int, error) {
	return c.do(http.MethodGet, path, nil)
}

func (c *Client) Patch(path string, body interface{}) ([]byte, int, error) {
	return c.do(http.MethodPatch, path, body)
}

func (c *Client) Post(path string, body interface{}) ([]byte, int, error) {
	return c.do(http.MethodPost, path, body)
}

func (c *Client) Delete(path string) ([]byte, int, error) {
	return c.DeleteQuery(path, nil)
}

func (c *Client) DeleteQuery(path string, query map[string]string) ([]byte, int, error) {
	if len(query) > 0 {
		u, err := url.Parse(path)
		if err != nil {
			return nil, 0, err
		}
		q := u.Query()
		for k, v := range query {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
		path = u.String()
	}
	return c.do(http.MethodDelete, path, nil)
}

func (c *Client) do(method, path string, body interface{}) ([]byte, int, error) {
	if c.apiURL == "" {
		return nil, 0, fmt.Errorf("API URL not configured (set UMKIM_API_URL)")
	}
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		r = bytes.NewReader(raw)
	}
	u := c.apiURL + path
	req, err := http.NewRequest(method, u, r)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.applyAuth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	_ = c.saveCookies()
	return raw, resp.StatusCode, nil
}

func (c *Client) authJSON(method, path string, body interface{}) (map[string]interface{}, error) {
	var r io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.authURL+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.applyAuth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out map[string]interface{}
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) applyAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
		return
	}
	if c.devMode {
		req.Header.Set("X-Fleet-Subject-Kind", "User")
		req.Header.Set("X-Fleet-Subject-Name", "umkim-cli")
		req.Header.Set("X-Fleet-DC-Admin", "true")
	}
}

func (c *Client) cookieBase() *url.URL {
	raw := c.authURL
	if raw == "" {
		raw = c.apiURL
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	u.Path = "/"
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u
}

func normalizeStoredCookies(cookies []*http.Cookie, base *url.URL) {
	if base == nil {
		return
	}
	host := base.Hostname()
	for _, ck := range cookies {
		if ck.Path == "" {
			ck.Path = "/"
		}
		if ck.Domain == "" && host != "" {
			ck.Domain = host
		}
	}
}

func (c *Client) saveCookies() error {
	if c.cookiePath == "" || c.http.Jar == nil {
		return nil
	}
	base := c.cookieBase()
	if base == nil || base.Scheme == "" {
		return nil
	}
	var cookies []*http.Cookie
	seen := map[string]bool{}
	for _, u := range []*url.URL{base, c.apiBase()} {
		if u == nil {
			continue
		}
		for _, ck := range c.http.Jar.Cookies(u) {
			key := ck.Name + "|" + ck.Value
			if seen[key] {
				continue
			}
			seen[key] = true
			cookies = append(cookies, ck)
		}
	}
	raw, err := json.Marshal(cookies)
	if err != nil {
		return err
	}
	return os.WriteFile(c.cookiePath, raw, 0o600)
}

func (c *Client) apiBase() *url.URL {
	u, err := url.Parse(c.apiURL)
	if err != nil {
		return nil
	}
	u.Path = "/"
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u
}

func (c *Client) loadCookies(jar http.CookieJar) error {
	if c.cookiePath == "" {
		return nil
	}
	raw, err := os.ReadFile(c.cookiePath)
	if err != nil {
		return err
	}
	var cookies []*http.Cookie
	if err := json.Unmarshal(raw, &cookies); err != nil {
		return err
	}
	base := c.cookieBase()
	normalizeStoredCookies(cookies, base)
	if base != nil {
		jar.SetCookies(base, cookies)
	}
	if api := c.apiBase(); api != nil && api.String() != base.String() {
		jar.SetCookies(api, cookies)
	}
	return nil
}

// DownloadGET writes a GET response body to destPath (for bootstrap bundles).
func (c *Client) DownloadGET(path, destPath string) error {
	raw, code, err := c.Get(path)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d: %s", path, code, strings.TrimSpace(string(raw)))
	}
	return os.WriteFile(destPath, raw, 0o600)
}

func JSONIndent(raw []byte) string {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(out)
}

func ExpectOK(raw []byte, code int, allowed ...int) error {
	for _, a := range allowed {
		if code == a {
			return nil
		}
	}
	if code >= 200 && code < 300 {
		return nil
	}
	return fmt.Errorf("HTTP %d: %s", code, strings.TrimSpace(string(raw)))
}
