package dcm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	AnnotationDCMCatalogInstanceID = "fleet.umkim.com/dcm-catalog-instance-id"
)

// Client calls the DCM control-plane catalog API.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type UserValue struct {
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

func (uv UserValue) StringValue() string {
	if len(uv.Value) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(uv.Value, &s); err == nil {
		return s
	}
	return strings.TrimSpace(string(uv.Value))
}

func userValueFromString(path, value string) UserValue {
	b, _ := json.Marshal(value)
	return UserValue{Path: path, Value: b}
}

// UserValueFromString builds a DCM user_value with a string JSON value.
func UserValueFromString(path, value string) UserValue {
	return userValueFromString(path, value)
}

type CatalogItemInstanceSpec struct {
	CatalogItemID   string                 `json:"catalog_item_id"`
	CatalogItemName string                 `json:"catalog_item_name,omitempty"`
	ServiceType     string                 `json:"service_type,omitempty"`
	UserValues      []UserValue            `json:"user_values"`
	ProviderHints   map[string]interface{} `json:"provider_hints,omitempty"`
	LabelSelector   map[string]string      `json:"label_selector,omitempty"`
}

type CatalogItemInstance struct {
	ID          string                  `json:"id"`
	UID         string                  `json:"uid"`
	Path        string                  `json:"path"`
	DisplayName string                  `json:"display_name"`
	Spec        CatalogItemInstanceSpec `json:"spec"`
}

func (c *CatalogItemInstance) InstanceID() string {
	if c.ID != "" {
		return c.ID
	}
	if c.Path != "" {
		if i := strings.LastIndex(c.Path, "/"); i >= 0 {
			return c.Path[i+1:]
		}
		return strings.TrimPrefix(c.Path, "catalog-item-instances/")
	}
	return c.UID
}

func (c *CatalogItemInstance) IsClusterInstance() bool {
	return c.Spec.ServiceType == "cluster"
}

func UserValueString(values []UserValue, path string) string {
	for _, uv := range values {
		if uv.Path == path {
			v := uv.StringValue()
			if v != "" {
				return v
			}
		}
	}
	return ""
}

func UserValueInt(values []UserValue, path string, def int) int {
	s := UserValueString(values, path)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// UserValueHelmField reads a Helm values field from a dotted user_values path or from the
// values YAML blob (path "values").
func UserValueHelmField(values []UserValue, keys ...string) string {
	if len(keys) == 0 {
		return ""
	}
	dotted := "values." + strings.Join(keys, ".")
	if v := UserValueString(values, dotted); v != "" {
		return v
	}
	raw := UserValueString(values, "values")
	if raw == "" {
		return ""
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		return ""
	}
	cur := interface{}(doc)
	for _, key := range keys {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return ""
		}
		cur, ok = m[key]
		if !ok {
			return ""
		}
	}
	return scalarToString(cur)
}

func UserValueHelmInt(values []UserValue, def int, keys ...string) int {
	s := UserValueHelmField(values, keys...)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func scalarToString(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.Itoa(int(x))
	default:
		return ""
	}
}

func (c *Client) ListCatalogItemInstances(ctx context.Context) ([]CatalogItemInstance, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/catalog-item-instances", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list catalog-item-instances: %s: %s", resp.Status, string(body))
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var items []CatalogItemInstance
	if err := json.Unmarshal(raw, &items); err == nil && len(items) > 0 {
		return items, nil
	}
	var wrapped struct {
		Results []CatalogItemInstance `json:"results"`
		Items   []CatalogItemInstance `json:"items"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("decode catalog-item-instances: %w", err)
	}
	if len(wrapped.Results) > 0 {
		return wrapped.Results, nil
	}
	return wrapped.Items, nil
}

func (c *Client) GetCatalogItemInstance(ctx context.Context, id string) (*CatalogItemInstance, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/catalog-item-instances/"+id, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("catalog-item-instance %s not found", id)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get catalog-item-instance %s: %s: %s", id, resp.Status, string(raw))
	}
	var inst CatalogItemInstance
	if err := json.Unmarshal(raw, &inst); err != nil {
		return nil, fmt.Errorf("decode catalog-item-instance: %w", err)
	}
	return &inst, nil
}

type PlacementTarget struct {
	Provider      string `json:"provider"`
	SprmID        string `json:"sprm_id"`
	Role          string `json:"role"`
	Status        string `json:"status"`
	StatusMessage string `json:"status_message"`
	Healthy       bool   `json:"healthy"`
}

type PlacementTargetsResponse struct {
	ResourceID     string            `json:"resource_id"`
	ApprovalStatus string            `json:"approval_status"`
	Targets        []PlacementTarget `json:"targets"`
	HealthyCount   int               `json:"healthy_count"`
	TargetCount    int               `json:"target_count"`
}

func (c *Client) GetPlacementTargets(ctx context.Context, instanceID string) (*PlacementTargetsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/catalog-item-instances/"+instanceID+"/placement-targets", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("placement-targets %s: %s: %s", instanceID, resp.Status, string(raw))
	}
	var out PlacementTargetsResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode placement-targets: %w", err)
	}
	return &out, nil
}

type CreateCatalogItemInstanceRequest struct {
	APIVersion  string `json:"api_version,omitempty"`
	DisplayName string `json:"display_name"`
	Spec        CatalogItemInstanceSpec `json:"spec"`
}

type CreateCatalogItemInstanceResponse struct {
	ID  string `json:"id"`
	UID string `json:"uid"`
}

func (c *Client) CreateCatalogItemInstance(ctx context.Context, req CreateCatalogItemInstanceRequest) (*CreateCatalogItemInstanceResponse, error) {
	if req.APIVersion == "" {
		req.APIVersion = "v1alpha1"
	}
	raw, err := c.PostJSON(ctx, "/catalog-item-instances", req)
	if err != nil {
		return nil, err
	}
	var out CreateCatalogItemInstanceResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode create catalog-item-instance: %w", err)
	}
	return &out, nil
}

func (c *Client) DeleteCatalogItemInstance(ctx context.Context, id string, deletionMode string) error {
	if deletionMode == "" {
		deletionMode = "strict"
	}
	endpoint := c.BaseURL + "/catalog-item-instances/" + id + "?deletion_mode=" + deletionMode
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete catalog-item-instance %s: %s: %s", id, resp.Status, string(body))
	}
	return nil
}

// InstanceTargetsCluster reports whether a catalog instance is placed on or routed to cluster.
func InstanceTargetsCluster(inst CatalogItemInstance, cluster string) bool {
	cluster = strings.TrimSpace(cluster)
	if cluster == "" {
		return false
	}
	for _, uv := range inst.Spec.UserValues {
		if uv.Path == "routing.cluster" && uv.StringValue() == cluster {
			return true
		}
	}
	return false
}

// ProviderNameTargetsCluster reports whether a DCM provider name belongs to cluster
// (e.g. helm-lab-gpu03, k8s-lab-gpu03).
func ProviderNameTargetsCluster(provider, cluster string) bool {
	provider = strings.TrimSpace(provider)
	cluster = strings.TrimSpace(cluster)
	if provider == "" || cluster == "" {
		return false
	}
	return strings.HasSuffix(provider, "-"+cluster)
}

func (c *Client) PostJSON(ctx context.Context, path string, body interface{}) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("POST %s: %s: %s", path, resp.Status, string(raw))
	}
	return raw, nil
}
