package httpapi

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"golang.org/x/crypto/ssh"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const fleetAnsibleSSHSecretName = "fleet-site-ansible-ssh"

// hubFleetSSHPublicKey returns the authorized_keys line for the shared fleet Ansible key.
func (s *Server) hubFleetSSHPublicKey(ctx context.Context) string {
	var sec corev1.Secret
	if err := s.Client.Get(ctx, client.ObjectKey{Namespace: s.Cfg.Namespace, Name: fleetAnsibleSSHSecretName}, &sec); err != nil {
		return ""
	}
	for _, key := range []string{"id_rsa", "id_ed25519"} {
		if pub := sshPublicKeyLine(sec.Data[key]); pub != "" {
			return pub
		}
	}
	return ""
}

func sshPublicKeyLine(pem []byte) string {
	if len(pem) == 0 {
		return ""
	}
	signer, err := ssh.ParsePrivateKey(pem)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
}
