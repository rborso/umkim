package httpapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/invites"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// inviteRedeemJobName / inviteRedeemSecretName live in the ingress-auth
	// namespace of the cluster whose own LLDAP the invite targets.
	inviteRedeemJobName      = "lldap-invite-redeem"
	inviteRedeemSecretName   = "lldap-invite-redeem"
	inviteRedeemNamespace    = "fleet-ingress-auth"
	inviteRedeemAdminSecret  = "lldap-credentials"
	inviteRedeemAdminKey     = "LLDAP_LDAP_USER_PASS"
	inviteRedeemLLDAPService = "lldap"
	inviteRedeemWaitTimeout  = 3 * time.Minute
)

// clusterUsesOwnDirectory reports whether a cluster runs its own LLDAP. Clusters
// with `ingressAuth.directory: hub` run none: they bind the shared hub directory.
func clusterUsesOwnDirectory(fcj *fleetv1alpha1.FleetClusterJoin) bool {
	if fcj == nil || !fcj.Spec.IngressAuth.Enabled {
		return false
	}
	return fcj.Spec.IngressAuth.Directory != "hub"
}

// provisionInvitedUserOnSpoke creates the invited directory user in the
// cluster's own LLDAP.
//
// The hub cannot reach a spoke LLDAP HTTP API — it is a ClusterIP Service — so
// the work runs as a Job on the spoke next to LLDAP, using the image the LLDAP
// Deployment already runs and the admin password from the chart's
// lldap-credentials Secret. That mirrors the chart's own lldap-bind-account Job
// and needs no new exposure of the directory.
func (s *Server) provisionInvitedUserOnSpoke(ctx context.Context, inv invites.Invite, fcj *fleetv1alpha1.FleetClusterJoin, username, password, displayName, email string) error {
	clusterName := fcj.Spec.EffectiveClusterName(fcj.Name)
	cs, err := s.directSpokeClientset(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("spoke clientset for %s: %w", clusterName, err)
	}
	ns := inviteRedeemNamespace
	image, err := s.spokeLLDAPImage(ctx, cs, ns)
	if err != nil {
		return err
	}

	secret := buildInviteRedeemSecret(username, password, displayName, email, inv.Groups)
	if _, err := cs.CoreV1().Secrets(ns).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		// A leftover secret from an interrupted attempt would collide; replace it.
		if _, derr := cs.CoreV1().Secrets(ns).Update(ctx, secret, metav1.UpdateOptions{}); derr != nil {
			return fmt.Errorf("create invite redeem secret: %w", err)
		}
	}
	job := buildInviteRedeemJob(image)
	background := metav1.DeletePropagationBackground
	defer func() {
		// Best effort: the Job carries a TTL, but the Secret holds a password.
		_ = cs.BatchV1().Jobs(ns).Delete(context.Background(), job.Name, metav1.DeleteOptions{PropagationPolicy: &background})
		_ = cs.CoreV1().Secrets(ns).Delete(context.Background(), secret.Name, metav1.DeleteOptions{})
	}()
	if err := cs.BatchV1().Jobs(ns).Delete(ctx, job.Name, metav1.DeleteOptions{PropagationPolicy: &background}); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("clear previous invite redeem job: %w", err)
		}
	}
	if _, err := cs.BatchV1().Jobs(ns).Create(ctx, job, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create invite redeem job: %w", err)
	}
	return s.waitInviteRedeemJob(ctx, cs, ns, job.Name)
}

// waitInviteRedeemJob waits for the Job to finish and surfaces its log, which is
// where the createUser / group / password steps report.
func (s *Server) waitInviteRedeemJob(ctx context.Context, cs kubernetes.Interface, ns, name string) error {
	deadline := time.Now().Add(inviteRedeemWaitTimeout)
	for {
		job, err := cs.BatchV1().Jobs(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("read invite redeem job: %w", err)
		}
		for _, c := range job.Status.Conditions {
			if c.Status != corev1.ConditionTrue {
				continue
			}
			switch c.Type {
			case batchv1.JobComplete:
				return nil
			case batchv1.JobFailed:
				return fmt.Errorf("invite redeem job failed: %s", s.inviteRedeemJobLog(ctx, cs, ns, name))
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("invite redeem job did not finish in %s: %s",
				inviteRedeemWaitTimeout, s.inviteRedeemJobLog(ctx, cs, ns, name))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func (s *Server) inviteRedeemJobLog(ctx context.Context, cs kubernetes.Interface, ns, jobName string) string {
	pods, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + jobName})
	if err != nil || len(pods.Items) == 0 {
		return "no pod log"
	}
	raw, err := cs.CoreV1().Pods(ns).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{}).DoRaw(ctx)
	if err != nil {
		return "pod log unavailable"
	}
	out := strings.TrimSpace(string(raw))
	if len(out) > 600 {
		out = out[len(out)-600:]
	}
	return out
}

// spokeLLDAPImage reuses the image the spoke's own LLDAP Deployment runs, so the
// redeem Job needs no image of its own and no extra pull.
func (s *Server) spokeLLDAPImage(ctx context.Context, cs kubernetes.Interface, ns string) (string, error) {
	dep, err := cs.AppsV1().Deployments(ns).Get(ctx, inviteRedeemLLDAPService, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("read spoke lldap deployment: %w", err)
	}
	for _, c := range dep.Spec.Template.Spec.Containers {
		if strings.TrimSpace(c.Image) == "" {
			continue
		}
		if c.Name == inviteRedeemLLDAPService {
			return c.Image, nil
		}
	}
	if len(dep.Spec.Template.Spec.Containers) > 0 {
		if img := strings.TrimSpace(dep.Spec.Template.Spec.Containers[0].Image); img != "" {
			return img, nil
		}
	}
	return "", fmt.Errorf("spoke lldap deployment has no container image")
}

func buildInviteRedeemSecret(username, password, displayName, email string, groups []string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      inviteRedeemSecretName,
			Namespace: inviteRedeemNamespace,
			Labels:    map[string]string{"fleet.umkim.com/component": inviteRedeemJobName},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"INVITE_USERNAME":     username,
			"INVITE_PASSWORD":     password,
			"INVITE_DISPLAY_NAME": displayName,
			"INVITE_EMAIL":        email,
			"INVITE_GROUPS":       strings.Join(groups, " "),
		},
	}
}

// inviteRedeemScript creates the user, adds it to every invited group and sets
// its password. Values arrive as environment variables so nothing user-supplied
// is ever interpolated into the shell.
const inviteRedeemScript = `set -euo pipefail
url="${LLDAP_URL}"
for i in $(seq 1 40); do
  curl -fsS "${url}/health" >/dev/null 2>&1 && break
  sleep 3
done
token=$(curl -fsS -X POST "${url}/auth/simple/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"${LLDAP_ADMIN_PASSWORD}\"}" \
  | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
if [ -z "${token}" ]; then echo "LLDAP admin login failed"; exit 1; fi
gql() { curl -fsS -X POST "${url}/api/graphql" \
  -H "Authorization: Bearer ${token}" -H 'Content-Type: application/json' --data "$1"; }

created=$(gql "{\"query\":\"mutation(\$u:CreateUserInput!){createUser(user:\$u){id}}\",\"variables\":{\"u\":{\"id\":\"${INVITE_USERNAME}\",\"displayName\":\"${INVITE_DISPLAY_NAME}\",\"email\":\"${INVITE_EMAIL}\"}}}")
case "${created}" in
  *'"errors"'*) echo "createUser failed: ${created}"; exit 1;;
esac
echo "==> created directory user ${INVITE_USERNAME}"

for g in ${INVITE_GROUPS}; do
  groups=$(gql '{"query":"{ groups { id displayName } }"}' 2>/dev/null || true)
  gid=$(printf '%s' "${groups}" | tr '{' '\n' | grep -F "\"displayName\":\"${g}\"" | grep -o '"id":[0-9]*' | head -1 | grep -o '[0-9][0-9]*' || true)
  if [ -z "${gid}" ]; then echo "group ${g} not found"; exit 1; fi
  gql "{\"query\":\"mutation(\$u:String!,\$g:Int!){addUserToGroup(userId:\$u,groupId:\$g){__typename}}\",\"variables\":{\"u\":\"${INVITE_USERNAME}\",\"g\":${gid}}}" >/dev/null
  echo "==> ${INVITE_USERNAME} added to ${g} (id ${gid})"
done

/app/lldap_set_password \
  --base-url "${url}" \
  --admin-username admin \
  --admin-password "${LLDAP_ADMIN_PASSWORD}" \
  --username "${INVITE_USERNAME}" \
  --password "${INVITE_PASSWORD}"
echo "==> invite redeemed for ${INVITE_USERNAME}"`

func buildInviteRedeemJob(image string) *batchv1.Job {
	ttl := int32(600)
	backoff := int32(2)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      inviteRedeemJobName,
			Namespace: inviteRedeemNamespace,
			Labels:    map[string]string{"fleet.umkim.com/component": inviteRedeemJobName},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"fleet.umkim.com/component": inviteRedeemJobName},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:    "redeem",
						Image:   image,
						Command: []string{"/bin/bash", "-c"},
						Args:    []string{inviteRedeemScript},
						Env: []corev1.EnvVar{
							{Name: "LLDAP_URL", Value: fmt.Sprintf("http://%s:%d", inviteRedeemLLDAPService, 17170)},
							{Name: "LLDAP_ADMIN_PASSWORD", ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: inviteRedeemAdminSecret},
									Key:                  inviteRedeemAdminKey,
								},
							}},
							{Name: "INVITE_USERNAME", ValueFrom: secretEnvRef("INVITE_USERNAME")},
							{Name: "INVITE_PASSWORD", ValueFrom: secretEnvRef("INVITE_PASSWORD")},
							{Name: "INVITE_DISPLAY_NAME", ValueFrom: secretEnvRef("INVITE_DISPLAY_NAME")},
							{Name: "INVITE_EMAIL", ValueFrom: secretEnvRef("INVITE_EMAIL")},
							{Name: "INVITE_GROUPS", ValueFrom: secretEnvRef("INVITE_GROUPS")},
						},
					}},
				},
			},
		},
	}
}

func secretEnvRef(key string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: inviteRedeemSecretName},
			Key:                  key,
		},
	}
}
