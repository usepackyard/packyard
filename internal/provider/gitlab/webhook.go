package gitlab

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/usepackyard/packyard/internal/provider"
)

type webhookPayload struct {
	ObjectKind string `json:"object_kind"`
	Tag        string `json:"tag"`
	Project    struct {
		PathWithNamespace string `json:"path_with_namespace"`
	} `json:"project"`
	ObjectAttributes struct {
		Action string `json:"action"`
	} `json:"object_attributes"`
}

func (g *GitLab) ParseWebhook(body []byte) (*provider.WebhookEvent, error) {
	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse webhook payload: %w", err)
	}
	if payload.ObjectKind != "release" {
		return nil, fmt.Errorf("unsupported webhook object kind: %s", payload.ObjectKind)
	}
	owner, repo := splitPath(payload.Project.PathWithNamespace)
	action := payload.ObjectAttributes.Action
	if action == "create" || action == "update" {
		action = "published"
	}
	return &provider.WebhookEvent{
		RepoOwner: owner,
		RepoName:  repo,
		TagName:   payload.Tag,
		Action:    action,
	}, nil
}

func (g *GitLab) ValidateWebhook(r *http.Request, secret string, body []byte) error {
	got := r.Header.Get("X-Gitlab-Token")
	if got == "" {
		return fmt.Errorf("missing X-Gitlab-Token header")
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
		return fmt.Errorf("token mismatch")
	}
	return nil
}

func splitPath(path string) (string, string) {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i], path[i+1:]
		}
	}
	return "", path
}
