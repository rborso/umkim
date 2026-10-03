package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ansiblelogs"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type nodeOperationHistoryItem struct {
	ID         string `json:"id"`
	Type       string `json:"type,omitempty"`
	Node       string `json:"node,omitempty"`
	Phase      string `json:"phase,omitempty"`
	JobName    string `json:"jobName,omitempty"`
	Message    string `json:"message,omitempty"`
	Force      bool   `json:"force,omitempty"`
	ActorKind  string `json:"actorKind,omitempty"`
	ActorName  string `json:"actorName,omitempty"`
	StartedAt  string `json:"startedAt,omitempty"`
	FinishedAt string `json:"finishedAt,omitempty"`
	HasLogs    bool   `json:"hasLogs,omitempty"`
}

type nodeOperationHistoryList struct {
	Items []nodeOperationHistoryItem `json:"items"`
}

type nodeOperationLogsResponse struct {
	ID      string `json:"id"`
	JobName string `json:"jobName,omitempty"`
	Logs    string `json:"logs"`
	Source  string `json:"source,omitempty"`
}

func (s *Server) handleNodeOperations(w http.ResponseWriter, r *http.Request, clusterName, rest string) {
	rest = strings.Trim(rest, "/")
	if rest == "" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleListNodeOperations(r.Context(), w, clusterName)
		return
	}
	if strings.HasSuffix(rest, "/logs") {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := strings.TrimSuffix(rest, "/logs")
		id = strings.Trim(id, "/")
		if id == "" {
			http.Error(w, "operation id required", http.StatusBadRequest)
			return
		}
		s.handleNodeOperationLogs(r.Context(), w, clusterName, id)
		return
	}
	http.Error(w, "not found", http.StatusNotFound)
}

func (s *Server) handleListNodeOperations(ctx context.Context, w http.ResponseWriter, clusterName string) {
	fcj, err := s.findFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]nodeOperationHistoryItem, 0, len(fcj.Status.NodeOperationHistory))
	for _, rec := range fcj.Status.NodeOperationHistory {
		items = append(items, historyItemFromRecord(rec))
	}
	writeJSON(w, http.StatusOK, nodeOperationHistoryList{Items: items})
}

func (s *Server) handleNodeOperationLogs(ctx context.Context, w http.ResponseWriter, clusterName, opID string) {
	fcj, err := s.findFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, rec := range fcj.Status.NodeOperationHistory {
		if rec.ID != opID {
			continue
		}
		writeJSON(w, http.StatusOK, nodeOperationLogsResponse{
			ID:      rec.ID,
			JobName: rec.JobName,
			Logs:    rec.LogTail,
			Source:  "history",
		})
		return
	}
	if op := fcj.Status.NodeOperation; op != nil && op.ID == opID {
		if op.JobName != "" {
			logs, err := s.ansibleJobPodLogs(ctx, fcj.Namespace, op.JobName)
			if err == nil {
				writeJSON(w, http.StatusOK, nodeOperationLogsResponse{
					ID:      op.ID,
					JobName: op.JobName,
					Logs:    ansiblelogs.Tail(logs, 65536),
					Source:  "job",
				})
				return
			}
		}
		if op.Phase == fleetv1alpha1.NodeOpPhaseSucceeded || op.Phase == fleetv1alpha1.NodeOpPhaseFailed {
			writeJSON(w, http.StatusOK, nodeOperationLogsResponse{
				ID:     op.ID,
				Logs:   "",
				Source: "current",
			})
			return
		}
	}
	http.Error(w, "operation not found", http.StatusNotFound)
}

func historyItemFromRecord(rec fleetv1alpha1.ClusterNodeOperationRecord) nodeOperationHistoryItem {
	item := nodeOperationHistoryItem{
		ID:        rec.ID,
		Type:      rec.Type,
		Node:      rec.Node,
		Phase:     rec.Phase,
		JobName:   rec.JobName,
		Message:   rec.Message,
		Force:     rec.Force,
		ActorKind: rec.ActorKind,
		ActorName: rec.ActorName,
		HasLogs:   rec.LogTail != "",
	}
	if rec.StartedAt != nil {
		item.StartedAt = rec.StartedAt.Format(time.RFC3339)
	}
	if rec.FinishedAt != nil {
		item.FinishedAt = rec.FinishedAt.Format(time.RFC3339)
	}
	return item
}
