package harborcharts

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Repository struct {
	Name     string
	Versions []string
}

type Client struct {
	BaseURL  string
	User     string
	Password string
	HTTP     *http.Client
}

func NewClient(host, user, password string) *Client {
	host = strings.TrimRight(strings.TrimSpace(host), "/")
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "https://" + host
	}
	return &Client{
		BaseURL:  host,
		User:     user,
		Password: password,
		HTTP: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // lab Harbor NodePort uses the hub self-signed cert
			},
		},
	}
}

func (c *Client) ListCharts(project string) ([]Repository, error) {
	if c == nil || c.HTTP == nil {
		return nil, fmt.Errorf("harbor client is not configured")
	}
	project = strings.TrimSpace(project)
	if project == "" {
		project = "charts"
	}
	u := fmt.Sprintf("%s/api/v2.0/projects/%s/repositories?page_size=100", strings.TrimRight(c.BaseURL, "/"), url.PathEscape(project))
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.User, c.Password)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("harbor list repositories: HTTP %d", resp.StatusCode)
	}
	var repos []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &repos); err != nil {
		return nil, err
	}
	out := make([]Repository, 0, len(repos))
	for _, r := range repos {
		name := r.Name
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		vers, err := c.listVersions(project, name)
		if err != nil {
			vers = nil
		}
		out = append(out, Repository{Name: name, Versions: vers})
	}
	return out, nil
}

func (c *Client) listVersions(project, repo string) ([]string, error) {
	u := fmt.Sprintf("%s/api/v2.0/projects/%s/repositories/%s/artifacts?page_size=50",
		strings.TrimRight(c.BaseURL, "/"), url.PathEscape(project), url.PathEscape(repo))
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.User, c.Password)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("harbor artifacts: HTTP %d", resp.StatusCode)
	}
	var arts []struct {
		Tags []struct {
			Name string `json:"name"`
		} `json:"tags"`
	}
	if err := json.Unmarshal(body, &arts); err != nil {
		return nil, err
	}
	var vers []string
	seen := map[string]struct{}{}
	for _, a := range arts {
		for _, t := range a.Tags {
			n := strings.TrimSpace(t.Name)
			if n == "" {
				continue
			}
			if _, ok := seen[n]; ok {
				continue
			}
			seen[n] = struct{}{}
			vers = append(vers, n)
		}
	}
	return vers, nil
}
