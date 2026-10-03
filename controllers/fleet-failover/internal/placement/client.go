package placement

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

type RelocateRequest struct {
	ExcludeClusters []string `json:"exclude_clusters"`
	Reason          string   `json:"reason,omitempty"`
}

type RelocateResponse struct {
	ActiveCluster string `json:"active_cluster"`
	Provider      string `json:"provider,omitempty"`
	SprmID        string `json:"sprm_id,omitempty"`
}

func (c *Client) Relocate(ctx context.Context, resourceID string, req RelocateRequest) (*RelocateResponse, error) {
	url := strings.TrimRight(c.BaseURL, "/") + "/placements/" + resourceID + "/relocate"
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("relocate %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out RelocateResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
