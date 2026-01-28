package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteJSON_SetsContentType(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusCreated, map[string]string{"foo": "bar"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not valid JSON: %v\n%s", err, rec.Body.String())
	}
	if body["foo"] != "bar" {
		t.Errorf("body = %v", body)
	}
}

func TestWriteError_Shape(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "missing_field", "missing field")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "missing field" {
		t.Errorf("error field = %q, want %q", body["error"], "missing field")
	}
}

func TestDecodeJSON_HappyPath(t *testing.T) {
	body := `{"name":"hello"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	rec := httptest.NewRecorder()

	var v struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(rec, req, &v); err != nil {
		t.Fatalf("decodeJSON: %v", err)
	}
	if v.Name != "hello" {
		t.Errorf("Name = %q", v.Name)
	}
}

func TestDecodeJSON_RejectsOversizedBody(t *testing.T) {
	// One byte over the limit should fail; exactly at the limit should pass
	// (modulo the JSON shape costing a few bytes — easier to test with a
	// payload deliberately larger than maxJSONBodySize).
	huge := bytes.Repeat([]byte("a"), maxJSONBodySize+10)
	body := []byte(`{"name":"`)
	body = append(body, huge...)
	body = append(body, []byte(`"}`)...)

	req := httptest.NewRequest("POST", "/", io.NopCloser(bytes.NewReader(body)))
	rec := httptest.NewRecorder()

	var v struct {
		Name string `json:"name"`
	}
	err := decodeJSON(rec, req, &v)
	if err == nil {
		t.Fatal("expected error decoding oversized body")
	}
	if !strings.Contains(err.Error(), "request body too large") &&
		!strings.Contains(err.Error(), "http: request body too large") {
		// MaxBytesReader returns "http: request body too large".
		t.Errorf("unexpected error message: %v", err)
	}
}
