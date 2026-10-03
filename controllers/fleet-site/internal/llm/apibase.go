package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// APIBaseFromValues resolves OpenAI-compatible apiBase and model alias from merged values YAML.
func APIBaseFromValues(ctx context.Context, c client.Client, cfg config.Config, clusterName, valuesYAML string) (apiBase, model string, err error) {
	clusterName = strings.TrimSpace(clusterName)
	if clusterName == "" {
		return "", "", fmt.Errorf("cluster name is required")
	}
	edge, err := loadEdgeUpstream(ctx, c, cfg.EdgeClustersCMNamespace, cfg.EdgeClustersCMName)
	if err != nil {
		return "", "", err
	}
	upstream := edge[clusterName]
	if upstream == "" {
		return "", "", fmt.Errorf("no edge upstream for cluster %q", clusterName)
	}
	vals := parseValues(valuesYAML)
	model = stringVal(vals, "model")
	if model == "" {
		model = stringVal(vals, "hfRepo")
	}
	port := intVal(vals, 8080, "service", "port")
	apiBase = fmt.Sprintf("http://%s:%d/v1", upstream, port)
	return apiBase, model, nil
}
