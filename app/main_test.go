package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"text/template"
)

func TestDefaultNode(t *testing.T) {
	cases := map[string]string{
		"mainnet": "grpc.trongrid.io:50051",
		"nile":    "grpc.nile.trongrid.io:50051",
		"shasta":  "grpc.shasta.trongrid.io:50051",
		"unknown": "grpc.trongrid.io:50051",
	}
	for network, want := range cases {
		t.Run(network, func(t *testing.T) {
			if got := defaultNode(network); got != want {
				t.Fatalf("defaultNode(%q) = %q, want %q", network, got, want)
			}
		})
	}
}

func TestHealthEndpoint(t *testing.T) {
	t.Setenv("TRON_NETWORK", "nile")
	t.Setenv("TRON_NODE", "grpc.nile.trongrid.io:50051")
	s := &server{}
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	s.health(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q, want JSON", got)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if body["status"] != "ok" || body["network"] != "nile" {
		t.Fatalf("unexpected health response: %#v", body)
	}
}

func TestAccountRejectsMissingOrInvalidAddress(t *testing.T) {
	s := &server{}
	for _, target := range []string{"/api/account", "/api/account?address=not-an-address", "/api/account?address=" + strings.Repeat("T", 65)} {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			rec := httptest.NewRecorder()
			s.account(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
				t.Fatalf("expected JSON content type, got %q", rec.Header().Get("Content-Type"))
			}
		})
	}
}

func TestHomeRejectsUnknownPath(t *testing.T) {
	s := &server{tpl: template.Must(template.New("index").Parse(page))}
	req := httptest.NewRequest(http.MethodGet, "/unexpected", nil)
	rec := httptest.NewRecorder()
	s.home(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestSecurityHeaders(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := securityHeaders(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	for name, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	} {
		if got := rec.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}
