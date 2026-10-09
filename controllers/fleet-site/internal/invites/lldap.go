package invites

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LLDAP is a minimal client for the LLDAP HTTP/GraphQL API.
type LLDAP struct {
	BaseURL string
	HTTP    *http.Client
}

// NewLLDAP builds a client for a base URL such as http://lldap:17170.
func NewLLDAP(baseURL string) *LLDAP {
	return &LLDAP{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Login exchanges credentials for a bearer token.
func (l *LLDAP) Login(ctx context.Context, username, password string) (string, error) {
	payload, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.BaseURL+"/auth/simple/login", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lldap login: status %d", resp.StatusCode)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", fmt.Errorf("lldap login: empty token")
	}
	return out.Token, nil
}

func (l *LLDAP) gql(ctx context.Context, token, query string, vars map[string]any, out any) error {
	payload := map[string]any{"query": query}
	if vars != nil {
		payload["variables"] = vars
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.BaseURL+"/api/graphql", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := l.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lldap graphql: status %d", resp.StatusCode)
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return err
	}
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("lldap: %s", envelope.Errors[0].Message)
	}
	if out != nil && len(envelope.Data) > 0 {
		return json.Unmarshal(envelope.Data, out)
	}
	return nil
}

// UserExists reports whether a user id is present in the directory.
func (l *LLDAP) UserExists(ctx context.Context, token, userID string) (bool, error) {
	var out struct {
		Users []struct {
			ID string `json:"id"`
		} `json:"users"`
	}
	if err := l.gql(ctx, token, "{users{id}}", nil, &out); err != nil {
		return false, err
	}
	for _, u := range out.Users {
		if u.ID == userID {
			return true, nil
		}
	}
	return false, nil
}

// CreateUser creates a directory user (idempotent for existing ids).
func (l *LLDAP) CreateUser(ctx context.Context, token, id, displayName, email string) error {
	if displayName == "" {
		displayName = id
	}
	if email == "" {
		email = id + "@fleet.local"
	}
	query := "mutation($u:CreateUserInput!){createUser(user:$u){id}}"
	vars := map[string]any{"u": map[string]string{"id": id, "displayName": displayName, "email": email}}
	return l.gql(ctx, token, query, vars, nil)
}

// GroupID resolves a group's numeric id by display name.
func (l *LLDAP) GroupID(ctx context.Context, token, name string) (int, bool, error) {
	var out struct {
		Groups []struct {
			ID          int    `json:"id"`
			DisplayName string `json:"displayName"`
		} `json:"groups"`
	}
	if err := l.gql(ctx, token, "{groups{id displayName}}", nil, &out); err != nil {
		return 0, false, err
	}
	for _, g := range out.Groups {
		if g.DisplayName == name {
			return g.ID, true, nil
		}
	}
	return 0, false, nil
}

// AddUserToGroup adds a user to a group (idempotent).
func (l *LLDAP) AddUserToGroup(ctx context.Context, token, userID string, groupID int) error {
	query := "mutation($u:String!,$g:Int!){addUserToGroup(userId:$u,groupId:$g){__typename}}"
	return l.gql(ctx, token, query, map[string]any{"u": userID, "g": groupID}, nil)
}

// RemoveUserFromGroup removes a user from a group (idempotent).
func (l *LLDAP) RemoveUserFromGroup(ctx context.Context, token, userID string, groupID int) error {
	query := "mutation($u:String!,$g:Int!){removeUserFromGroup(userId:$u,groupId:$g){__typename}}"
	return l.gql(ctx, token, query, map[string]any{"u": userID, "g": groupID}, nil)
}

// DirectoryUser is a directory user summary.
type DirectoryUser struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}

// DirectoryGroup is a directory group with its members.
type DirectoryGroup struct {
	ID          int             `json:"id"`
	DisplayName string          `json:"displayName"`
	Users       []DirectoryUser `json:"users"`
}

// ListGroups returns all directory groups with their members.
func (l *LLDAP) ListGroups(ctx context.Context, token string) ([]DirectoryGroup, error) {
	var out struct {
		Groups []DirectoryGroup `json:"groups"`
	}
	query := "{groups{id displayName users{id displayName email}}}"
	if err := l.gql(ctx, token, query, nil, &out); err != nil {
		return nil, err
	}
	return out.Groups, nil
}

// ListUsers returns all directory users.
func (l *LLDAP) ListUsers(ctx context.Context, token string) ([]DirectoryUser, error) {
	var out struct {
		Users []DirectoryUser `json:"users"`
	}
	if err := l.gql(ctx, token, "{users{id displayName email}}", nil, &out); err != nil {
		return nil, err
	}
	return out.Users, nil
}

// CreateGroup creates a directory group.
func (l *LLDAP) CreateGroup(ctx context.Context, token, name string) error {
	query := "mutation($n:String!){createGroup(name:$n){id}}"
	return l.gql(ctx, token, query, map[string]any{"n": name}, nil)
}

// DeleteGroup deletes a directory group by id.
func (l *LLDAP) DeleteGroup(ctx context.Context, token string, groupID int) error {
	query := "mutation($g:Int!){deleteGroup(groupId:$g){__typename}}"
	return l.gql(ctx, token, query, map[string]any{"g": groupID}, nil)
}
