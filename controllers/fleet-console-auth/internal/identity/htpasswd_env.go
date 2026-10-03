package identity

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// HtpasswdEnvProvider authenticates against a single user:hash line from env or file write.
type HtpasswdEnvProvider struct {
	line string
	path string
}

func NewHtpasswdEnvProvider(line, writePath string) *HtpasswdEnvProvider {
	line = strings.TrimSpace(line)
	if line != "" && writePath != "" {
		_ = os.WriteFile(writePath, []byte(line+"\n"), 0o600)
	}
	return &HtpasswdEnvProvider{line: line, path: writePath}
}

func (p *HtpasswdEnvProvider) Name() string { return "htpasswd-env" }

func (p *HtpasswdEnvProvider) Authenticate(_ context.Context, username, password string) (*Principal, error) {
	if p.line == "" {
		return nil, fmt.Errorf("htpasswd env not configured")
	}
	parts := strings.SplitN(p.line, ":", 2)
	if len(parts) != 2 || parts[0] != username {
		return nil, fmt.Errorf("invalid credentials")
	}
	if verifyHtpasswdHash(parts[1], password) {
		return &Principal{Username: username, ExternalID: username, DisplayName: username, AuthMethod: "htpasswd"}, nil
	}
	return nil, fmt.Errorf("invalid credentials")
}
