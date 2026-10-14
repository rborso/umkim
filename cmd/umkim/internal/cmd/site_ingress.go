package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rborso/umkim/cmd/umkim/internal/output"
	"github.com/spf13/cobra"
)

type siteIngressRow struct {
	ID             string   `json:"id"`
	Method         string   `json:"method"`
	Address        string   `json:"address,omitempty"`
	WanAddressMode string   `json:"wanAddressMode,omitempty"`
	Zone           string   `json:"zone,omitempty"`
	Wildcard       string   `json:"wildcard,omitempty"`
	Description    string   `json:"description,omitempty"`
	OperatorNotes  string   `json:"operatorNotes,omitempty"`
	Assigned       []string `json:"assignedClusters,omitempty"`
}

type siteIngressDoc struct {
	Spec struct {
		Edge struct {
			HubGateway  bool             `json:"hubGateway,omitempty"`
			HubZone     string           `json:"hubZone,omitempty"`
			HubWildcard string           `json:"hubWildcard,omitempty"`
			Ingress     []siteIngressRow `json:"ingress,omitempty"`
		} `json:"edge"`
	} `json:"spec"`
}

func siteIngressCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ingress",
		Short: "Manage FleetSite ingress entries (port_forward / reverse_proxy)",
	}
	cmd.AddCommand(siteIngressListCmd())
	cmd.AddCommand(siteIngressAddCmd())
	cmd.AddCommand(siteIngressSetCmd())
	cmd.AddCommand(siteIngressDeleteCmd())
	cmd.AddCommand(siteIngressVerifyCmd())
	return cmd
}

func siteIngressVerifyCmd() *cobra.Command {
	var id string
	c := &cobra.Command{
		Use:   "verify [site]",
		Short: "Read-only check of a site's ingress entries",
		Long: `Check a site's ingress declarations without changing anything.

Each entry is checked for: well-formed fields (zone, address per WAN mode), a
unique wildcard (two port_forward entries must not claim the same public
hostnames), the cluster binding and its cardinality, the site-mode routes
published through it (host under the wildcard, DNS resolution, expected WAN
address in fixed mode), and whether the bound spoke Traefik answers over the
WireGuard address.

The operator-managed WAN DNS and router port-forward cannot be checked from the
hub and are reported as skipped.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]interface{}{}
			if v := strings.TrimSpace(id); v != "" {
				body["id"] = v
			}
			raw, code, err := api.Post("/sites/"+args[0]+"/ingress/verify", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 200); err != nil {
				return err
			}
			format, ferr := outputFormat()
			if ferr != nil {
				return ferr
			}
			return output.WriteSiteIngressVerify(raw, format)
		},
	}
	c.Flags().StringVar(&id, "id", "", "Verify a single ingress entry id")
	return c
}

func siteIngressListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list [site]",
		Short: "List ingress entries for a site",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			doc, _, err := fetchSiteIngress(args[0])
			if err != nil {
				return err
			}
			format, err := outputFormat()
			if err != nil {
				return err
			}
			rows := make([]map[string]interface{}, 0, len(doc.Spec.Edge.Ingress))
			for _, row := range doc.Spec.Edge.Ingress {
				m := map[string]interface{}{
					"id":     row.ID,
					"method": row.Method,
				}
				if row.Address != "" {
					m["address"] = row.Address
				}
				if row.WanAddressMode != "" {
					m["wanAddressMode"] = row.WanAddressMode
				}
				if row.Zone != "" {
					m["zone"] = row.Zone
				}
				if row.Wildcard != "" {
					m["wildcard"] = row.Wildcard
				}
				if row.Description != "" {
					m["description"] = row.Description
				}
				if row.OperatorNotes != "" {
					m["operatorNotes"] = row.OperatorNotes
				}
				if len(row.Assigned) > 0 {
					m["assignedClusters"] = row.Assigned
				}
				rows = append(rows, m)
			}
			return output.WriteSiteIngress(args[0], doc.Spec.Edge.HubGateway, doc.Spec.Edge.HubZone, rows, format)
		},
	}
}

func siteIngressAddCmd() *cobra.Command {
	var id, method, address, zone, wildcard, description, notes, wanAddressMode string
	c := &cobra.Command{
		Use:   "add [site]",
		Short: "Add an ingress entry (PATCH /sites/{name})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id = strings.TrimSpace(id)
			method = strings.TrimSpace(method)
			if id == "" || method == "" {
				return fmt.Errorf("--id and --method required")
			}
			if method != "port_forward" && method != "reverse_proxy" {
				return fmt.Errorf("--method must be port_forward or reverse_proxy")
			}
			doc, _, err := fetchSiteIngress(args[0])
			if err != nil {
				return err
			}
			for _, row := range doc.Spec.Edge.Ingress {
				if row.ID == id {
					return fmt.Errorf("ingress id %q already exists on site %s", id, args[0])
				}
			}
			row := siteIngressRow{
				ID:             id,
				Method:         method,
				Address:        strings.TrimSpace(address),
				WanAddressMode: strings.TrimSpace(wanAddressMode),
				Zone:           strings.TrimSpace(zone),
				Wildcard:       strings.TrimSpace(wildcard),
				Description:    strings.TrimSpace(description),
				OperatorNotes:  strings.TrimSpace(notes),
			}
			rows := append(append([]siteIngressRow{}, doc.Spec.Edge.Ingress...), row)
			return patchSiteIngressAction(args[0], rows, fmt.Sprintf("entry %q added", id))
		},
	}
	c.Flags().StringVar(&id, "id", "", "Ingress entry id (required)")
	c.Flags().StringVar(&method, "method", "", "port_forward or reverse_proxy (required)")
	c.Flags().StringVar(&address, "address", "", "WAN address (required for port_forward with fixed mode)")
	c.Flags().StringVar(&wanAddressMode, "wan-address-mode", "", "WAN validation mode: fixed or dns")
	c.Flags().StringVar(&zone, "zone", "", "DNS zone (required for port_forward)")
	c.Flags().StringVar(&wildcard, "wildcard", "", "DNS wildcard (defaults to *.zone for port_forward)")
	c.Flags().StringVar(&description, "description", "", "Short description")
	c.Flags().StringVar(&notes, "notes", "", "Operator notes (reverse_proxy)")
	return c
}

func siteIngressSetCmd() *cobra.Command {
	var method, address, zone, wildcard, description, notes, newID, wanAddressMode string
	c := &cobra.Command{
		Use:   "set [site] [id]",
		Short: "Update fields on an ingress entry",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			doc, _, err := fetchSiteIngress(args[0])
			if err != nil {
				return err
			}
			id := strings.TrimSpace(args[1])
			idx := -1
			for i, row := range doc.Spec.Edge.Ingress {
				if row.ID == id {
					idx = i
					break
				}
			}
			if idx < 0 {
				return fmt.Errorf("ingress id %q not found on site %s", id, args[0])
			}
			row := doc.Spec.Edge.Ingress[idx]
			if cmd.Flags().Changed("new-id") {
				nid := strings.TrimSpace(newID)
				if nid == "" {
					return fmt.Errorf("--new-id cannot be empty")
				}
				for i, other := range doc.Spec.Edge.Ingress {
					if i != idx && other.ID == nid {
						return fmt.Errorf("ingress id %q already exists", nid)
					}
				}
				row.ID = nid
			}
			if cmd.Flags().Changed("method") {
				method = strings.TrimSpace(method)
				if method != "port_forward" && method != "reverse_proxy" {
					return fmt.Errorf("--method must be port_forward or reverse_proxy")
				}
				row.Method = method
			}
			if cmd.Flags().Changed("address") {
				row.Address = strings.TrimSpace(address)
			}
			if cmd.Flags().Changed("wan-address-mode") {
				row.WanAddressMode = strings.TrimSpace(wanAddressMode)
			}
			if cmd.Flags().Changed("zone") {
				row.Zone = strings.TrimSpace(zone)
			}
			if cmd.Flags().Changed("wildcard") {
				row.Wildcard = strings.TrimSpace(wildcard)
			}
			if cmd.Flags().Changed("description") {
				row.Description = strings.TrimSpace(description)
			}
			if cmd.Flags().Changed("notes") {
				row.OperatorNotes = strings.TrimSpace(notes)
			}
			rows := append([]siteIngressRow{}, doc.Spec.Edge.Ingress...)
			rows[idx] = row
			label := id
			if row.ID != id {
				label = id + " → " + row.ID
			}
			return patchSiteIngressAction(args[0], rows, fmt.Sprintf("entry %q updated", label))
		},
	}
	c.Flags().StringVar(&newID, "new-id", "", "Rename ingress entry id")
	c.Flags().StringVar(&method, "method", "", "port_forward or reverse_proxy")
	c.Flags().StringVar(&address, "address", "", "WAN address")
	c.Flags().StringVar(&wanAddressMode, "wan-address-mode", "", "WAN validation mode: fixed or dns")
	c.Flags().StringVar(&zone, "zone", "", "DNS zone")
	c.Flags().StringVar(&wildcard, "wildcard", "", "DNS wildcard")
	c.Flags().StringVar(&description, "description", "", "Short description")
	c.Flags().StringVar(&notes, "notes", "", "Operator notes")
	return c
}

func siteIngressDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete [site] [id]",
		Short: "Delete an ingress entry",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			doc, _, err := fetchSiteIngress(args[0])
			if err != nil {
				return err
			}
			id := strings.TrimSpace(args[1])
			out := make([]siteIngressRow, 0, len(doc.Spec.Edge.Ingress))
			found := false
			for _, row := range doc.Spec.Edge.Ingress {
				if row.ID == id {
					found = true
					continue
				}
				out = append(out, row)
			}
			if !found {
				return fmt.Errorf("ingress id %q not found on site %s", id, args[0])
			}
			return patchSiteIngressAction(args[0], out, fmt.Sprintf("entry %q deleted", id))
		},
	}
}

func fetchSiteIngress(name string) (*siteIngressDoc, []byte, error) {
	raw, code, err := api.Get("/sites/" + name)
	if err != nil {
		return nil, nil, err
	}
	if err := mustOK(raw, code); err != nil {
		return nil, nil, err
	}
	var doc siteIngressDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, nil, err
	}
	return &doc, raw, nil
}

func patchSiteIngress(site string, rows []siteIngressRow) error {
	return patchSiteIngressAction(site, rows, "")
}

func patchSiteIngressAction(site string, rows []siteIngressRow, action string) error {
	// Clear read-only assignedClusters before patch (API also strips them).
	clean := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		m := map[string]interface{}{
			"id":     row.ID,
			"method": row.Method,
		}
		if row.Address != "" {
			m["address"] = row.Address
		}
		if row.WanAddressMode != "" {
			m["wanAddressMode"] = row.WanAddressMode
		}
		if row.Zone != "" {
			m["zone"] = row.Zone
		}
		if row.Wildcard != "" {
			m["wildcard"] = row.Wildcard
		}
		if row.Description != "" {
			m["description"] = row.Description
		}
		if row.OperatorNotes != "" {
			m["operatorNotes"] = row.OperatorNotes
		}
		clean = append(clean, m)
	}
	body := map[string]interface{}{"ingress": clean}
	raw, code, err := api.Patch("/sites/"+site, body)
	if err != nil {
		return err
	}
	if err := mustOK(raw, code); err != nil {
		return err
	}
	format, ferr := outputFormat()
	if ferr != nil {
		return ferr
	}
	if action == "" {
		action = "updated"
	}
	if format == output.FormatJSON {
		printJSON(raw)
		return nil
	}
	fmt.Printf("OK: site %q ingress %s\n", site, action)
	doc, _, err := fetchSiteIngress(site)
	if err != nil {
		return fmt.Errorf("patch accepted but failed to re-read site: %w", err)
	}
	want := make(map[string]struct{}, len(clean))
	for _, m := range clean {
		id, _ := m["id"].(string)
		want[id] = struct{}{}
	}
	got := make(map[string]struct{}, len(doc.Spec.Edge.Ingress))
	for _, row := range doc.Spec.Edge.Ingress {
		got[row.ID] = struct{}{}
	}
	for id := range want {
		if _, ok := got[id]; !ok {
			return fmt.Errorf("patch reported OK but ingress %q was not persisted (is the FleetSite CRD up to date?)", id)
		}
	}
	if len(want) == 0 && len(doc.Spec.Edge.Ingress) != 0 {
		return fmt.Errorf("patch reported OK but site still has %d ingress entr(ies)", len(doc.Spec.Edge.Ingress))
	}
	if len(want) > 0 && len(doc.Spec.Edge.Ingress) != len(want) {
		return fmt.Errorf("patch reported OK but ingress count mismatch: want %d got %d", len(want), len(doc.Spec.Edge.Ingress))
	}
	outRows := make([]map[string]interface{}, 0, len(doc.Spec.Edge.Ingress))
	for _, row := range doc.Spec.Edge.Ingress {
		m := map[string]interface{}{
			"id":     row.ID,
			"method": row.Method,
		}
		if row.Address != "" {
			m["address"] = row.Address
		}
		if row.WanAddressMode != "" {
			m["wanAddressMode"] = row.WanAddressMode
		}
		if row.Zone != "" {
			m["zone"] = row.Zone
		}
		if row.Wildcard != "" {
			m["wildcard"] = row.Wildcard
		}
		if row.Description != "" {
			m["description"] = row.Description
		}
		if row.OperatorNotes != "" {
			m["operatorNotes"] = row.OperatorNotes
		}
		if len(row.Assigned) > 0 {
			m["assignedClusters"] = row.Assigned
		}
		outRows = append(outRows, m)
	}
	return output.WriteSiteIngress(site, doc.Spec.Edge.HubGateway, doc.Spec.Edge.HubZone, outRows, format)
}
