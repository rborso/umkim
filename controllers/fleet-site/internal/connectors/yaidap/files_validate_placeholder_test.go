package yaidap

import (
	"strings"
	"testing"
)

// A placeholder cloud-init password becomes a known credential on every guest
// the site creates, so it is rejected at validation rather than shipped.
func TestGroupVarsRejectsPlaceholderCloudInitPassword(t *testing.T) {
	bad := []string{
		"cloudinit_password: \"CHANGE_ME\"\n",
		"cloudinit_password: changeme\n",
		"cloudinit_password: 'REPLACE_ME'\n",
	}
	for _, c := range bad {
		if err := validateGroupVarsYAML("group_vars/all/all.yml", c); err == nil {
			t.Errorf("accepted a placeholder password: %q", strings.TrimSpace(c))
		}
	}
	good := []string{
		"memory: 4096\ncores: 2\n",                          // no password at all: key auth
		"cloudinit_password: \"\"\n",                        // explicitly blank
		"cloudinit_password: \"a-real-generated-secret\"\n", // a real value
	}
	for _, c := range good {
		if err := validateGroupVarsYAML("group_vars/all/all.yml", c); err != nil {
			t.Errorf("rejected a valid file %q: %v", strings.TrimSpace(c), err)
		}
	}
}
