package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/client"
)

type server struct {
	tron *client.GrpcClient
	tpl  *template.Template
}

type accountResponse struct {
	Address      string  `json:"address"`
	BalanceTRX   float64 `json:"balanceTRX"`
	BalanceSUN   int64   `json:"balanceSUN"`
	Network      string  `json:"network"`
	Active       bool    `json:"active"`
	CheckedAtUTC string  `json:"checkedAtUtc"`
}

func main() {
	addr := env("LISTEN_ADDR", ":8080")
	network := strings.ToLower(env("TRON_NETWORK", "mainnet"))
	node := env("TRON_NODE", defaultNode(network))
	if _, _, err := net.SplitHostPort(node); err != nil {
		log.Fatalf("invalid TRON_NODE %q: expected host:port", node)
	}

	tron := client.NewGrpcClient(node)
	opts := []client.GrpcClientOption{client.GRPCInsecure()}
	if err := tron.Start(opts...); err != nil {
		log.Fatalf("connect to TRON node %q: %v", node, err)
	}
	defer tron.Stop()

	tpl := template.Must(template.New("index").Parse(page))
	s := &server{tron: tron, tpl: tpl}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.home)
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/account", s.account)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("GoTRON Console listening on %s (network=%s node=%s)", addr, network, node)
	log.Fatal(httpServer.ListenAndServe())
}

func (s *server) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.tpl.Execute(w, map[string]string{"Network": strings.ToUpper(env("TRON_NETWORK", "mainnet"))}); err != nil {
		log.Printf("render home: %v", err)
	}
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok", "service": "gotron-console",
		"network": strings.ToLower(env("TRON_NETWORK", "mainnet")),
		"node": env("TRON_NODE", defaultNode(strings.ToLower(env("TRON_NETWORK", "mainnet")))),
		"timeUtc": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *server) account(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("address"))
	if raw == "" || len(raw) > 64 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Enter a valid TRON Base58 address."})
		return
	}
	parsed, err := address.Base58ToAddress(raw)
	if err != nil || parsed.String() == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "The address format is invalid. Use a TRON Base58 address beginning with T."})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	type result struct {
		account interface{}
		err error
	}
	ch := make(chan result, 1)
	go func() {
		a, e := s.tron.GetAccount(parsed.String())
		ch <- result{account: a, err: e}
	}()
	select {
	case <-ctx.Done():
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "The TRON node did not respond in time. Try again."})
		return
	case res := <-ch:
		if res.err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "The TRON node could not retrieve this account. Try again shortly."})
			return
		}
		// The SDK returns the protocol account type; its balance is denominated in SUN.
		type balanceAccount interface { GetBalance() int64 }
		a, ok := res.account.(balanceAccount)
		if !ok {
			// Keep the response explicit rather than guessing at an SDK-specific payload shape.
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Unexpected account response from the connected node."})
			return
		}
		sun := a.GetBalance()
		writeJSON(w, http.StatusOK, accountResponse{
			Address: parsed.String(), BalanceTRX: float64(sun) / 1_000_000,
			BalanceSUN: sun, Network: strings.ToLower(env("TRON_NETWORK", "mainnet")),
			Active: sun > 0, CheckedAtUTC: time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func defaultNode(network string) string {
	switch network {
	case "nile":
		return "grpc.nile.trongrid.io:50051"
	case "shasta":
		return "grpc.shasta.trongrid.io:50051"
	default:
		return "grpc.trongrid.io:50051"
	}
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" { return v }
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	w.WriteHeader(status)
	if err := enc.Encode(value); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("write JSON response: %v", err)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

const page = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="theme-color" content="#090b10"><meta name="description" content="A lightweight TRON account explorer powered by GoTRON SDK.">
<title>GoTRON Console</title>
<style>
:root{color-scheme:dark;--bg:#090b10;--panel:#11151d;--line:#242b38;--muted:#8b96a9;--text:#f3f6fc;--accent:#a3f36b;--red:#ff6969}*{box-sizing:border-box}body{margin:0;background:radial-gradient(ellipse at 15% -10%,#1b2a20 0,transparent 42%),var(--bg);color:var(--text);font:15px/1.5 Inter,ui-sans-serif,system-ui,-apple-system,sans-serif;min-height:100vh}.wrap{max-width:1120px;margin:auto;padding:28px 22px 56px}.top{display:flex;align-items:center;justify-content:space-between;gap:16px;border-bottom:1px solid var(--line);padding-bottom:22px}.brand{display:flex;align-items:center;gap:12px;font-weight:750;letter-spacing:-.04em;font-size:20px}.mark{display:grid;place-items:center;width:40px;height:40px;border-radius:13px;background:var(--accent);color:#10150e;font-size:22px;font-weight:900}.pill{border:1px solid #344233;color:var(--accent);border-radius:999px;padding:7px 11px;font-size:11px;font-weight:800;letter-spacing:.12em}.hero{padding:62px 0 34px;max-width:760px}.eyebrow{color:var(--accent);font-size:11px;font-weight:800;letter-spacing:.18em;text-transform:uppercase}.hero h1{font-size:clamp(38px,6vw,68px);line-height:1.02;letter-spacing:-.065em;margin:16px 0}.hero p{color:var(--muted);font-size:17px;max-width:590px}.layout{display:grid;grid-template-columns:1.25fr .75fr;gap:18px}.card{background:linear-gradient(145deg,#141923,#0e1118);border:1px solid var(--line);border-radius:20px;padding:24px;min-width:0}.card h2{font-size:17px;letter-spacing:-.025em;margin:0 0 6px}.sub{color:var(--muted);font-size:13px;margin:0 0 22px}.field{display:flex;gap:10px}.field input{min-width:0;flex:1;background:#090c12;border:1px solid #303949;color:var(--text);border-radius:11px;padding:14px;font:inherit;outline:none}.field input:focus{border-color:var(--accent);box-shadow:0 0 0 3px #a3f36b1c}button{border:0;border-radius:11px;padding:0 18px;background:var(--accent);color:#11170e;font:700 14px inherit;font-weight:800;cursor:pointer}button:disabled{opacity:.55;cursor:wait}.hint{color:var(--muted);font-size:12px;margin-top:12px}.status{display:flex;align-items:center;gap:8px;color:var(--muted);font-size:12px;margin-top:22px}.dot{width:8px;height:8px;border-radius:50%;background:var(--accent)}.metric-label{font-size:12px;color:var(--muted)}.balance{font-size:clamp(32px,4vw,46px);font-weight:750;letter-spacing:-.06em;margin:10px 0 2px;overflow-wrap:anywhere}.unit{font-size:14px;color:var(--muted);font-weight:500;letter-spacing:0}.result{display:none}.result.show{display:block}.address{overflow-wrap:anywhere;color:#d6dfed;font:12px/1.7 ui-monospace,SFMono-Regular,monospace;background:#090c12;border:1px solid var(--line);padding:12px;border-radius:10px;margin-top:18px}.meta{display:flex;justify-content:space-between;gap:12px;border-bottom:1px solid var(--line);padding:12px 0;font-size:13px}.meta:last-child{border:0}.meta span:first-child{color:var(--muted)}.error{display:none;margin-top:16px;color:#ffb5b5;background:#361c22;border:1px solid #65303a;padding:12px;border-radius:10px;font-size:13px}.error.show{display:block}.features{display:grid;grid-template-columns:repeat(3,1fr);gap:14px;margin-top:18px}.feature{border:1px solid var(--line);border-radius:16px;padding:18px}.feature strong{display:block;margin:8px 0 4px;font-size:13px}.feature p{margin:0;color:var(--muted);font-size:12px}.icon{color:var(--accent);font-size:20px}footer{display:flex;justify-content:space-between;gap:12px;flex-wrap:wrap;border-top:1px solid var(--line);margin-top:44px;padding-top:20px;color:var(--muted);font-size:12px}footer a{color:var(--accent);text-decoration:none}@media(max-width:760px){.wrap{padding:18px 14px 36px}.hero{padding:44px 0 24px}.layout{grid-template-columns:1fr}.field{flex-direction:column}.field button{height:48px}.features{grid-template-columns:1fr}.top{align-items:flex-start}.card{padding:19px}.pill{font-size:9px}} 
</style></head>
<body><main class="wrap"><header class="top"><div class="brand"><div class="mark">T</div><span>GoTRON <span style="color:var(--muted);font-weight:500">Console</span></span></div><span class="pill">● {{.Network}} NETWORK</span></header>
<section class="hero"><div class="eyebrow">Blockchain intelligence · powered by Go</div><h1>Your window into<br>the TRON network.</h1><p>Look up any TRON account and retrieve its on-chain balance directly through the GoTRON SDK.</p></section>
<section class="layout"><div class="card"><h2>Account lookup</h2><p class="sub">Enter a public TRON address to inspect its current balance.</p><form id="lookup" class="field"><input id="address" name="address" placeholder="T..." autocomplete="off" autocapitalize="off" spellcheck="false" maxlength="64" aria-label="TRON public address" required><button id="submit" type="submit">Inspect account ↗</button></form><div class="hint">Public address only. Never enter a seed phrase or private key.</div><div class="error" id="error" role="alert"></div><div class="status"><span class="dot"></span><span id="status">Connected to the configured TRON node</span></div></div>
<div class="card"><div id="empty"><div class="eyebrow">Account snapshot</div><h2 style="margin-top:12px">Ready when you are.</h2><p class="sub" style="margin-top:10px;margin-bottom:0">Your balance and account details will appear here after a successful lookup.</p></div><div id="result" class="result"><div class="metric-label">Current balance</div><div class="balance" id="balance">— <span class="unit">TRX</span></div><div class="metric-label" id="sun">— SUN</div><div class="address" id="resultAddress"></div><div class="meta"><span>Network</span><strong id="network"></strong></div><div class="meta"><span>Account status</span><strong id="active"></strong></div><div class="meta"><span>Checked (UTC)</span><strong id="checked"></strong></div></div></div></section>
<section class="features"><article class="feature"><span class="icon">⌁</span><strong>Direct node queries</strong><p>Account data comes from the configured TRON gRPC node.</p></article><article class="feature"><span class="icon">◈</span><strong>Read-only by design</strong><p>No private keys, seed phrases, or signing credentials are collected.</p></article><article class="feature"><span class="icon">↗</span><strong>API-ready</strong><p>JSON endpoints make it straightforward to extend this console.</p></article></section>
<footer><span>GoTRON Console · Open-source SDK interface</span><span><a href="/api/health">Service health ↗</a> · No transaction signing enabled</span></footer></main>
<script>
const form=document.querySelector('#lookup'),input=document.querySelector('#address'),button=document.querySelector('#submit'),error=document.querySelector('#error'),status=document.querySelector('#status');
form.addEventListener('submit',async e=>{e.preventDefault();const value=input.value.trim();error.classList.remove('show');error.textContent='';button.disabled=true;button.textContent='Querying node…';status.textContent='Requesting account data…';try{const res=await fetch('/api/account?address='+encodeURIComponent(value),{headers:{Accept:'application/json'}});const data=await res.json();if(!res.ok)throw new Error(data.error||'The account lookup failed.');document.querySelector('#empty').style.display='none';document.querySelector('#result').classList.add('show');document.querySelector('#balance').innerHTML=new Intl.NumberFormat(undefined,{maximumFractionDigits:6}).format(data.balanceTRX)+' <span class="unit">TRX</span>';document.querySelector('#sun').textContent=new Intl.NumberFormat().format(data.balanceSUN)+' SUN';document.querySelector('#resultAddress').textContent=data.address;document.querySelector('#network').textContent=data.network.toUpperCase();document.querySelector('#active').textContent=data.active?'Balance above zero':'Zero balance';document.querySelector('#checked').textContent=new Date(data.checkedAtUtc).toLocaleTimeString([],{hour:'2-digit',minute:'2-digit',second:'2-digit',timeZone:'UTC'})+' UTC';status.textContent='Account data retrieved successfully.';}catch(err){error.textContent=err.message||'Unable to query the TRON node.';error.classList.add('show');status.textContent='Lookup did not complete.';}finally{button.disabled=false;button.textContent='Inspect account ↗';}});
</script></body></html>`
