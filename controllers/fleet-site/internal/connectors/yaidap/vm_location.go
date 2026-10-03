package yaidap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sitehosts"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PersistResolvedVmLocation writes a concrete hypervisor into guests.yml and host_vars
// after successful VM placement. Returns true when inventory files changed.
func PersistResolvedVmLocation(ctx context.Context, c client.Client, ns, siteName, guestName, hypervisor, actor string) (bool, error) {
	guestName = strings.TrimSpace(guestName)
	hypervisor = strings.TrimSpace(hypervisor)
	if guestName == "" || hypervisor == "" {
		return false, nil
	}
	if sitehosts.VmLocationIsScheduleToken(hypervisor) {
		return false, fmt.Errorf("refusing to persist schedule token %q", hypervisor)
	}
	if err := sitehosts.ValidateVmLocation("", hypervisor); err != nil {
		return false, err
	}

	changed := false
	err := connectors.PatchInventory(ctx, c, ns, siteName, provider, func(data map[string]string) (connectors.InventoryPatch, error) {
		data = connectors.NormalizeMetaKeys(data)
		guestsRaw, ok := connectors.GetInventoryFile(data, "guests.yml")
		if !ok {
			return connectors.InventoryPatch{}, fmt.Errorf("guests.yml missing")
		}
		guestsDoc, err := ParseGuestsYAML(guestsRaw)
		if err != nil {
			return connectors.InventoryPatch{}, err
		}
		if !vmGuestInIndex(guestName, guestsDoc) {
			return connectors.InventoryPatch{}, fmt.Errorf("guest %q not in guests.yml", guestName)
		}
		hvRaw, ok := connectors.GetInventoryFile(data, "hypervisors.yml")
		if !ok {
			return connectors.InventoryPatch{}, fmt.Errorf("hypervisors.yml missing")
		}
		hvDoc, err := ParseHypervisorsYAML(hvRaw)
		if err != nil {
			return connectors.InventoryPatch{}, err
		}
		if !hypervisorInIndex(hypervisor, hvDoc) {
			return connectors.InventoryPatch{}, fmt.Errorf("hypervisor %q not in hypervisors.yml", hypervisor)
		}

		hostVarsPath := "host_vars/" + guestName + ".yml"
		hostVarsRaw, ok := connectors.GetInventoryFile(data, hostVarsPath)
		if !ok || strings.TrimSpace(hostVarsRaw) == "" {
			return connectors.InventoryPatch{}, fmt.Errorf("%s missing", hostVarsPath)
		}

		newGuests, guestsChanged, err := patchGuestVmLocationInGuestsYAML(guestsRaw, guestName, hypervisor)
		if err != nil {
			return connectors.InventoryPatch{}, err
		}
		newHostVars, hvChanged, err := patchVmLocationInHostVarsYAML(hostVarsRaw, hypervisor)
		if err != nil {
			return connectors.InventoryPatch{}, err
		}
		if !guestsChanged && !hvChanged {
			return connectors.InventoryPatch{}, nil
		}
		changed = true

		var log connectors.RevisionLog
		if r, err := decodeRevisionLog(data[connectors.MetaKeyRevisions()]); err != nil {
			return connectors.InventoryPatch{}, err
		} else {
			log = r
		}
		log = connectors.AppendRevision(log, connectors.Revision{
			Actor:   actor,
			Scope:   connectors.RevisionScopeProvision,
			Summary: fmt.Sprintf("resolved vm_location %s for %s", hypervisor, guestName),
			After:   revisionTextJSON(newGuests),
		})
		meta, err := connectors.LoadMetaFromData(data, provider)
		if err != nil {
			return connectors.InventoryPatch{}, err
		}
		meta.RevisionHead = log.Items[0].ID
		meta.SyncPhase = "dirty"

		put := map[string]string{}
		if guestsChanged {
			put["guests.yml"] = newGuests
		}
		if hvChanged {
			put[hostVarsPath] = newHostVars
		}
		return connectors.InventoryPatch{
			Meta:      &meta,
			Revisions: &log,
			FilePut:   put,
		}, nil
	})
	if err != nil {
		return false, err
	}
	return changed, nil
}

func patchGuestVmLocationInGuestsYAML(raw, guestName, hypervisor string) (string, bool, error) {
	doc, err := ParseGuestsYAML(raw)
	if err != nil {
		return "", false, err
	}
	entry, inGuests := doc.Guests.Hosts[guestName]
	if inGuests {
		if strings.TrimSpace(entry.VmLocation) == hypervisor {
			return raw, false, nil
		}
		entry.VmLocation = hypervisor
		doc.Guests.Hosts[guestName] = entry
	} else {
		return "", false, fmt.Errorf("guest %q not in guests.yml", guestName)
	}
	out, err := marshalGuestsYAML(doc)
	if err != nil {
		return "", false, err
	}
	return out, true, nil
}

func patchVmLocationInHostVarsYAML(raw, hypervisor string) (string, bool, error) {
	doc := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		return "", false, fmt.Errorf("host_vars: %w", err)
	}
	current, _ := doc["vm_location"].(string)
	if strings.TrimSpace(current) == hypervisor {
		return raw, false, nil
	}
	doc["vm_location"] = hypervisor
	var body bytes.Buffer
	body.WriteString("---\n")
	enc := yaml.NewEncoder(&body)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return "", false, err
	}
	_ = enc.Close()
	return body.String(), true, nil
}

func marshalGuestsYAML(doc GuestsYAML) (string, error) {
	if doc.Guests.Hosts == nil {
		doc.Guests.Hosts = map[string]GuestEntry{}
	}
	if doc.External.Hosts == nil {
		doc.External.Hosts = map[string]GuestEntry{}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	_ = enc.Close()
	return strings.TrimRight(buf.String(), "\n") + "\n", nil
}

func revisionTextJSON(s string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{"text": s})
	return b
}
