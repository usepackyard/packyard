package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/usepackyard/packyard/internal/provider"
)

type webhookPayload struct {
	Action  string `json:"action"`
	Release struct {
		TagName string `json:"tag_name"`
		Draft   bool   `json:"draft"`
	} `json:"release"`
	Repository struct {
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name string `json:"name"`
	} `json:"repository"`
}

func (g *GitHub) ParseWebhook(body []byte) (*provider.WebhookEvent, error) {
	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse webhook payload: %w", err)
	}
	return &provider.WebhookEvent{
		RepoOwner: payload.Repository.Owner.Login,
		RepoName:  payload.Repository.Name,
		TagName:   payload.Release.TagName,
		Action:    payload.Action,
		IsDraft:   payload.Release.Draft,
	}, nil
}

func (g *GitHub) ValidateWebhook(r *http.Request, secret string, body []byte) error {
	sig := r.Header.Get("X-Hub-Signature-256")
	if sig == "" {
		return fmt.Errorf("missing X-Hub-Signature-256 header")
	}
	if !strings.HasPrefix(sig, "sha256=") {
		return fmt.Errorf("invalid signature format")
	}
	sigHex := strings.TrimPrefix(sig, "sha256=")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sigHex), []byte(expected)) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}
