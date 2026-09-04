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
			[]string{"NAME", "SITE_ID", "PHASE", "CLUSTERS", "REGION", "EXPOSURE", "JUMPHOST"},
			body.Sites,
			func(s map[string]interface{}) []string {
				spec, _ := s["spec"].(map[string]interface{})
				status, _ := s["status"].(map[string]interface{})
				edge, _ := spec["edge"].(map[string]interface{})
				return []string{
					strField(s, "metadata", "name"),
					specStr(spec, "siteId", "siteName"),
					strField(status, "phase"),
					fmt.Sprintf("%d", intField(s, "clusterCount")),
					specStr(spec, "region"),
					specStr(edge, "exposureMode"),
					specStr(spec, "jumphost"),
				}
			},
		)
		return nil
	}
	printTable(
		[]string{"NAME", "SITE_ID", "PHASE", "CLUSTERS"},
		body.Sites,
		func(s map[string]interface{}) []string {
			spec, _ := s["spec"].(map[string]interface{})
			status, _ := s["status"].(map[string]interface{})
			return []string{
				strField(s, "metadata", "name"),
				specStr(spec, "siteId", "siteName"),
				strField(status, "phase"),
				fmt.Sprintf("%d", intField(s, "clusterCount")),
			}
		},
	)
	return nil
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
	raw, ok := m[key].([]interface{})
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ",")
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
