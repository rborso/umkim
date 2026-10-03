package gpucatalog

import (
	_ "embed"
	"strings"
	"sync"

	"sigs.k8s.io/yaml"
)

//go:embed catalog.yaml
var catalogYAML []byte

type Catalog struct {
	PCIDevices map[string]PCIDevice `json:"pci_devices"`
}

type PCIDevice struct {
	MarketingName string `json:"marketing_name"`
	DriverBranch  string `json:"driver_branch"`
	VRAMGB        int    `json:"vram_gb"`
}

var (
	loadOnce sync.Once
	cached   *Catalog
	loadErr  error
)

func Load() (*Catalog, error) {
	loadOnce.Do(func() {
		var c Catalog
		if err := yaml.Unmarshal(catalogYAML, &c); err != nil {
			loadErr = err
			return
		}
		cached = &c
	})
	return cached, loadErr
}

func LookupPCI(pciID string) (PCIDevice, bool) {
	c, err := Load()
	if err != nil || c == nil {
		return PCIDevice{}, false
	}
	id := strings.ToLower(strings.TrimSpace(pciID))
	dev, ok := c.PCIDevices[id]
	return dev, ok
}
