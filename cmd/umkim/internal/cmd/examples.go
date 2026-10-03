package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// commandExamples are cobra Example texts (printed under the Examples heading).
// Lines are indented with two spaces. Invocations use only flags and arguments
// the command accepts.
var commandExamples = map[string]string{
	"umkim": "  umkim site list",

	"umkim config":      "  umkim config show",
	"umkim config show": "  umkim config show",
	"umkim config set":  "  umkim config set apiUrl https://example.com/fleet/api/v1alpha1",

	"umkim login":  "  umkim login --username CHANGE_ME",
	"umkim logout": "  umkim logout",
	"umkim whoami": "  umkim whoami",

	"umkim auth":                         "  umkim auth users list",
	"umkim auth users":                   "  umkim auth users list",
	"umkim auth users list":              "  umkim auth users list",
	"umkim auth groups":                  "  umkim auth groups list",
	"umkim auth groups list":             "  umkim auth groups list",
	"umkim auth role-bindings":           "  umkim auth role-bindings list",
	"umkim auth role-bindings list":      "  umkim auth role-bindings list",
	"umkim auth role-bindings get":       "  umkim auth role-bindings get binding1",
	"umkim auth role-bindings create":    "  umkim auth role-bindings create -f rb.json",
	"umkim auth role-bindings delete":    "  umkim auth role-bindings delete binding1",
	"umkim auth service-accounts":        "  umkim auth service-accounts list",
	"umkim auth service-accounts list":   "  umkim auth service-accounts list",
	"umkim auth service-accounts create": "  umkim auth service-accounts create -f sa.json",
	"umkim auth audit":                   "  umkim auth audit list",
	"umkim auth audit list":              "  umkim auth audit list --limit 50",
	"umkim auth llm-api-keys":            "  umkim auth llm-api-keys list",
	"umkim auth llm-api-keys list":       "  umkim auth llm-api-keys list",
	"umkim auth llm-api-keys create":     "  umkim auth llm-api-keys create --name lab-1",
	"umkim auth llm-api-keys delete":     "  umkim auth llm-api-keys delete lab-1",
	"umkim auth llm-api-keys rotate":     "  umkim auth llm-api-keys rotate lab-1",
	"umkim auth llm-api-keys revoke":     "  umkim auth llm-api-keys revoke lab-1",
	"umkim auth llm-api-keys activate":   "  umkim auth llm-api-keys activate lab-1",

	"umkim site":                          "  umkim site list",
	"umkim site create":                   "  umkim site create -f site.json",
	"umkim site list":                     "  umkim site list",
	"umkim site get":                      "  umkim site get site1",
	"umkim site patch":                    "  umkim site patch site1 --region region1",
	"umkim site delete":                   "  umkim site delete site1 --confirm",
	"umkim site validate":                 "  umkim site validate site1",
	"umkim site ingress":                  "  umkim site ingress list site1",
	"umkim site ingress list":             "  umkim site ingress list site1",
	"umkim site ingress add":              "  umkim site ingress add site1 --id wan1 --method port_forward --zone example.com --wan-address-mode dns",
	"umkim site ingress set":              "  umkim site ingress set site1 wan1 --zone example.com",
	"umkim site ingress delete":           "  umkim site ingress delete site1 wan1",
	"umkim site host":                     "  umkim site host list site1",
	"umkim site host list":                "  umkim site host list site1",
	"umkim site host get":                 "  umkim site host get site1 host1",
	"umkim site host provision":           "  umkim site host provision site1 host1 --action status",
	"umkim site host prepare":             "  umkim site host prepare site1 host1",
	"umkim site host destroy":             "  umkim site host destroy site1 host1 --confirm",
	"umkim site host cluster-apply":       "  umkim site host cluster-apply site1 host1",
	"umkim site provision":                "  umkim site provision get site1",
	"umkim site provision get":            "  umkim site provision get site1",
	"umkim site provision set":            "  umkim site provision set site1 --provider yaidap",
	"umkim site provision run":            "  umkim site provision run site1 --action status --subject host1",
	"umkim site provision status":         "  umkim site provision status site1 action1",
	"umkim site provision actions":        "  umkim site provision actions site1",
	"umkim site provision yaidap-sync":    "  umkim site provision yaidap-sync site1",
	"umkim site inventory":                "  umkim site inventory get site1",
	"umkim site inventory get":            "  umkim site inventory get site1",
	"umkim site inventory revisions":      "  umkim site inventory revisions site1",
	"umkim site inventory sync":           "  umkim site inventory sync site1",
	"umkim site inventory files":          "  umkim site inventory files list site1",
	"umkim site inventory files list":     "  umkim site inventory files list site1",
	"umkim site inventory files get":      "  umkim site inventory files get site1 --path guests.yml",
	"umkim site inventory files put":      "  umkim site inventory files put site1 --path guests.yml -f guests.yml",
	"umkim site inventory files delete":   "  umkim site inventory files delete site1 --path guests.yml",
	"umkim site inventory migrate":        "  umkim site inventory migrate site1",
	"umkim site inventory import":         "  umkim site inventory import preview site1",
	"umkim site inventory import preview": "  umkim site inventory import preview site1",
	"umkim site inventory import apply":   "  umkim site inventory import apply site1 --confirm",

	"umkim cluster":      "  umkim cluster list",
	"umkim cluster list": "  umkim cluster list --require gpu",
	"umkim cluster candidates": `  umkim cluster candidates --require gpu
  umkim cluster candidates --require kvm,ha_topology
  umkim cluster candidates --require storage.rwx --site site1`,
	"umkim cluster get":                      "  umkim cluster get lab-1",
	"umkim cluster create":                   "  umkim cluster create -f cluster.json",
	"umkim cluster patch":                    "  umkim cluster patch lab-1 --ingress-id wan1",
	"umkim cluster delete":                   "  umkim cluster delete lab-1 --cleanup-kubernetes",
	"umkim cluster provision":                "  umkim cluster provision lab-1",
	"umkim cluster wait":                     "  umkim cluster wait lab-1 --phase Ready",
	"umkim cluster bootstrap":                "  umkim cluster bootstrap lab-1 --bootstrap-host 192.0.2.10",
	"umkim cluster validate":                 "  umkim cluster validate lab-1",
	"umkim cluster discover":                 "  umkim cluster discover lab-1 --wait 5m",
	"umkim cluster storage":                  "  umkim cluster storage status lab-1",
	"umkim cluster storage enable-local":     "  umkim cluster storage enable-local lab-1 --dir /srv/fleet-nfs",
	"umkim cluster storage validate":         "  umkim cluster storage validate lab-1 --server 192.0.2.20 --export-base /srv/fleet-nfs",
	"umkim cluster storage status":           "  umkim cluster storage status lab-1",
	"umkim cluster platform":                 "  umkim cluster platform status lab-1",
	"umkim cluster platform enable-kubevirt": "  umkim cluster platform enable-kubevirt lab-1 --lan-bridge",
	"umkim cluster platform status":          "  umkim cluster platform status lab-1",
	"umkim cluster gpu":                      "  umkim cluster gpu status lab-1",
	"umkim cluster gpu status":               "  umkim cluster gpu status lab-1",
	"umkim cluster gpu host-prep":            "  umkim cluster gpu host-prep lab-1 --wait 45m",
	"umkim cluster gpu enable":               "  umkim cluster gpu enable lab-1 --wait 30m",
	"umkim cluster gpu discover":             "  umkim cluster gpu discover lab-1",
	"umkim cluster ingress-auth":             "  umkim cluster ingress-auth enable lab-1 --lldap-password-file CHANGE_ME",
	"umkim cluster ingress-auth enable":      "  umkim cluster ingress-auth enable lab-1 --lldap-password-file CHANGE_ME",
	"umkim cluster ingress-auth disable":     "  umkim cluster ingress-auth disable lab-1",
	"umkim cluster acme":                     "  umkim cluster acme enable lab-1",
	"umkim cluster acme enable":              "  umkim cluster acme enable lab-1",
	"umkim cluster acme disable":             "  umkim cluster acme disable lab-1",

	"umkim app":        "  umkim app list",
	"umkim app list":   "  umkim app list",
	"umkim app get":    "  umkim app get app1",
	"umkim app create": "  umkim app create --cluster lab-1 --chart whoami --route-mode none",
	"umkim app update": "  umkim app update app1 --version 0.1.0 --route-mode hub",
	"umkim app delete": "  umkim app delete app1",

	"umkim chart":        "  umkim chart list",
	"umkim chart list":   "  umkim chart list",
	"umkim chart get":    "  umkim chart get whoami",
	"umkim chart add":    "  umkim chart add --origin oci://example.com/charts/whoami --version 0.1.0",
	"umkim chart delete": "  umkim chart delete whoami",

	"umkim infra":     "  umkim infra get hub",
	"umkim infra get": "  umkim infra get hub",

	"umkim ingress-auth":     "  umkim ingress-auth get",
	"umkim ingress-auth get": "  umkim ingress-auth get",
	"umkim ingress-auth set": "  umkim ingress-auth set --hub-enabled --portal-host auth.example.com",

	"umkim acme":     "  umkim acme get",
	"umkim acme get": "  umkim acme get",
	"umkim acme set": "  umkim acme set --hub-enabled --email admin@example.com",

	"umkim llm":                         "  umkim llm get",
	"umkim llm get":                     "  umkim llm get",
	"umkim llm set":                     "  umkim llm set --hub-enabled --public-host llm.example.com",
	"umkim llm reconcile":               "  umkim llm reconcile --wait",
	"umkim llm backends":                "  umkim llm backends sync-from-apps",
	"umkim llm backends sync-from-apps": "  umkim llm backends sync-from-apps",
	"umkim llm profile":                 "  umkim llm profile list",
	"umkim llm profile list":            "  umkim llm profile list",
	"umkim llm profile get":             "  umkim llm profile get profile1",
	"umkim llm profile create":          "  umkim llm profile create --id profile1 --profile-file profile.yaml",
	"umkim llm profile update":          "  umkim llm profile update profile1 --chart-version 0.1.0",
	"umkim llm profile delete":          "  umkim llm profile delete profile1",
	"umkim llm profile values":          "  umkim llm profile values profile1",
	"umkim llm profile verify":          "  umkim llm profile verify profile1 --cluster lab-1",
	"umkim llm profile test":            "  umkim llm profile test list",
	"umkim llm profile test list":       "  umkim llm profile test list",
	"umkim llm profile test get":        "  umkim llm profile test get test1",
	"umkim llm profile test end":        "  umkim llm profile test end test1",
	"umkim llm profile test chat":       "  umkim llm profile test chat test1 --message hello",

	"umkim edge":          "  umkim edge dns validate --host example.com",
	"umkim edge dns":      "  umkim edge dns validate --host example.com",
	"umkim edge hostname": "  umkim edge hostname check --host app.example.com",

	"umkim node":         "  umkim node list --cluster lab-1",
	"umkim node list":    "  umkim node list --cluster lab-1",
	"umkim node patch":   "  umkim node patch --cluster lab-1 node1 --label role=worker",
	"umkim node add":     "  umkim node add --cluster lab-1 --address 192.0.2.10",
	"umkim node remove":  "  umkim node remove --cluster lab-1 node1",
	"umkim node replace": "  umkim node replace --cluster lab-1 --old-node node1 --address 192.0.2.11",

	"umkim node-op":          "  umkim node-op history --cluster lab-1",
	"umkim node-op history":  "  umkim node-op history --cluster lab-1",
	"umkim node-op logs":     "  umkim node-op logs --cluster lab-1 --id op1",
	"umkim node-op validate": "  umkim node-op validate --cluster lab-1 -f body.json --phase pre",

	"umkim vm":         "  umkim vm list --cluster lab-1",
	"umkim vm list":    "  umkim vm list --cluster lab-1",
	"umkim vm get":     "  umkim vm get vm1 --cluster lab-1",
	"umkim vm start":   "  umkim vm start vm1 --cluster lab-1",
	"umkim vm stop":    "  umkim vm stop vm1 --cluster lab-1",
	"umkim vm restart": "  umkim vm restart vm1 --cluster lab-1",
	"umkim vm console": "  umkim vm console vm1 --cluster lab-1 --type serial",
}

func applyCommandExamples(root *cobra.Command) error {
	seen := map[string]struct{}{}
	var missing []string
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		path := c.CommandPath()
		seen[path] = struct{}{}
		ex, ok := commandExamples[path]
		if !ok || strings.TrimSpace(ex) == "" {
			missing = append(missing, path)
			return
		}
		c.Example = ex
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(root)
	var extra []string
	for path := range commandExamples {
		if _, ok := seen[path]; !ok {
			extra = append(extra, path)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		return fmt.Errorf("command examples mismatch: missing=%v extra=%v", missing, extra)
	}
	return nil
}
