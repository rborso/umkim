package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
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

func (c *Client) AuthGet(path string) ([]byte, int, error) {
	return c.authDo(http.MethodGet, path, nil)
}

func (c *Client) AuthPost(path string, body interface{}) ([]byte, int, error) {
	return c.authDo(http.MethodPost, path, body)
}

func (c *Client) AuthDelete(path string) ([]byte, int, error) {
	return c.authDo(http.MethodDelete, path, nil)
}

func (c *Client) authDo(method, path string, body interface{}) ([]byte, int, error) {
	if c.authURL == "" {
		return nil, 0, fmt.Errorf("auth URL not configured (set UMKIM_AUTH_URL)")
	}
	var r io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.authURL+path, r)
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

func (c *Client) Get(path string) ([]byte, int, error) {
	return c.do(http.MethodGet, path, nil)
}

func (c *Client) Patch(path string, body interface{}) ([]byte, int, error) {
	return c.do(http.MethodPatch, path, body)
}

func (c *Client) Put(path string, body interface{}) ([]byte, int, error) {
	return c.do(http.MethodPut, path, body)
}

// PutBytes sends a raw request body (for inventory YAML file PUT).
func (c *Client) PutBytes(path, contentType string, body []byte) ([]byte, int, error) {
	return c.doRaw(http.MethodPut, path, contentType, body)
}

func (c *Client) Post(path string, body interface{}) ([]byte, int, error) {
	return c.do(http.MethodPost, path, body)
}

// PostMultipart POSTs multipart/form-data (for FleetChart .tgz uploads).
// fileField is the form field name for the file (API expects "chart").
func (c *Client) PostMultipart(path, fileField, filePath string, fields map[string]string) ([]byte, int, error) {
	if c.apiURL == "" {
		return nil, 0, fmt.Errorf("API URL not configured (set UMKIM_API_URL)")
	}
	f, err := os.Open(filePath)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile(fileField, filepath.Base(filePath))
	if err != nil {
		return nil, 0, err
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, 0, err
	}
	for k, v := range fields {
		if strings.TrimSpace(v) == "" {
			continue
		}
		if err := w.WriteField(k, v); err != nil {
			return nil, 0, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, 0, err
	}

	u := c.apiURL + path
	req, err := http.NewRequest(http.MethodPost, u, &buf)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", w.FormDataContentType())
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
	return c.doRaw(method, path, "application/json", readAll(r))
}

func readAll(r io.Reader) []byte {
	if r == nil {
		return nil
	}
	raw, _ := io.ReadAll(r)
	return raw
}

func (c *Client) doRaw(method, path, contentType string, body []byte) ([]byte, int, error) {
	if c.apiURL == "" {
		return nil, 0, fmt.Errorf("API URL not configured (set UMKIM_API_URL)")
	}
	var r io.Reader
	if len(body) > 0 {
		r = bytes.NewReader(body)
	}
	u := c.apiURL + path
	req, err := http.NewRequest(method, u, r)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 && contentType != "" {
		req.Header.Set("Content-Type", contentType)
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

// WebsocketURL converts the fleet API path into a ws/wss URL.
func (c *Client) WebsocketURL(path string) (string, error) {
	if c.apiURL == "" {
		return "", fmt.Errorf("API URL not configured (set UMKIM_API_URL)")
	}
	u, err := url.Parse(c.apiURL + path)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	default:
		return "", fmt.Errorf("unsupported API URL scheme %q", u.Scheme)
	}
	return u.String(), nil
}

// SetAuthHeaders copies bearer/dev headers and session cookies for websocket dials.
func (c *Client) SetAuthHeaders(h http.Header) {
	req, _ := http.NewRequest(http.MethodGet, c.apiURL+"/", nil)
	c.applyAuth(req)
	for k, vals := range req.Header {
		for _, v := range vals {
			h.Add(k, v)
		}
	}
	if c.http != nil && c.http.Jar != nil {
		if u, err := url.Parse(c.apiURL); err == nil {
			for _, ck := range c.http.Jar.Cookies(u) {
				h.Add("Cookie", ck.Name+"="+ck.Value)
			}
		}
		if c.authURL != "" {
			if u, err := url.Parse(c.authURL); err == nil {
				for _, ck := range c.http.Jar.Cookies(u) {
					h.Add("Cookie", ck.Name+"="+ck.Value)
				}
			}
		}
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
