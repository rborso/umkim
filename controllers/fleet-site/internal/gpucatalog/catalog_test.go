package gpucatalog

import "testing"

func TestLookupPCI(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.PCIDevices) == 0 {
		t.Fatalf("empty pci_devices: %#v", c)
	}
	dev, ok := LookupPCI("10de:1c8c")
	if !ok {
		t.Fatal("expected 1050 Ti in catalog")
	}
	if dev.DriverBranch != "535" {
		t.Fatalf("branch: %s", dev.DriverBranch)
	}
	dev2, ok := LookupPCI("10de:1f82")
	if !ok || dev2.MarketingName == "" {
		t.Fatal("expected 2060 SUPER in catalog")
	}
}
