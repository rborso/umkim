package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// waitClusterCapabilityDiscovery triggers discovery (or waits for an in-flight run)
// and polls until phase Succeeded. When requireGPU is true, cluster capabilities.gpu must be true.
func waitClusterCapabilityDiscovery(clusterName string, timeout time.Duration, requireGPU bool) error {
	deadline := time.Now().Add(timeout)
	started := false
	for time.Now().Before(deadline) {
		if !started {
			raw, code, err := api.Post("/clusters/"+clusterName+"/discover-capabilities", nil)
			if err != nil {
				return err
			}
			switch code {
			case http.StatusOK:
				if requireGPU {
					if err := assertClusterGPUCapability(clusterName); err != nil {
						return err
					}
				}
				return nil
			case http.StatusAccepted, http.StatusConflict:
				// job started or already running; poll status only
			default:
				if err := mustOK(raw, code, http.StatusAccepted, http.StatusOK); err != nil {
					return err
				}
			}
			started = true
		}
		phase, err := fetchClusterDiscoveryPhase(clusterName)
		if err != nil {
			return err
		}
		switch phase {
		case "Succeeded":
			if requireGPU {
				if err := assertClusterGPUCapability(clusterName); err != nil {
					return err
				}
			}
			return nil
		case "Failed":
			return fmt.Errorf("capability discovery failed for cluster %q", clusterName)
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("timed out waiting for capability discovery on cluster %q", clusterName)
}

func assertClusterGPUCapability(clusterName string) error {
	raw, code, err := api.Get("/clusters/" + clusterName)
	if err != nil {
		return err
	}
	if err := mustOK(raw, code); err != nil {
		return err
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	caps, _ := doc["capabilities"].(map[string]interface{})
	if caps == nil {
		return fmt.Errorf("cluster %q: capabilities missing after discovery", clusterName)
	}
	gpu, _ := caps["gpu"].(bool)
	if !gpu {
		return fmt.Errorf("cluster %q: fleet.capability/gpu is still false after discovery (device plugin may not be advertising nvidia.com/gpu)", clusterName)
	}
	return nil
}
