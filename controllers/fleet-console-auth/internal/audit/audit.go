package audit

import (
	"context"
	"fmt"
	"time"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Recorder struct {
	Client    client.Client
	Namespace string
}

func (r *Recorder) Record(ctx context.Context, actor authv1.SubjectRef, action, result string, resource, scope *authv1.ScopeRef, http *authv1.AuditHTTPInfo, message string) error {
	now := metav1.NewTime(time.Now().UTC())
	name := fmt.Sprintf("%d-%s", now.UnixNano(), sanitize(actor.Name))
	ev := authv1.FleetAuditEvent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name[:min(63, len(name))],
			Namespace: r.Namespace,
			Labels: map[string]string{
				"fleet.umkim.com/audit": "true",
			},
		},
		Spec: authv1.FleetAuditEventSpec{
			Timestamp: now,
			Actor:     actor,
			Action:    action,
			Resource:  resource,
			Scope:     scope,
			Result:    result,
			HTTP:      http,
			Message:   message,
		},
	}
	return r.Client.Create(ctx, &ev)
}

func sanitize(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' {
			out = append(out, c)
		} else {
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "event"
	}
	return string(out)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
