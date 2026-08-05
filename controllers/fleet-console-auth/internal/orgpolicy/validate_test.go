package orgpolicy

import (
	"testing"
)

func TestValidateMutation(t *testing.T) {
	body := []byte(`{
		"label_selector": {"fleet.org":"acme","fleet.placement/pool":"dedicated"},
		"cluster_requirements": {
			"matchLabels": {"fleet.org":"acme","fleet.pool":"dedicated"}
		}
	}`)
	if err := ValidateMutation("acme", "acme-dedicated-container", body); err != nil {
		t.Fatalf("valid policy: %v", err)
	}
	if err := ValidateMutation("acme", "other-policy", body); err == nil {
		t.Fatal("expected prefix error")
	}
	shared := []byte(`{
		"label_selector": {"fleet.org":"acme","fleet.placement/pool":"shared"},
		"cluster_requirements": {
			"matchLabels": {"fleet.org":"acme","fleet.pool":"dedicated"}
		}
	}`)
	if err := ValidateMutation("acme", "acme-bad", shared); err == nil {
		t.Fatal("expected shared pool reject")
	}
	wrongOrg := []byte(`{
		"label_selector": {"fleet.org":"beta"},
		"cluster_requirements": {
			"matchLabels": {"fleet.org":"beta","fleet.pool":"dedicated"}
		}
	}`)
	if err := ValidateMutation("acme", "acme-x", wrongOrg); err == nil {
		t.Fatal("expected org mismatch reject")
	}
	if err := ValidateMutation("acme", "acme-del", nil); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestBelongsToOrg(t *testing.T) {
	obj := map[string]interface{}{
		"id": "acme-foo",
		"label_selector": map[string]interface{}{"fleet.org": "acme"},
	}
	if !BelongsToOrg("acme", obj) {
		t.Fatal("expected belongs")
	}
	if BelongsToOrg("beta", obj) {
		t.Fatal("beta should not see acme policy")
	}
}
