package capdiscover

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	AnnPhase = "fleet.umkim.com/capability-discovery-phase"
	AnnMsg   = "fleet.umkim.com/capability-discovery-message"
	AnnAt    = "fleet.umkim.com/capability-discovery-at"
	AnnNodes = "fleet.umkim.com/capability-discovery-nodes"
	AnnCaps  = "fleet.umkim.com/capability-discovery-cluster"

	PhaseIdle      = "Idle"
	PhaseRunning   = "Running"
	PhaseSucceeded = "Succeeded"
	PhaseFailed    = "Failed"
)

// NodeResult is per-node discovered capabilities.
type NodeResult struct {
	Name         string          `json:"name"`
	Caps         map[string]bool `json:"capabilities"`
	GPUCount     int             `json:"gpu_count,omitempty"`
	NvidiaPciID  string          `json:"nvidia_pci_id,omitempty"`
	DriverLoaded bool            `json:"driver_loaded,omitempty"`
}

// UnmarshalJSON accepts both numeric JSON values and numeric strings. Older
// runner images emitted the latter from nested Ansible templates.
func (n *NodeResult) UnmarshalJSON(data []byte) error {
	var raw struct {
		Name         string          `json:"name"`
		Caps         map[string]bool `json:"capabilities"`
		GPUCount     json.RawMessage `json:"gpu_count"`
		NvidiaPciID  string          `json:"nvidia_pci_id"`
		DriverLoaded bool            `json:"driver_loaded"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	count, err := decodeFlexibleInt(raw.GPUCount)
	if err != nil {
		return fmt.Errorf("gpu_count: %w", err)
	}
	n.Name = raw.Name
	n.Caps = raw.Caps
	n.GPUCount = count
	n.NvidiaPciID = raw.NvidiaPciID
	n.DriverLoaded = raw.DriverLoaded
	return nil
}

// IntMap is a JSON object whose values may be numbers or numeric strings.
type IntMap map[string]int

// UnmarshalJSON accepts numeric strings for compatibility with older runner
// results while keeping the in-memory representation strongly typed.
func (m *IntMap) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out := make(IntMap, len(raw))
	for key, value := range raw {
		n, err := decodeFlexibleInt(value)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		out[key] = n
	}
	*m = out
	return nil
}

func decodeFlexibleInt(raw json.RawMessage) (int, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return 0, nil
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, fmt.Errorf("expected integer or numeric string")
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("expected integer or numeric string: %q", s)
	}
	return n, nil
}

// Result is the outcome of a discovery run.
type Result struct {
	Phase               string          `json:"phase"`
	Message             string          `json:"message,omitempty"`
	CompletedAt         string          `json:"completed_at,omitempty"`
	Nodes               []NodeResult    `json:"nodes,omitempty"`
	ClusterCapabilities map[string]bool `json:"cluster_capabilities,omitempty"`
	ClusterResources    IntMap          `json:"cluster_resources,omitempty"`
}

// ErrAlreadyRunning is returned when discovery is already in progress.
var ErrAlreadyRunning = fmt.Errorf("capability discovery already running")

// Begin marks discovery as running; returns ErrAlreadyRunning if busy.
func Begin(ctx context.Context, hubClient client.Client, clusterName string) error {
	mc, err := getManagedCluster(ctx, hubClient, clusterName)
	if err != nil {
		return err
	}
	if IsRunning(mc.GetAnnotations()) {
		return ErrAlreadyRunning
	}
	return setDiscoveryPhase(ctx, hubClient, mc, PhaseRunning, "Discovery starting", nil)
}

// Fail records a failed discovery on the ManagedCluster.
func Fail(ctx context.Context, hubClient client.Client, clusterName, msg string) error {
	mc, err := getManagedCluster(ctx, hubClient, clusterName)
	if err != nil {
		return err
	}
	res := &Result{Phase: PhaseFailed, Message: msg, CompletedAt: time.Now().UTC().Format(time.RFC3339)}
	return setDiscoveryPhase(ctx, hubClient, mc, res.Phase, res.Message, res)
}

func getManagedCluster(ctx context.Context, c client.Client, name string) (*unstructured.Unstructured, error) {
	var mc unstructured.Unstructured
	mc.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	if err := c.Get(ctx, client.ObjectKey{Name: name}, &mc); err != nil {
		return nil, err
	}
	return &mc, nil
}

func setDiscoveryPhase(ctx context.Context, hubClient client.Client, mc *unstructured.Unstructured, phase, msg string, res *Result) error {
	ann := mc.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	if res != nil {
		if err := WriteResultAnnotations(ann, res); err != nil {
			return err
		}
	} else {
		ann[AnnPhase] = phase
		ann[AnnMsg] = msg
		if phase == PhaseRunning {
			delete(ann, AnnAt)
		}
	}
	mc.SetAnnotations(ann)
	return hubClient.Update(ctx, mc)
}

// WriteResultAnnotations merges a discovery result into annotation map.
func WriteResultAnnotations(ann map[string]string, res *Result) error {
	if ann == nil || res == nil {
		return nil
	}
	ann[AnnPhase] = res.Phase
	ann[AnnMsg] = res.Message
	if res.CompletedAt != "" {
		ann[AnnAt] = res.CompletedAt
	}
	if res.ClusterCapabilities != nil {
		b, err := json.Marshal(res.ClusterCapabilities)
		if err != nil {
			return err
		}
		ann[AnnCaps] = string(b)
	}
	if res.ClusterResources != nil {
		b, err := json.Marshal(res.ClusterResources)
		if err != nil {
			return err
		}
		ann["fleet.umkim.com/capability-discovery-resources"] = string(b)
	}
	if res.Nodes != nil {
		b, err := json.Marshal(res.Nodes)
		if err != nil {
			return err
		}
		ann[AnnNodes] = string(b)
	}
	return nil
}

// FromAnnotations parses discovery status from ManagedCluster annotations.
func FromAnnotations(ann map[string]string) *Result {
	if ann == nil {
		return nil
	}
	phase := ann[AnnPhase]
	if phase == "" {
		return nil
	}
	out := &Result{
		Phase:       phase,
		Message:     ann[AnnMsg],
		CompletedAt: ann[AnnAt],
	}
	if s := ann[AnnCaps]; s != "" {
		_ = json.Unmarshal([]byte(s), &out.ClusterCapabilities)
	}
	if s := ann[AnnNodes]; s != "" {
		_ = json.Unmarshal([]byte(s), &out.Nodes)
	}
	if s := ann["fleet.umkim.com/capability-discovery-resources"]; s != "" {
		_ = json.Unmarshal([]byte(s), &out.ClusterResources)
	}
	return out
}

// IsRunning reports whether a discovery run is in progress.
func IsRunning(ann map[string]string) bool {
	return ann != nil && ann[AnnPhase] == PhaseRunning
}

// ClusterNotFound wraps not-found for API mapping.
func ClusterNotFound(err error) bool {
	return apierrors.IsNotFound(err)
}
