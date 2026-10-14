package main

import (
	"encoding/json"
	"encoding/hex"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"net/http"
	"net/http/httptest"
	"strings"
	"strconv"
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

func TestVerifyTronMessageSignature(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	if err != nil { t.Fatal(err) }
	walletAddress := address.PubkeyToAddress(privateKey.PublicKey).String()
	message := "GoTRON Console Authentication\nAddress: " + walletAddress + "\nNonce: one-time"
	digest := crypto.Keccak256Hash([]byte("\x19TRON Signed Message:\n" + strconv.Itoa(len(message)) + message))
	sig, err := crypto.Sign(digest.Bytes(), privateKey)
	if err != nil { t.Fatal(err) }
	if !verifyTronMessageSignature(message, "0x"+hex.EncodeToString(sig), walletAddress) {
		t.Fatal("expected generated signature to verify")
	}
	if verifyTronMessageSignature(message+" altered", hex.EncodeToString(sig), walletAddress) {
		t.Fatal("altered message must not verify")
	}
	otherKey, err := crypto.GenerateKey()
	if err != nil { t.Fatal(err) }
	otherAddress := address.PubkeyToAddress(otherKey.PublicKey).String()
	if verifyTronMessageSignature(message, hex.EncodeToString(sig), otherAddress) {
		t.Fatal("signature must not verify for a different address")
	}
}

func TestAuthChallengeVerifyAndReplayProtection(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	if err != nil { t.Fatal(err) }
	walletAddress := address.PubkeyToAddress(privateKey.PublicKey).String()
	s := &server{challenges: make(map[string]authChallenge), sessions: make(map[string]authSession)}
	t.Setenv("TRON_NETWORK", "nile")
	t.Setenv("AUTH_COOKIE_SECURE", "false")

	challengeReq := httptest.NewRequest(http.MethodPost, "/api/auth/challenge", strings.NewReader(`{"address":"`+walletAddress+`"}`))
	challengeReq.Header.Set("Content-Type", "application/json")
	challengeRec := httptest.NewRecorder()
	s.authChallenge(challengeRec, challengeReq)
	if challengeRec.Code != http.StatusOK { t.Fatalf("challenge status=%d body=%s", challengeRec.Code, challengeRec.Body.String()) }
	var challenge struct { Nonce string `json:"nonce"`; Message string `json:"message"` }
	if err := json.Unmarshal(challengeRec.Body.Bytes(), &challenge); err != nil { t.Fatal(err) }
	if challenge.Nonce == "" || challenge.Message == "" { t.Fatal("challenge response missing nonce or message") }

	digest := crypto.Keccak256Hash([]byte("\x19TRON Signed Message:\n" + strconv.Itoa(len(challenge.Message)) + challenge.Message))
	sig, err := crypto.Sign(digest.Bytes(), privateKey)
	if err != nil { t.Fatal(err) }
	payload := `{"address":"`+walletAddress+`","nonce":"`+challenge.Nonce+`","signature":"`+hex.EncodeToString(sig)+`"}`
	verifyReq := httptest.NewRequest(http.MethodPost, "/api/auth/verify", strings.NewReader(payload))
	verifyReq.Header.Set("Content-Type", "application/json")
	verifyRec := httptest.NewRecorder()
	s.authVerify(verifyRec, verifyReq)
	if verifyRec.Code != http.StatusOK { t.Fatalf("verify status=%d body=%s", verifyRec.Code, verifyRec.Body.String()) }
	cookies := verifyRec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != "gotron_session" || !cookies[0].HttpOnly { t.Fatal("expected HttpOnly session cookie") }
	sessionReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	sessionReq.AddCookie(cookies[0])
	sessionRec := httptest.NewRecorder()
	s.authMe(sessionRec, sessionReq)
	if sessionRec.Code != http.StatusOK { t.Fatalf("session status=%d body=%s", sessionRec.Code, sessionRec.Body.String()) }

	replayReq := httptest.NewRequest(http.MethodPost, "/api/auth/verify", strings.NewReader(payload))
	replayReq.Header.Set("Content-Type", "application/json")
	replayRec := httptest.NewRecorder()
	s.authVerify(replayRec, replayReq)
	if replayRec.Code != http.StatusUnauthorized { t.Fatalf("replayed challenge status=%d, want unauthorized", replayRec.Code) }
}

func TestExpiredSessionRejected(t *testing.T) {
	s := &server{sessions: map[string]authSession{"expired": {Address: "TExample", Network: "nile", Expires: time.Now().Add(-time.Minute)}}}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "gotron_session", Value: "expired"})
	rec := httptest.NewRecorder()
	s.authMe(rec, req)
	if rec.Code != http.StatusUnauthorized { t.Fatalf("status=%d, want %d", rec.Code, http.StatusUnauthorized) }
}
