package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const cmLabelApp = "fleet-site-provision-inventory"

// InventoryCMName returns the hub ConfigMap name for a provider inventory store.
func InventoryCMName(siteName, provider string) string {
	_ = provider
	return "fleet-site-" + siteName + "-yaidap-inventory"
}

// InventoryKeys are the standard ConfigMap data keys.
var InventoryKeys = []string{"meta.json", "catalog.json", "guests.json", "revisions.json"}

// InventoryPatch is the result of one inventory store mutation.
type InventoryPatch struct {
	Meta       *InventoryMeta
	Catalog    *json.RawMessage // legacy
	Guests     *json.RawMessage // legacy
	Revisions  *RevisionLog
	FilePut    map[string]string // logical path -> content
	FileDelete []string          // logical paths
}

type inventoryMutator func(data map[string]string) (InventoryPatch, error)

// Load reads all inventory keys from the provider ConfigMap.
func Load(ctx context.Context, c client.Client, ns, siteName, provider string) (map[string]string, error) {
	name := InventoryCMName(siteName, provider)
	var cm corev1.ConfigMap
	err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if cm.Data == nil {
		return map[string]string{}, nil
	}
	return cm.Data, nil
}

// LoadMeta returns meta.json or defaults for a missing store.
func LoadMeta(ctx context.Context, c client.Client, ns, siteName, provider string) (InventoryMeta, error) {
	data, err := Load(ctx, c, ns, siteName, provider)
	if err != nil {
		return InventoryMeta{}, err
	}
	return LoadMetaFromData(data, provider)
}

func defaultMeta(provider string) InventoryMeta {
	return InventoryMeta{
		Provider:      provider,
		SchemaVersion: 1,
		SyncPhase:     "pending",
	}
}

func decodeMeta(raw, provider string) (InventoryMeta, error) {
	if strings.TrimSpace(raw) == "" {
		return defaultMeta(provider), nil
	}
	var meta InventoryMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return InventoryMeta{}, fmt.Errorf("invalid meta.json: %w", err)
	}
	if meta.Provider == "" {
		meta.Provider = provider
	}
	if meta.SchemaVersion == 0 {
		meta.SchemaVersion = 1
	}
	return meta, nil
}

// LoadMetaFromData returns meta from a raw CM data map.
func LoadMetaFromData(data map[string]string, provider string) (InventoryMeta, error) {
	if data == nil {
		return defaultMeta(provider), nil
	}
	return decodeMeta(metaRaw(data), provider)
}

// PatchInventory applies a mutator with optimistic retry on the ConfigMap.
//
// The whole read-mutate-write sequence runs inside the retry so that a conflict
// re-reads the latest store and re-runs the mutator. Retrying only the write with
// the previous snapshot would drop concurrent changes (a lost update).
func PatchInventory(ctx context.Context, c client.Client, ns, siteName, provider string, mutate inventoryMutator) error {
	return retry.OnError(retry.DefaultRetry, func(err error) bool {
		return apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)
	}, func() error {
		data, err := Load(ctx, c, ns, siteName, provider)
		if err != nil {
			return err
		}
		if data == nil {
			data = map[string]string{}
		}
		patch, err := mutate(data)
		if err != nil {
			return err
		}
		if patch.Meta == nil && patch.Catalog == nil && patch.Guests == nil && patch.Revisions == nil &&
			len(patch.FilePut) == 0 && len(patch.FileDelete) == 0 {
			return nil
		}
		data = NormalizeMetaKeys(data)
		if patch.Meta != nil {
			raw, err := json.Marshal(patch.Meta)
			if err != nil {
				return err
			}
			data[metaKeyMeta] = string(raw)
			delete(data, legacyMetaKey)
		}
		if patch.Catalog != nil {
			data[legacyCatalogKey] = string(*patch.Catalog)
		}
		if patch.Guests != nil {
			data[legacyGuestsKey] = string(*patch.Guests)
		}
		if patch.Revisions != nil {
			raw, err := encodeRevisions(*patch.Revisions)
			if err != nil {
				return err
			}
			data[metaKeyRevisions] = raw
			delete(data, legacyRevisionsKey)
		}
		for _, p := range patch.FileDelete {
			p = NormalizeInventoryPath(p)
			if p == "" {
				continue
			}
			delete(data, FileKey(p))
		}
		for p, content := range patch.FilePut {
			p = NormalizeInventoryPath(p)
			if p == "" {
				continue
			}
			if p == "_meta/status.json" {
				data[metaKeyStatus] = content
				continue
			}
			if p == "_meta/import_preview.json" {
				data[metaKeyImportPreview] = content
				continue
			}
			data[FileKey(p)] = content
		}
		if err := CheckInventoryStoreSize(data); err != nil {
			return err
		}
		return writeInventoryOnce(ctx, c, ns, siteName, provider, data)
	})
}

// writeInventoryOnce performs a single read-modify-write of the inventory ConfigMap.
// Callers must retry via PatchInventory so the mutator re-runs on the latest store;
// retrying the write here would reapply a stale snapshot and drop concurrent changes.
func writeInventoryOnce(ctx context.Context, c client.Client, ns, siteName, provider string, data map[string]string) error {
	name := InventoryCMName(siteName, provider)
	var existing corev1.ConfigMap
	err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &existing)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels: map[string]string{
					"app":        cmLabelApp,
					"fleet-site": siteName,
					"provider":   provider,
				},
			},
			Data: data,
		}
		return c.Create(ctx, cm)
	}
	existing.Data = data
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	existing.Labels["app"] = cmLabelApp
	existing.Labels["fleet-site"] = siteName
	existing.Labels["provider"] = provider
	return c.Update(ctx, &existing)
}

// ListRevisions returns revision items newest-first.
func ListRevisions(ctx context.Context, c client.Client, ns, siteName, provider string, limit int) ([]Revision, error) {
	data, err := Load(ctx, c, ns, siteName, provider)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	log, err := decodeRevisions(revisionsRaw(data))
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit >= len(log.Items) {
		return log.Items, nil
	}
	return log.Items[:limit], nil
}

// GetRevision returns one revision by id.
func GetRevision(ctx context.Context, c client.Client, ns, siteName, provider, id string) (Revision, bool, error) {
	data, err := Load(ctx, c, ns, siteName, provider)
	if err != nil {
		return Revision{}, false, err
	}
	if data == nil {
		return Revision{}, false, nil
	}
	log, err := decodeRevisions(revisionsRaw(data))
	if err != nil {
		return Revision{}, false, err
	}
	rev, ok := findRevision(log, id)
	return rev, ok, nil
}
