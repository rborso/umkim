package identity

import (
	"context"
	"crypto/subtle"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type HtpasswdProvider struct {
	path string
}

func NewHtpasswdProvider(path string) *HtpasswdProvider {
	return &HtpasswdProvider{path: path}
}

func (p *HtpasswdProvider) Name() string { return "htpasswd" }

func (p *HtpasswdProvider) Authenticate(_ context.Context, username, password string) (*Principal, error) {
	if p.path == "" {
		return nil, fmt.Errorf("htpasswd not configured")
	}
	data, err := os.ReadFile(p.path)
	if err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if parts[0] != username {
			continue
		}
		if verifyHtpasswdHash(parts[1], password) {
			return &Principal{
				Username:    username,
				ExternalID:  username,
				DisplayName: username,
				AuthMethod:  "htpasswd",
			}, nil
		}
		return nil, fmt.Errorf("invalid credentials")
	}
	return nil, fmt.Errorf("invalid credentials")
}

func verifyHtpasswdHash(hash, password string) bool {
	hash = strings.TrimSpace(hash)
	if strings.HasPrefix(hash, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
	}
	// Apache apr1 / crypt not supported in v1; use bcrypt via htpasswd -B
	return subtle.ConstantTimeCompare([]byte(hash), []byte(password)) == 1
}
