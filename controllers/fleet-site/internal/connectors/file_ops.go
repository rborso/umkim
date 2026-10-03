package connectors

import (
	"context"
	"encoding/json"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FileValidator validates inventory file content before write.
type FileValidator func(path, content string) error

// PutInventoryFile writes or replaces one inventory file.
func PutInventoryFile(ctx context.Context, c client.Client, ns, siteName, provider, logicalPath, content, actor string, validate FileValidator) error {
	logicalPath = NormalizeInventoryPath(logicalPath)
	if logicalPath == "" {
		return ErrInvalidInventoryPath
	}
	if !InventoryPathAllowed(logicalPath) {
		return ErrInvalidInventoryPath
	}
	if validate != nil {
		if err := validate(logicalPath, content); err != nil {
			return err
		}
	}
	return PatchInventory(ctx, c, ns, siteName, provider, func(data map[string]string) (InventoryPatch, error) {
		data = NormalizeMetaKeys(data)
		before := data[FileKey(logicalPath)]
		var log RevisionLog
		if r, err := decodeRevisions(revisionsRaw(data)); err != nil {
			return InventoryPatch{}, err
		} else {
			log = r
		}
		log = AppendRevision(log, Revision{
			Actor:   actor,
			Scope:   RevisionScopeFile,
			Summary: "file " + logicalPath + " updated",
			Before:  rawTextJSON(before),
			After:   rawTextJSON(content),
		})
		meta, err := LoadMetaFromData(data, provider)
		if err != nil {
			return InventoryPatch{}, err
		}
		meta.RevisionHead = log.Items[0].ID
		if pathSynced(logicalPath) {
			meta.SyncPhase = "dirty"
		}
		return InventoryPatch{
			Meta:      &meta,
			Revisions: &log,
			FilePut:   map[string]string{logicalPath: content},
		}, nil
	})
}

// DeleteInventoryFile removes one inventory file.
func DeleteInventoryFile(ctx context.Context, c client.Client, ns, siteName, provider, logicalPath, actor string) error {
	logicalPath = NormalizeInventoryPath(logicalPath)
	if logicalPath == "" {
		return ErrInvalidInventoryPath
	}
	return PatchInventory(ctx, c, ns, siteName, provider, func(data map[string]string) (InventoryPatch, error) {
		data = NormalizeMetaKeys(data)
		before := data[FileKey(logicalPath)]
		if before == "" {
			return InventoryPatch{}, ErrInventoryFileNotFound
		}
		var log RevisionLog
		if r, err := decodeRevisions(revisionsRaw(data)); err != nil {
			return InventoryPatch{}, err
		} else {
			log = r
		}
		log = AppendRevision(log, Revision{
			Actor:   actor,
			Scope:   RevisionScopeFile,
			Summary: "file " + logicalPath + " deleted",
			Before:  rawTextJSON(before),
		})
		meta, err := LoadMetaFromData(data, provider)
		if err != nil {
			return InventoryPatch{}, err
		}
		meta.RevisionHead = log.Items[0].ID
		if pathSynced(logicalPath) {
			meta.SyncPhase = "dirty"
		}
		return InventoryPatch{
			Meta:       &meta,
			Revisions:  &log,
			FileDelete: []string{logicalPath},
		}, nil
	})
}

// PutInventoryFiles merges multiple files in one patch.
func PutInventoryFiles(ctx context.Context, c client.Client, ns, siteName, provider, actor string, files map[string]string, validate FileValidator) error {
	if len(files) == 0 {
		return nil
	}
	for p, content := range files {
		if validate != nil {
			if err := validate(NormalizeInventoryPath(p), content); err != nil {
				return err
			}
		}
	}
	return PatchInventory(ctx, c, ns, siteName, provider, func(data map[string]string) (InventoryPatch, error) {
		data = NormalizeMetaKeys(data)
		var log RevisionLog
		if r, err := decodeRevisions(revisionsRaw(data)); err != nil {
			return InventoryPatch{}, err
		} else {
			log = r
		}
		dirty := false
		put := map[string]string{}
		for p, content := range files {
			p = NormalizeInventoryPath(p)
			if p == "" {
				continue
			}
			put[p] = content
			if pathSynced(p) {
				dirty = true
			}
		}
		log = AppendRevision(log, Revision{
			Actor:   actor,
			Scope:   RevisionScopeImport,
			Summary: "import merge",
			After:   json.RawMessage(mustJSON(put)),
		})
		meta, err := LoadMetaFromData(data, provider)
		if err != nil {
			return InventoryPatch{}, err
		}
		meta.RevisionHead = log.Items[0].ID
		if dirty {
			meta.SyncPhase = "dirty"
		}
		return InventoryPatch{
			Meta:      &meta,
			Revisions: &log,
			FilePut:   put,
		}, nil
	})
}

func mustJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

func rawTextJSON(s string) json.RawMessage {
	if s == "" {
		return nil
	}
	b, _ := json.Marshal(s)
	return json.RawMessage(b)
}

// PutStatusJSON replaces _meta/status.json (hub-only).
func PutStatusJSON(ctx context.Context, c client.Client, ns, siteName, provider, content string) error {
	return PatchInventory(ctx, c, ns, siteName, provider, func(data map[string]string) (InventoryPatch, error) {
		return InventoryPatch{
			FilePut: map[string]string{"_meta/status.json": content},
		}, nil
	})
}

// GetStatusJSON reads _meta/status.json.
func GetStatusJSON(ctx context.Context, c client.Client, ns, siteName, provider string) (string, error) {
	data, err := Load(ctx, c, ns, siteName, provider)
	if err != nil {
		return "", err
	}
	if data == nil {
		return "{}", nil
	}
	data = NormalizeMetaKeys(data)
	if v := data[metaKeyStatus]; v != "" {
		return v, nil
	}
	return "{}", nil
}
