package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// File holds CLI connection and auth settings.
type File struct {
	APIURL  string `json:"apiUrl,omitempty"`
	AuthURL string `json:"authUrl,omitempty"`
	Token   string `json:"token,omitempty"`
	DevMode bool   `json:"devMode,omitempty"`
}

func Dir() (string, error) {
	if v := strings.TrimSpace(os.Getenv("UMKIM_CONFIG_DIR")); v != "" {
		return v, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "umkim"), nil
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func Load() (File, error) {
	path, err := Path()
	if err != nil {
		return File{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return File{}, nil
		}
		return File{}, err
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		return File{}, err
	}
	return f, nil
}

func Save(f File) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path, err := Path()
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

// Resolve merges file, env, and flags into effective settings.
func Resolve(f File, flagAPI, flagAuth, flagToken string) File {
	out := f
	if v := strings.TrimSpace(os.Getenv("UMKIM_API_URL")); v != "" {
		out.APIURL = v
	}
	if v := strings.TrimSpace(os.Getenv("UMKIM_AUTH_URL")); v != "" {
		out.AuthURL = v
	}
	if v := strings.TrimSpace(os.Getenv("UMKIM_TOKEN")); v != "" {
		out.Token = v
	}
	if strings.EqualFold(os.Getenv("UMKIM_DEV"), "1") || strings.EqualFold(os.Getenv("UMKIM_DEV"), "true") {
		out.DevMode = true
	}
	if flagAPI != "" {
		out.APIURL = flagAPI
	}
	if flagAuth != "" {
		out.AuthURL = flagAuth
	}
	if flagToken != "" {
		out.Token = flagToken
	}
	if out.APIURL == "" && out.DevMode {
		out.APIURL = "http://127.0.0.1:18080/api/v1alpha1"
	}
	if out.AuthURL == "" && out.APIURL != "" {
		out.AuthURL = deriveAuthURL(out.APIURL)
	}
	return out
}

func deriveAuthURL(apiURL string) string {
	apiURL = strings.TrimRight(apiURL, "/")
	if strings.Contains(apiURL, "/fleet/api/") {
		return strings.Replace(apiURL, "/fleet/api/", "/fleet/auth/", 1)
	}
	// Direct fleet-site: auth may be on console host; caller should set UMKIM_AUTH_URL.
	return ""
}
