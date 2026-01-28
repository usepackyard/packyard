package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"testing"
)

const samplePayload = `{
  "action": "published",
  "release": {"tag_name": "v1.2.3", "draft": false},
  "repository": {"owner": {"login": "octo"}, "name": "hello"}
}`

func sigFor(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestParseWebhook(t *testing.T) {
	g := &GitHub{}
	event, err := g.ParseWebhook([]byte(samplePayload))
	if err != nil {
		t.Fatalf("ParseWebhook: %v", err)
	}
	if event.RepoOwner != "octo" || event.RepoName != "hello" {
		t.Errorf("repo = %s/%s", event.RepoOwner, event.RepoName)
	}
	if event.TagName != "v1.2.3" {
		t.Errorf("TagName = %q", event.TagName)
	}
	if event.Action != "published" || event.IsDraft {
		t.Errorf("Action=%s IsDraft=%v", event.Action, event.IsDraft)
	}
}

func TestParseWebhook_InvalidJSON(t *testing.T) {
	g := &GitHub{}
	_, err := g.ParseWebhook([]byte(`{ not json`))
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestValidateWebhook_HappyPath(t *testing.T) {
	g := &GitHub{}
	secret := "shhh"
	body := samplePayload

	req := httptest.NewRequest("POST", "/hooks/github", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sigFor(secret, body))

	if err := g.ValidateWebhook(req, secret, []byte(body)); err != nil {
		t.Fatalf("ValidateWebhook: %v", err)
	}
}

func TestValidateWebhook_BadSignature(t *testing.T) {
	g := &GitHub{}
	req := httptest.NewRequest("POST", "/x", nil)
	req.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")

	err := g.ValidateWebhook(req, "shhh", []byte(samplePayload))
	if err == nil {
		t.Fatal("expected signature mismatch")
	}
}

func TestValidateWebhook_WrongSecret(t *testing.T) {
	g := &GitHub{}
	body := samplePayload
	// Sign with secret A, verify with secret B.
	req := httptest.NewRequest("POST", "/x", nil)
	req.Header.Set("X-Hub-Signature-256", sigFor("secret-A", body))

	err := g.ValidateWebhook(req, "secret-B", []byte(body))
	if err == nil {
		t.Fatal("expected mismatch for wrong secret")
	}
}

func TestValidateWebhook_MissingHeader(t *testing.T) {
	g := &GitHub{}
	req := httptest.NewRequest("POST", "/x", nil)

	err := g.ValidateWebhook(req, "shhh", []byte(samplePayload))
	if err == nil {
		t.Fatal("expected error for missing header")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error should mention missing header: %v", err)
	}
}

func TestValidateWebhook_InvalidPrefix(t *testing.T) {
	g := &GitHub{}
	req := httptest.NewRequest("POST", "/x", nil)
	// Missing "sha256=" prefix.
	req.Header.Set("X-Hub-Signature-256", "deadbeef")

	err := g.ValidateWebhook(req, "shhh", []byte(samplePayload))
	if err == nil {
		t.Fatal("expected error for missing sha256= prefix")
	}
}

