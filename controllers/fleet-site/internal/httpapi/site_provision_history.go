package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ansiblelogs"
	"k8s.io/apimachinery/pkg/types"
)

type siteProvisionHistoryItem struct {
	ID        string `json:"id"`
	Action    string `json:"action,omitempty"`
	Subject   string `json:"subject,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Phase     string `json:"phase,omitempty"`
	Message   string `json:"message,omitempty"`
	Confirm   bool   `json:"confirm,omitempty"`
	JobName   string `json:"jobName,omitempty"`
	ActorKind string `json:"actorKind,omitempty"`
	ActorName string `json:"actorName,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
	EndedAt   string `json:"endedAt,omitempty"`
	HasLogs   bool   `json:"hasLogs,omitempty"`
}

type siteProvisionHistoryList struct {
	Items []siteProvisionHistoryItem `json:"items"`
}

type siteProvisionLogsResponse struct {
	ID      string `json:"id"`
	JobName string `json:"jobName,omitempty"`
	Logs    string `json:"logs"`
	Source  string `json:"source,omitempty"`
}

func (s *Server) handleSiteProvisionActionLogs(ctx context.Context, w http.ResponseWriter, siteName, id string) {
	var fs fleetv1alpha1.FleetSite
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: siteName}, &fs); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.refreshProvisionJobIfNeeded(ctx, &fs)
	rec, ok := findProvisionAction(&fs, id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if rec.LogTail != "" {
		writeJSON(w, http.StatusOK, siteProvisionLogsResponse{
			ID:      rec.ID,
			JobName: rec.JobName,
			Logs:    rec.LogTail,
			Source:  "history",
		})
		return
	}
	if rec.JobName != "" {
		logs, err := s.ansibleJobPodLogs(ctx, s.Cfg.Namespace, rec.JobName)
		if err == nil {
			writeJSON(w, http.StatusOK, siteProvisionLogsResponse{
				ID:      rec.ID,
				JobName: rec.JobName,
				Logs:    ansiblelogs.Tail(logs, 65536),
				Source:  "job",
			})
			return
		}
	}
	writeJSON(w, http.StatusOK, siteProvisionLogsResponse{
		ID:     rec.ID,
		Logs:   "",
		Source: "current",
	})
}

func provisionHistoryItems(fs *fleetv1alpha1.FleetSite) []siteProvisionHistoryItem {
	if fs.Status.Provision == nil {
		return nil
	}
	records := make([]fleetv1alpha1.SiteProvisionActionRecord, 0, len(fs.Status.Provision.History)+1)
	if fs.Status.Provision.Current != nil {
		records = append(records, *fs.Status.Provision.Current)
	}
	records = append(records, fs.Status.Provision.History...)
	items := make([]siteProvisionHistoryItem, 0, len(records))
	for _, rec := range records {
		items = append(items, provisionHistoryItemFromRecord(rec))
	}
	return items
}

func provisionHistoryItemFromRecord(rec fleetv1alpha1.SiteProvisionActionRecord) siteProvisionHistoryItem {
	item := siteProvisionHistoryItem{
		ID:        rec.ID,
		Action:    rec.Action,
		Subject:   rec.Subject,
		Provider:  rec.Provider,
		Phase:     rec.Phase,
		Message:   rec.Message,
		Confirm:   rec.Confirm,
		JobName:   rec.JobName,
		ActorKind: rec.ActorKind,
		ActorName: rec.ActorName,
		HasLogs:   rec.LogTail != "" || rec.JobName != "",
	}
	if rec.StartedAt != nil {
		item.StartedAt = rec.StartedAt.Format(time.RFC3339)
	}
	if rec.EndedAt != nil {
		item.EndedAt = rec.EndedAt.Format(time.RFC3339)
	}
	return item
}

func provisionActionSubpath(rest string) (id string, wantLogs bool, ok bool) {
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return "", false, false
	}
	if strings.HasSuffix(rest, "/logs") {
		id = strings.TrimSuffix(rest, "/logs")
		id = strings.Trim(id, "/")
		return id, true, id != ""
	}
	if strings.Contains(rest, "/") {
		return "", false, false
	}
	return rest, false, true
}
