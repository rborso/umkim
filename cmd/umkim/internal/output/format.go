package output

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rborso/umkim/cmd/umkim/internal/client"
)

// Format controls CLI presentation (kubectl-style).
type Format string

const (
	FormatTable Format = "table"
	FormatWide  Format = "wide"
	FormatJSON  Format = "json"
)

func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "table", "default":
		return FormatTable, nil
	case "wide":
		return FormatWide, nil
	case "json":
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("unknown output format %q (use table, wide, or json)", s)
	}
}

func WriteJSON(raw []byte) {
	fmt.Println(client.JSONIndent(raw))
}

func WriteClusters(raw []byte, format Format) error {
	if format == FormatJSON {
		WriteJSON(raw)
		return nil
	}
	var body struct {
		Clusters []map[string]interface{} `json:"clusters"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	if format == FormatWide {
		printTable(
			[]string{"NAME", "SITE", "PHASE", "READY", "TOPOLOGY", "INGRESS", "METHOD", "UPSTREAM", "JUMPHOST", "ACME"},
			body.Clusters,
			func(c map[string]interface{}) []string {
				return []string{
					strField(c, "name"),
					strField(c, "site"),
					clusterPhase(c),
					yesNo(boolField(c, "available")),
					clusterTopology(c),
					strField(c, "ingress_id"),
					strField(c, "method"),
					strField(c, "upstream_host"),
					strField(c, "jumphost"),
					yesNo(boolField(c, "acme_enabled")),
				}
			},
		)
		return nil
	}
	printTable(
		[]string{"NAME", "SITE", "PHASE", "READY"},
		body.Clusters,
		func(c map[string]interface{}) []string {
			return []string{
				strField(c, "name"),
				strField(c, "site"),
				clusterPhase(c),
				yesNo(boolField(c, "available")),
			}
		},
	)
	return nil
}

func WriteCharts(raw []byte, format Format) error {
	if format == FormatJSON {
		WriteJSON(raw)
		return nil
	}
	var body struct {
		Charts []map[string]interface{} `json:"charts"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	if format == FormatWide {
		printTable(
			[]string{"NAME", "SOURCE", "VERSION", "PHASE", "POLICY", "CHART", "ORIGIN"},
			body.Charts,
			func(c map[string]interface{}) []string {
				return []string{
					strField(c, "name"),
					strField(c, "source"),
					strField(c, "version"),
					strField(c, "phase"),
					strField(c, "imagePolicy"),
					strField(c, "chart"),
					truncate(strField(c, "origin"), 48),
				}
			},
		)
		return nil
	}
	printTable(
		[]string{"NAME", "SOURCE", "VERSION", "PHASE", "POLICY"},
		body.Charts,
		func(c map[string]interface{}) []string {
			return []string{
				strField(c, "name"),
				strField(c, "source"),
				strField(c, "version"),
				strField(c, "phase"),
				strField(c, "imagePolicy"),
			}
		},
	)
	return nil
}

// WriteSiteIngress prints ingress entries for one site (kubectl-style table).
func WriteSiteIngress(site string, hubGateway bool, hubZone string, rows []map[string]interface{}, format Format) error {
	if format == FormatJSON {
		payload, err := json.Marshal(map[string]interface{}{
			"site":       site,
			"hubGateway": hubGateway,
			"hubZone":    hubZone,
			"ingress":    rows,
		})
		if err != nil {
			return err
		}
		WriteJSON(payload)
		return nil
	}
	if hubGateway {
		fmt.Printf("SITE\t%s\nHUB_GATEWAY\tyes\n", site)
		if strings.TrimSpace(hubZone) != "" {
			fmt.Printf("HUB_ZONE\t%s\n", hubZone)
		}
	} else {
		fmt.Printf("SITE\t%s\nHUB_GATEWAY\tno\n", site)
	}
	if len(rows) == 0 {
		fmt.Println("No ingress entries.")
		return nil
	}
	headers := []string{"ID", "METHOD", "ADDRESS", "ZONE", "CLUSTERS"}
	if format == FormatWide {
		headers = []string{"ID", "METHOD", "ADDRESS", "ZONE", "WILDCARD", "CLUSTERS", "DESCRIPTION"}
	}
	printTable(headers, rows, func(r map[string]interface{}) []string {
		assigned := joinStrings(r, "assignedClusters")
		if assigned == "" {
			assigned = "—"
		}
		if format == FormatWide {
			desc := strField(r, "description")
			if desc == "" {
				desc = strField(r, "operatorNotes")
			}
			if desc == "" {
				desc = "—"
			}
			wc := strField(r, "wildcard")
			if wc == "" {
				wc = "—"
			}
			addr := strField(r, "address")
			if addr == "" {
				addr = "—"
			}
			zone := strField(r, "zone")
			if zone == "" {
				zone = "—"
			}
			return []string{
				strField(r, "id"),
				strField(r, "method"),
				addr,
				zone,
				wc,
				assigned,
				desc,
			}
		}
		addr := strField(r, "address")
		if addr == "" {
			addr = "—"
		}
		zone := strField(r, "zone")
		if zone == "" {
			zone = "—"
		}
		return []string{
			strField(r, "id"),
			strField(r, "method"),
			addr,
			zone,
			assigned,
		}
	})
	return nil
}

// WriteSiteIngressVerify renders the read-only site ingress verification
// response: one table row per (ingress entry, check).
func WriteSiteIngressVerify(raw []byte, format Format) error {
	if format == FormatJSON {
		WriteJSON(raw)
		return nil
	}
	var body struct {
		Site string                   `json:"site"`
		OK   bool                     `json:"ok"`
		Rows []map[string]interface{} `json:"ingress"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	okLabel := "no"
	if body.OK {
		okLabel = "yes"
	}
	fmt.Printf("SITE\t%s\nOK\t%s\n", body.Site, okLabel)
	rows := make([]map[string]interface{}, 0)
	for _, r := range body.Rows {
		id := strField(r, "id")
		checks, _ := r["checks"].([]interface{})
		for _, c := range checks {
			cm, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			rows = append(rows, map[string]interface{}{
				"id":     id,
				"check":  strField(cm, "name"),
				"status": strField(cm, "status"),
				"detail": strField(cm, "detail"),
			})
		}
	}
	if len(rows) == 0 {
		fmt.Println("No ingress checks.")
		return nil
	}
	printTable([]string{"ID", "CHECK", "STATUS", "DETAIL"}, rows, func(r map[string]interface{}) []string {
		return []string{strField(r, "id"), strField(r, "check"), strField(r, "status"), strField(r, "detail")}
	})
	return nil
}

func WriteSites(raw []byte, format Format) error {
	if format == FormatJSON {
		WriteJSON(raw)
		return nil
	}
	var body struct {
		Sites []map[string]interface{} `json:"sites"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	if format == FormatWide {
		printTable(
			[]string{"NAME", "SITE_ID", "PHASE", "CLUSTERS", "REGION", "INGRESS", "JUMPHOST"},
			body.Sites,
			func(s map[string]interface{}) []string {
				spec, _ := s["spec"].(map[string]interface{})
				status, _ := s["status"].(map[string]interface{})
				return []string{
					strField(s, "metadata", "name"),
					specStr(spec, "siteId", "siteName"),
					strField(status, "phase"),
					fmt.Sprintf("%d", intField(s, "clusterCount")),
					specStr(spec, "region"),
					siteIngressSummary(spec),
					specStr(spec, "jumphost"),
				}
			},
		)
		return nil
	}
	printTable(
		[]string{"NAME", "SITE_ID", "PHASE", "CLUSTERS", "INGRESS"},
		body.Sites,
		func(s map[string]interface{}) []string {
			spec, _ := s["spec"].(map[string]interface{})
			status, _ := s["status"].(map[string]interface{})
			return []string{
				strField(s, "metadata", "name"),
				specStr(spec, "siteId", "siteName"),
				strField(status, "phase"),
				fmt.Sprintf("%d", intField(s, "clusterCount")),
				siteIngressSummary(spec),
			}
		},
	)
	return nil
}

func WriteSiteHosts(raw []byte, format Format) error {
	if format == FormatJSON {
		WriteJSON(raw)
		return nil
	}
	var body struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	printTable(
		[]string{"NAME", "KIND", "PHASE", "ANSIBLE_HOST", "VM_LOCATION"},
		body.Items,
		func(h map[string]interface{}) []string {
			status, _ := h["status"].(map[string]interface{})
			return []string{
				strField(h, "name"),
				strField(h, "kind"),
				strField(status, "phase"),
				strField(h, "ansible_host"),
				strField(h, "vm_location"),
			}
		},
	)
	return nil
}

func siteIngressSummary(spec map[string]interface{}) string {
	if spec == nil {
		return "—"
	}
	edge, _ := spec["edge"].(map[string]interface{})
	if edge == nil {
		return "—"
	}
	parts := []string{}
	if boolField(edge, "hubGateway") {
		parts = append(parts, "hub_gateway")
	}
	raw, _ := edge["ingress"].([]interface{})
	for _, item := range raw {
		row, _ := item.(map[string]interface{})
		if row == nil {
			continue
		}
		id := strField(row, "id")
		method := strField(row, "method")
		if id == "" {
			continue
		}
		if method != "" {
			parts = append(parts, id+"("+method+")")
		} else {
			parts = append(parts, id)
		}
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, ",")
}

func WriteNodes(raw []byte, format Format) error {
	if format == FormatJSON {
		WriteJSON(raw)
		return nil
	}
	var body struct {
		Nodes []map[string]interface{} `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	if format == FormatWide {
		printTable(
			[]string{"NAME", "READY", "ROLES", "VERSION", "OS"},
			body.Nodes,
			func(n map[string]interface{}) []string {
				return []string{
					strField(n, "name"),
					yesNo(boolField(n, "ready")),
					joinStrings(n, "roles"),
					strField(n, "kubelet_version"),
					strField(n, "os_image"),
				}
			},
		)
		return nil
	}
	printTable(
		[]string{"NAME", "READY", "ROLES"},
		body.Nodes,
		func(n map[string]interface{}) []string {
			return []string{
				strField(n, "name"),
				yesNo(boolField(n, "ready")),
				joinStrings(n, "roles"),
			}
		},
	)
	return nil
}

func WriteVMs(raw []byte, format Format) error {
	if format == FormatJSON {
		WriteJSON(raw)
		return nil
	}
	var body struct {
		VMs []map[string]interface{} `json:"vms"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	headers := []string{"NAMESPACE", "NAME", "PHASE", "READY", "NODE", "IP"}
	if format == FormatWide {
		headers = []string{"NAMESPACE", "NAME", "PHASE", "READY", "STATUS", "NODE", "IP"}
	}
	printTable(headers, body.VMs, func(v map[string]interface{}) []string {
		row := []string{
			strField(v, "namespace"),
			strField(v, "name"),
			strField(v, "phase"),
			yesNo(boolField(v, "ready")),
		}
		if format == FormatWide {
			row = append(row, strField(v, "printableStatus"))
		}
		row = append(row, strField(v, "nodeName"), strField(v, "ip"))
		return row
	})
	return nil
}

func WriteNodeOps(raw []byte, format Format) error {
	if format == FormatJSON {
		WriteJSON(raw)
		return nil
	}
	var body struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	if format == FormatWide {
		printTable(
			[]string{"ID", "TYPE", "NODE", "PHASE", "STARTED", "ACTOR", "MESSAGE"},
			body.Items,
			func(it map[string]interface{}) []string {
				return []string{
					strField(it, "id"),
					strField(it, "type"),
					strField(it, "node"),
					strField(it, "phase"),
					strField(it, "startedAt"),
					actor(it),
					truncate(strField(it, "message"), 48),
				}
			},
		)
		return nil
	}
	printTable(
		[]string{"ID", "TYPE", "NODE", "PHASE", "STARTED"},
		body.Items,
		func(it map[string]interface{}) []string {
			return []string{
				strField(it, "id"),
				strField(it, "type"),
				strField(it, "node"),
				strField(it, "phase"),
				strField(it, "startedAt"),
			}
		},
	)
	return nil
}

func WriteApps(raw []byte, format Format) error {
	if format == FormatJSON {
		WriteJSON(raw)
		return nil
	}
	var body struct {
		Apps []map[string]interface{} `json:"apps"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	if format == FormatWide {
		printTable(
			[]string{"NAME", "CLUSTER", "CHART", "VERSION", "NAMESPACE", "PHASE", "ROUTE", "ROUTE_HOST"},
			body.Apps,
			func(a map[string]interface{}) []string {
				route, _ := a["route"].(map[string]interface{})
				mode := strField(route, "mode")
				host := strField(route, "hubHost")
				if host == "" {
					host = strField(route, "siteHost")
				}
				return []string{
					strField(a, "name"),
					strField(a, "clusterName"),
					truncate(strField(a, "chart"), 32),
					strField(a, "version"),
					strField(a, "namespace"),
					strField(a, "phase"),
					mode,
					host,
				}
			},
		)
		return nil
	}
	printTable(
		[]string{"NAME", "CLUSTER", "CHART", "PHASE", "ROUTE"},
		body.Apps,
		func(a map[string]interface{}) []string {
			route, _ := a["route"].(map[string]interface{})
			return []string{
				strField(a, "name"),
				strField(a, "clusterName"),
				truncate(strField(a, "chart"), 32),
				strField(a, "phase"),
				strField(route, "mode"),
			}
		},
	)
	return nil
}

func WriteHubInfrastructure(raw []byte, format Format) error {
	if format == FormatJSON {
		WriteJSON(raw)
		return nil
	}
	var hub map[string]interface{}
	if err := json.Unmarshal(raw, &hub); err != nil {
		return err
	}
	if format == FormatWide {
		printTable(
			[]string{"HOST", "SITE", "HUB_ZONE", "WIREGUARD", "GATEWAY", "PUBLIC"},
			[]map[string]interface{}{hub},
			func(h map[string]interface{}) []string {
				return []string{
					strField(h, "host_name"),
					strField(h, "management_site_id"),
					strField(h, "hub_zone"),
					strField(h, "wireguard_ip"),
					yesNo(boolField(h, "hub_gateway")),
					strField(h, "public_endpoint"),
				}
			},
		)
		return nil
	}
	fmt.Printf("HOST\t%s\nSITE\t%s\nZONE\t%s\nWIREGUARD\t%s\n",
		strField(hub, "host_name"),
		strField(hub, "management_site_id"),
		strField(hub, "hub_zone"),
		strField(hub, "wireguard_ip"),
	)
	return nil
}

func WriteAuthUsers(raw []byte, format Format) error {
	return writeAuthCRList(raw, format, "users", []string{"NAME", "DISPLAY", "AUTH", "GROUPS"},
		func(item map[string]interface{}) []string {
			spec, _ := item["spec"].(map[string]interface{})
			meta, _ := item["metadata"].(map[string]interface{})
			groups := joinStrings(spec, "groups")
			return []string{
				strField(meta, "name"),
				specStr(spec, "displayName"),
				specStr(spec, "authMethod"),
				truncate(groups, 40),
			}
		})
}

func WriteAuthGroups(raw []byte, format Format) error {
	return writeAuthCRList(raw, format, "groups", []string{"NAME", "DISPLAY"},
		func(item map[string]interface{}) []string {
			spec, _ := item["spec"].(map[string]interface{})
			meta, _ := item["metadata"].(map[string]interface{})
			return []string{strField(meta, "name"), specStr(spec, "displayName")}
		})
}

func WriteAuthRoleBindings(raw []byte, format Format) error {
	return writeAuthCRList(raw, format, "roleBindings", []string{"NAME", "ROLE", "SUBJECTS", "SCOPES"},
		func(item map[string]interface{}) []string {
			spec, _ := item["spec"].(map[string]interface{})
			meta, _ := item["metadata"].(map[string]interface{})
			roleRef, _ := spec["roleRef"].(map[string]interface{})
			subjects := formatSubjects(spec["subjects"])
			scopes := formatScopes(spec["scopes"])
			return []string{
				strField(meta, "name"),
				strField(roleRef, "name"),
				truncate(subjects, 40),
				truncate(scopes, 40),
			}
		})
}

func WriteAuthServiceAccounts(raw []byte, format Format) error {
	return writeAuthCRList(raw, format, "serviceAccounts", []string{"NAME", "DISPLAY"},
		func(item map[string]interface{}) []string {
			spec, _ := item["spec"].(map[string]interface{})
			meta, _ := item["metadata"].(map[string]interface{})
			return []string{strField(meta, "name"), specStr(spec, "displayName")}
		})
}

func WriteAuthAudit(raw []byte, format Format) error {
	if format == FormatJSON {
		WriteJSON(raw)
		return nil
	}
	var body struct {
		Events []map[string]interface{} `json:"auditEvents"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	printTable(
		[]string{"TIME", "ACTOR", "ACTION", "RESULT", "PATH"},
		body.Events,
		func(ev map[string]interface{}) []string {
			spec, _ := ev["spec"].(map[string]interface{})
			actorObj, _ := spec["actor"].(map[string]interface{})
			httpObj, _ := spec["http"].(map[string]interface{})
			actorStr := strField(actorObj, "kind")
			if n := strField(actorObj, "name"); n != "" {
				if actorStr != "" {
					actorStr += ":" + n
				} else {
					actorStr = n
				}
			}
			return []string{
				truncate(strField(spec, "timestamp"), 24),
				truncate(actorStr, 24),
				strField(spec, "action"),
				strField(spec, "result"),
				truncate(strField(httpObj, "path"), 48),
			}
		},
	)
	return nil
}

func WriteAuthLLMApiKeys(raw []byte, format Format) error {
	if format == FormatJSON {
		WriteJSON(raw)
		return nil
	}
	var body struct {
		APIEndpoint string                   `json:"apiEndpoint"`
		Keys        []map[string]interface{} `json:"keys"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	if body.APIEndpoint != "" {
		fmt.Printf("API_ENDPOINT\t%s\n", body.APIEndpoint)
	}
	printTable(
		[]string{"NAME", "DISPLAY", "PREFIX", "ACTIVE", "MODELS"},
		body.Keys,
		func(k map[string]interface{}) []string {
			spec, _ := k["spec"].(map[string]interface{})
			status, _ := k["status"].(map[string]interface{})
			meta, _ := k["metadata"].(map[string]interface{})
			return []string{
				strField(meta, "name"),
				specStr(spec, "displayName"),
				strField(status, "keyPrefix"),
				yesNo(boolField(status, "active")),
				truncate(joinStrings(spec, "models"), 32),
			}
		},
	)
	return nil
}

func writeAuthCRList(raw []byte, format Format, key string, headers []string, rowFn func(map[string]interface{}) []string) error {
	if format == FormatJSON {
		WriteJSON(raw)
		return nil
	}
	var body map[string][]map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	printTable(headers, body[key], rowFn)
	return nil
}

func formatSubjects(raw interface{}) string {
	list, ok := raw.([]interface{})
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		parts = append(parts, strField(m, "kind")+":"+strField(m, "name"))
	}
	return strings.Join(parts, ", ")
}

func formatScopes(raw interface{}) string {
	list, ok := raw.([]interface{})
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		parts = append(parts, strField(m, "kind")+"/"+strField(m, "name"))
	}
	return strings.Join(parts, ", ")
}

func clusterPhase(c map[string]interface{}) string {
	if p := strField(c, "provisioning_phase"); p != "" {
		return p
	}
	return strField(c, "phase")
}

func clusterTopology(c map[string]interface{}) string {
	if labels, ok := c["labels"].(map[string]interface{}); ok {
		if t := strField(labels, "fleet.topology/topology"); t != "" {
			return t
		}
	}
	topo, _ := c["topology"].(map[string]interface{})
	if topo == nil {
		return ""
	}
	if t := strField(topo, "topology"); t != "" {
		return t
	}
	if strField(topo, "ha") == "true" {
		return "ha"
	}
	return ""
}

func actor(it map[string]interface{}) string {
	k := strField(it, "actorKind")
	n := strField(it, "actorName")
	if k == "" && n == "" {
		return ""
	}
	if k == "" {
		return n
	}
	if n == "" {
		return k
	}
	return k + "/" + n
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func specStr(spec map[string]interface{}, keys ...string) string {
	if spec == nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := spec[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func strField(m map[string]interface{}, keys ...string) string {
	if m == nil {
		return ""
	}
	cur := m
	for i, k := range keys {
		v, ok := cur[k]
		if !ok {
			return ""
		}
		if i == len(keys)-1 {
			switch t := v.(type) {
			case string:
				return t
			case float64:
				return fmt.Sprintf("%v", t)
			case bool:
				if t {
					return "true"
				}
				return "false"
			default:
				return fmt.Sprintf("%v", v)
			}
		}
		next, ok := v.(map[string]interface{})
		if !ok {
			return ""
		}
		cur = next
	}
	return ""
}

func boolField(m map[string]interface{}, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

func intField(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	default:
		return 0
	}
}

func joinStrings(m map[string]interface{}, key string) string {
	raw, ok := m[key]
	if !ok || raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case []string:
		return strings.Join(v, ",")
	case []interface{}:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ",")
	default:
		return ""
	}
}

func printTable[T any](headers []string, rows []T, rowFn func(T) []string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	cells := make([][]string, len(rows))
	for i, row := range rows {
		cells[i] = rowFn(row)
		for j, c := range cells[i] {
			if len(c) > widths[j] {
				widths[j] = len(c)
			}
		}
	}
	fmt.Println(formatRow(headers, widths))
	for _, row := range cells {
		fmt.Println(formatRow(row, widths))
	}
}

func formatRow(cols []string, widths []int) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = fmt.Sprintf("%-*s", widths[i], c)
	}
	return strings.Join(parts, "  ")
}
