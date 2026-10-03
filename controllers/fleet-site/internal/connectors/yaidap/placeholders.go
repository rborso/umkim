package yaidap

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const inventoryPlaceholderMarker = "CHANGE_ME"

// InventoryPlaceholderWarnings reports YAML fields that still contain CHANGE_ME placeholders.
func InventoryPlaceholderWarnings(path, content string) []string {
	if !strings.Contains(content, inventoryPlaceholderMarker) {
		return nil
	}
	var doc interface{}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return []string{
			fmt.Sprintf("%s contains %s placeholder text; edit before the next YAIDAP/ansible run", path, inventoryPlaceholderMarker),
		}
	}
	var fields []string
	collectPlaceholderFields("", doc, &fields)
	if len(fields) == 0 {
		return []string{
			fmt.Sprintf("%s contains %s placeholder text; edit before the next YAIDAP/ansible run", path, inventoryPlaceholderMarker),
		}
	}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		out = append(out, fmt.Sprintf(
			"%s: %q is still %s — edit before the next YAIDAP/ansible run",
			path, field, inventoryPlaceholderMarker,
		))
	}
	return out
}

func collectPlaceholderFields(prefix string, v interface{}, out *[]string) {
	switch x := v.(type) {
	case map[string]interface{}:
		for k, val := range x {
			next := k
			if prefix != "" {
				next = prefix + "." + k
			}
			collectPlaceholderFields(next, val, out)
		}
	case []interface{}:
		for i, val := range x {
			next := fmt.Sprintf("%s[%d]", prefix, i)
			if prefix == "" {
				next = fmt.Sprintf("[%d]", i)
			}
			collectPlaceholderFields(next, val, out)
		}
	case string:
		if strings.Contains(x, inventoryPlaceholderMarker) {
			field := prefix
			if field == "" {
				field = "(root)"
			}
			*out = append(*out, field)
		}
	}
}

func attachImportWarnings(ch ImportFileChange) ImportFileChange {
	seen := map[string]bool{}
	for _, content := range []string{ch.Remote, ch.Local} {
		if strings.TrimSpace(content) == "" {
			continue
		}
		for _, w := range InventoryPlaceholderWarnings(ch.Path, content) {
			if !seen[w] {
				seen[w] = true
				ch.Warnings = append(ch.Warnings, w)
			}
		}
	}
	return ch
}
