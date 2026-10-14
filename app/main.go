package main

import (
	"context"
	"encoding/json"
	"errors"
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
	if err := tron.Start(client.GRPCInsecure()); err != nil {
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
:root{color-scheme:dark;--bg:#090b10;--panel:#11151d;--line:#242b38;--muted:#8b96a9;--text:#f3f6fc;--accent:#a3f36b;--red:#ff6969}*{box-sizing:border-box}body{margin:0;background:radial-gradient(ellipse at 15% -10%,#1b2a20 0,transparent 42%),var(--bg);color:var(--text);font:15px/1.5 Inter,ui-sans-serif,system-ui,-apple-system,sans-serif;min-height:100vh}.wrap{max-width:1120px;margin:auto;padding:28px 22px 56px}.top{display:flex;align-items:center;justify-content:space-between;gap:16px;border-bottom:1px solid var(--line);padding-bottom:22px}.brand{display:flex;align-items:center;gap:12px;font-weight:750;letter-spacing:-.04em;font-size:20px}.mark{display:grid;place-items:center;width:40px;height:40px;border-radius:13px;background:var(--accent);color:#10150e;font-size:22px;font-weight:900}.pill{border:1px solid #344233;color:var(--accent);border-radius:999px;padding:7px 11px;font-size:11px;font-weight:800;letter-spacing:.12em}.hero{padding:62px 0 34px;max-width:760px}.eyebrow{color:var(--accent);font-size:11px;font-weight:800;letter-spacing:.18em;text-transform:uppercase}.hero h1{font-size:clamp(38px,6vw,68px);line-height:1.02;letter-spacing:-.065em;margin:16px 0}.hero p{color:var(--muted);font-size:17px;max-width:590px}.layout{display:grid;grid-template-columns:1.25fr .75fr;gap:18px}.card{background:linear-gradient(145deg,#141923,#0e1118);border:1px solid var(--line);border-radius:20px;padding:24px;min-width:0}.card h2{font-size:17px;letter-spacing:-.025em;margin:0 0 6px}.sub{color:var(--muted);font-size:13px;margin:0 0 22px}.field{display:flex;gap:10px}.field input{min-width:0;flex:1;background:#090c12;border:1px solid #303949;color:var(--text);border-radius:11px;padding:14px;font:inherit;outline:none}.field input:focus{border-color:var(--accent);box-shadow:0 0 0 3px #a3f36b1c}button{border:0;border-radius:11px;padding:0 18px;background:var(--accent);color:#11170e;font:700 14px inherit;font-weight:800;cursor:pointer}button:disabled{opacity:.55;cursor:wait}.hint{color:var(--muted);font-size:12px;margin-top:12px}.status{display:flex;align-items:center;gap:8px;color:var(--muted);font-size:12px;margin-top:22px}.dot{width:8px;height:8px;border-radius:50%;background:var(--accent)}.metric-label{font-size:12px;color:var(--muted)}.balance{font-size:clamp(32px,4vw,46px);font-weight:750;letter-spacing:-.06em;margin:10px 0 2px;overflow-wrap:anywhere}.unit{font-size:14px;color:var(--muted);font-weight:500;letter-spacing:0}.result{display:none}.result.show{display:block}.address{overflow-wrap:anywhere;color:#d6dfed;font:12px/1.7 ui-monospace,SFMono-Regular,monospace;background:#090c12;border:1px solid var(--line);padding:12px;border-radius:10px;margin-top:18px}.meta{display:flex;justify-content:space-between;gap:12px;border-bottom:1px solid var(--line);padding:12px 0;font-size:13px}.meta:last-child{border:0}.meta span:first-child{color:var(--muted)}.error{display:none;margin-top:16px;color:#ffb5b5;background:#361c22;border:1px solid #65303a;padding:12px;border-radius:10px;font-size:13px}.error.show{display:block}.wallet-head{display:flex;align-items:center;gap:10px}.wallet-head button,.wallet-actions button,.stack button{min-height:42px}.wallet-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:18px;margin-top:18px}.section-head{display:flex;align-items:flex-start;justify-content:space-between;gap:12px}.wallet-indicator{font-size:11px;border:1px solid var(--line);border-radius:999px;padding:6px 9px;color:var(--muted);white-space:nowrap}.wallet-indicator.connected{color:var(--accent);border-color:#344233}.wallet-summary{overflow-wrap:anywhere;background:#090c12;border:1px solid var(--line);border-radius:10px;padding:13px;color:#d6dfed;font:12px/1.7 ui-monospace,SFMono-Regular,monospace;margin:12px 0}.wallet-actions{display:flex;gap:10px;flex-wrap:wrap}.secondary{background:#202734;color:var(--text);border:1px solid #303949}.stack{display:grid;gap:12px}.stack label{display:grid;gap:6px;color:var(--muted);font-size:12px}.stack input,.stack select{width:100%;min-width:0;background:#090c12;border:1px solid #303949;color:var(--text);border-radius:11px;padding:13px;font:inherit;font-size:14px;outline:none}.stack input:focus,.stack select:focus{border-color:var(--accent)}.stack button{padding:0 15px}.stack button:disabled,.wallet-actions button:disabled{opacity:.45;cursor:not-allowed}.wallet-grid .error{margin-top:10px}@media(max-width:760px){.wallet-grid{grid-template-columns:1fr}.wallet-head{align-items:flex-end;flex-direction:column}.wallet-head button{min-height:40px}.section-head{flex-direction:column}}.features{display:grid;grid-template-columns:repeat(3,1fr);gap:14px;margin-top:18px}.feature{border:1px solid var(--line);border-radius:16px;padding:18px}.feature strong{display:block;margin:8px 0 4px;font-size:13px}.feature p{margin:0;color:var(--muted);font-size:12px}.icon{color:var(--accent);font-size:20px}footer{display:flex;justify-content:space-between;gap:12px;flex-wrap:wrap;border-top:1px solid var(--line);margin-top:44px;padding-top:20px;color:var(--muted);font-size:12px}footer a{color:var(--accent);text-decoration:none}@media(max-width:760px){.wrap{padding:18px 14px 36px}.hero{padding:44px 0 24px}.layout{grid-template-columns:1fr}.field{flex-direction:column}.field button{height:48px}.features{grid-template-columns:1fr}.top{align-items:flex-start}.card{padding:19px}.pill{font-size:9px}} 
</style></head>
<body><main class="wrap"><header class="top"><div class="brand"><div class="mark">T</div><span>GoTRON <span style="color:var(--muted);font-weight:500">Console</span></span></div><div class="wallet-head"><span class="pill">● {{.Network}} NETWORK</span><button id="connectWallet" type="button">Connect wallet ↗</button></div></header>
<section class="hero"><div class="eyebrow">Blockchain intelligence · powered by Go</div><h1>Your window into<br>the TRON network.</h1><p>Look up any TRON account and retrieve its on-chain balance directly through the GoTRON SDK.</p></section>
<section class="layout"><div class="card"><h2>Account lookup</h2><p class="sub">Enter a public TRON address to inspect its current balance.</p><form id="lookup" class="field"><input id="address" name="address" placeholder="T..." autocomplete="off" autocapitalize="off" spellcheck="false" maxlength="64" aria-label="TRON public address" required><button id="submit" type="submit">Inspect account ↗</button></form><div class="hint">Public address only. Never enter a seed phrase or private key.</div><div class="error" id="error" role="alert"></div><div class="status"><span class="dot"></span><span id="status">Connected to the configured TRON node</span></div></div>
<div class="card"><div id="empty"><div class="eyebrow">Account snapshot</div><h2 style="margin-top:12px">Ready when you are.</h2><p class="sub" style="margin-top:10px;margin-bottom:0">Your balance and account details will appear here after a successful lookup.</p></div><div id="result" class="result"><div class="metric-label">Current balance</div><div class="balance" id="balance">— <span class="unit">TRX</span></div><div class="metric-label" id="sun">— SUN</div><div class="address" id="resultAddress"></div><div class="meta"><span>Network</span><strong id="network"></strong></div><div class="meta"><span>Account status</span><strong id="active"></strong></div><div class="meta"><span>Checked (UTC)</span><strong id="checked"></strong></div></div></div></section>
<section class="wallet-grid">
<div class="card"><div class="section-head"><div><div class="eyebrow">Wallet access</div><h2 style="margin-top:8px">Wallet connection & sign-in</h2><p class="sub" style="margin-top:8px">Connect with TronLink, verify the selected network, and sign a one-time login message.</p></div><span class="wallet-indicator" id="walletIndicator">Disconnected</span></div>
<div class="wallet-summary" id="walletSummary">No wallet connected. Your keys remain in your wallet extension.</div>
<div class="wallet-actions"><button type="button" id="signIn" disabled>Sign authentication message</button><button type="button" id="disconnect" class="secondary" disabled>Disconnect UI</button></div>
<div class="hint" id="authStatus">Wallet connection is available in a browser with TronLink installed.</div><div class="error" id="walletError" role="alert"></div></div>
<div class="card"><div class="eyebrow">Send TRX</div><h2 style="margin-top:8px">Transfer native TRX</h2><p class="sub" style="margin-top:8px">The connected wallet reviews and signs the transaction.</p>
<form id="trxForm" class="stack"><label>Recipient address<input id="trxTo" placeholder="T..." required maxlength="64" autocomplete="off"></label><label>Amount (TRX)<input id="trxAmount" type="number" min="0.000001" step="0.000001" placeholder="0.1" required></label><button id="sendTrx" type="submit" disabled>Review & send TRX</button></form>
</div>
<div class="card"><div class="eyebrow">TRC-20 token</div><h2 style="margin-top:8px">Transfer a token</h2><p class="sub" style="margin-top:8px">Enter the token contract address and raw token amount. Confirm details in your wallet.</p>
<form id="tokenForm" class="stack"><label>Token contract<input id="tokenContract" placeholder="TRC-20 contract address" required maxlength="64" autocomplete="off"></label><label>Recipient address<input id="tokenTo" placeholder="T..." required maxlength="64" autocomplete="off"></label><label>Token amount (base units)<input id="tokenAmount" type="number" min="1" step="1" placeholder="1000000" required></label><button id="sendToken" type="submit" disabled>Review & send token</button></form><p class="hint">Use base units based on the token's decimals. The app does not guess token decimals.</p></div>
<div class="card"><div class="eyebrow">TRON resources</div><h2 style="margin-top:8px">Stake / resource management</h2><p class="sub" style="margin-top:8px">Stake TRX for bandwidth or energy using the wallet's supported Stake 2.0 methods.</p>
<form id="stakeForm" class="stack"><label>Amount (TRX)<input id="stakeAmount" type="number" min="1" step="0.000001" placeholder="10" required></label><label>Resource<select id="stakeResource"><option value="ENERGY">Energy</option><option value="BANDWIDTH">Bandwidth</option></select></label><button id="stakeTrx" type="submit" disabled>Review & stake TRX</button></form>
<div class="wallet-actions" style="margin-top:12px"><button type="button" id="unstakeTrx" class="secondary" disabled>Unstake / unfreeze</button></div><p class="hint">Staking changes available balances and may impose protocol restrictions. Review the wallet prompt before confirming.</p></div>
<div class="card"><div class="eyebrow">Transaction monitor</div><h2 style="margin-top:8px">Track a transaction</h2><p class="sub" style="margin-top:8px">Look up a transaction ID and inspect its on-chain confirmation status.</p>
<form id="trackForm" class="stack"><label>Transaction ID<input id="txid" placeholder="64-character transaction ID" minlength="64" maxlength="64" required autocomplete="off"></label><button type="submit">Check transaction</button></form><div id="txStatus" class="wallet-summary" style="margin-top:14px">No transaction selected.</div><div id="txLink" class="hint"></div></div>
</section>
<section class="features"><article class="feature"><span class="icon">⌁</span><strong>Direct node queries</strong><p>Account data comes from the configured TRON gRPC node.</p></article><article class="feature"><span class="icon">◈</span><strong>Read-only by design</strong><p>No private keys, seed phrases, or signing credentials are collected.</p></article><article class="feature"><span class="icon">↗</span><strong>API-ready</strong><p>JSON endpoints make it straightforward to extend this console.</p></article></section>
<footer><span>GoTRON Console · Open-source SDK interface</span><span><a href="/api/health">Service health ↗</a> · No transaction signing enabled</span></footer></main>
<script>
const form=document.querySelector('#lookup'),input=document.querySelector('#address'),button=document.querySelector('#submit'),error=document.querySelector('#error'),status=document.querySelector('#status');
form.addEventListener('submit',async e=>{e.preventDefault();const value=input.value.trim();error.classList.remove('show');error.textContent='';button.disabled=true;button.textContent='Querying node…';status.textContent='Requesting account data…';try{const res=await fetch('/api/account?address='+encodeURIComponent(value),{headers:{Accept:'application/json'}});const data=await res.json();if(!res.ok)throw new Error(data.error||'The account lookup failed.');document.querySelector('#empty').style.display='none';document.querySelector('#result').classList.add('show');document.querySelector('#balance').innerHTML=new Intl.NumberFormat(undefined,{maximumFractionDigits:6}).format(data.balanceTRX)+' <span class="unit">TRX</span>';document.querySelector('#sun').textContent=new Intl.NumberFormat().format(data.balanceSUN)+' SUN';document.querySelector('#resultAddress').textContent=data.address;document.querySelector('#network').textContent=data.network.toUpperCase();document.querySelector('#active').textContent=data.active?'Balance above zero':'Zero balance';document.querySelector('#checked').textContent=new Date(data.checkedAtUtc).toLocaleTimeString([],{hour:'2-digit',minute:'2-digit',second:'2-digit',timeZone:'UTC'})+' UTC';status.textContent='Account data retrieved successfully.';}catch(err){error.textContent=err.message||'Unable to query the TRON node.';error.classList.add('show');status.textContent='Lookup did not complete.';}finally{button.disabled=false;button.textContent='Inspect account ↗';}});

const walletState={address:'',network:'',tronWeb:null,authenticated:false};
const $=id=>document.getElementById(id);
function walletError(message){$('walletError').textContent=message;$('walletError').classList.add('show');}
function clearWalletError(){$('walletError').classList.remove('show');$('walletError').textContent='';}
function shortAddress(a){return a?a.slice(0,7)+'…'+a.slice(-6):'';}
function getTronWeb(){return window.tronWeb&&window.tronWeb.ready?window.tronWeb:null;}
function setWalletConnected(address,network,tronWeb){walletState.address=address;walletState.network=network;walletState.tronWeb=tronWeb;walletState.authenticated=false;$('walletIndicator').textContent='Wallet connected';$('walletIndicator').classList.add('connected');$('walletSummary').textContent=address+'  ·  '+network;$('connectWallet').textContent=shortAddress(address)+' ▾';$('signIn').disabled=false;$('disconnect').disabled=false;$('sendTrx').disabled=false;$('sendToken').disabled=false;$('stakeTrx').disabled=false;$('unstakeTrx').disabled=false;$('authStatus').textContent='Connected. Sign a message to prove wallet control for this browser session.';}
async function connectWallet(){clearWalletError();try{if(!window.tronLink){throw new Error('TronLink was not detected. Install or enable the TronLink wallet, then reload this page.');}const permission=await window.tronLink.request({method:'tron_requestAccounts'});if(permission&&permission.code&&permission.code!==200){throw new Error(permission.message||'Wallet permission was not granted.');}const tw=getTronWeb()||window.tronWeb;if(!tw||!tw.defaultAddress||!tw.defaultAddress.base58){throw new Error('TronLink did not expose an active account. Unlock your wallet and try again.');}const addr=tw.defaultAddress.base58;const net=tw.fullNode&&tw.fullNode.host?tw.fullNode.host:'unknown network';const expected=(document.querySelector('.pill').textContent.match(/(MAINNET|NILE|SHASTA)/)||[])[1];const host=String(net).toLowerCase();if(expected==='NILE'&&!host.includes('nile')){throw new Error('Wrong network selected. Switch TronLink to Nile and reconnect.');}if(expected==='SHASTA'&&!host.includes('shasta')){throw new Error('Wrong network selected. Switch TronLink to Shasta and reconnect.');}if(expected==='MAINNET'&&(host.includes('nile')||host.includes('shasta'))){throw new Error('Wrong network selected. Switch TronLink to TRON mainnet and reconnect.');}setWalletConnected(addr,expected||net,tw);}catch(e){walletError(e.message||'Wallet connection failed.');$('authStatus').textContent='Wallet not connected.';}}
function disconnectWallet(){walletState.address='';walletState.network='';walletState.tronWeb=null;walletState.authenticated=false;$('walletIndicator').textContent='Disconnected';$('walletIndicator').classList.remove('connected');$('walletSummary').textContent='No wallet connected. Your keys remain in your wallet extension.';$('connectWallet').textContent='Connect wallet ↗';$('signIn').disabled=true;$('disconnect').disabled=true;$('sendTrx').disabled=true;$('sendToken').disabled=true;$('stakeTrx').disabled=true;$('unstakeTrx').disabled=true;$('authStatus').textContent='Disconnected from this interface.';clearWalletError();}
$('connectWallet').addEventListener('click',connectWallet);$('disconnect').addEventListener('click',disconnectWallet);
$('signIn').addEventListener('click',async()=>{clearWalletError();try{const tw=walletState.tronWeb;if(!tw||!walletState.address)throw new Error('Connect a wallet first.');const issuedAt=new Date().toISOString();const message='GoTRON Console sign-in\nAddress: '+walletState.address+'\nNetwork: '+walletState.network+'\nIssued At: '+issuedAt+'\nPurpose: Prove wallet control for this session. No transaction is authorized.';let signature;if(tw.trx.signMessageV2){signature=await tw.trx.signMessageV2(message);}else if(tw.trx.sign){signature=await tw.trx.sign(message);}else{throw new Error('This TronLink version does not support message signing. Update the wallet and retry.');}walletState.authenticated=true;$('authStatus').textContent='Message signed by '+shortAddress(walletState.address)+'. This proves wallet control locally; it does not create a server-side login session.';$('walletSummary').textContent=walletState.address+'\nSignature: '+signature+'\nSigned message: '+message;}catch(e){walletError(e.message||'Message signing was rejected or failed.');}});
function assertWallet(){if(!walletState.tronWeb||!walletState.address)throw new Error('Connect your TronLink wallet first.');const current=walletState.tronWeb.defaultAddress&&walletState.tronWeb.defaultAddress.base58;if(current!==walletState.address)throw new Error('The wallet account changed. Reconnect before continuing.');return walletState.tronWeb;}
function validTronAddress(tw,a){return typeof a==='string'&&tw.isAddress(a);}
async function watchTx(txid){$('txid').value=txid;$('txStatus').textContent='Submitted: '+txid+' · checking confirmation…';const tw=walletState.tronWeb;if(tw){for(let i=0;i<12;i++){await new Promise(r=>setTimeout(r,2500));try{const info=await tw.trx.getTransactionInfo(txid);if(info&&info.id){const result=info.receipt&&info.receipt.result?info.receipt.result:'PENDING';$('txStatus').textContent='Transaction '+result+' · block '+(info.blockNumber||'—')+' · fee '+(info.fee||0)+' SUN';$('txLink').innerHTML='<a style="color:var(--accent)" target="_blank" rel="noopener noreferrer" href="https://tronscan.org/#/transaction/'+encodeURIComponent(txid)+'">View on TronScan ↗</a>';return;}}catch(_){} }$('txStatus').textContent='Submitted transaction. Confirmation is not yet available from the wallet node; check again shortly.';}$('txLink').innerHTML='<a style="color:var(--accent)" target="_blank" rel="noopener noreferrer" href="https://tronscan.org/#/transaction/'+encodeURIComponent(txid)+'">View on TronScan ↗</a>';}
$('trxForm').addEventListener('submit',async e=>{e.preventDefault();const b=$('sendTrx');b.disabled=true;try{const tw=assertWallet(),to=$('trxTo').value.trim(),amount=Number($('trxAmount').value);if(!validTronAddress(tw,to))throw new Error('Recipient is not a valid TRON address.');if(!Number.isFinite(amount)||amount<=0)throw new Error('Enter an amount greater than zero.');if(!confirm('Review transfer before opening your wallet:\n\nFrom: '+walletState.address+'\nTo: '+to+'\nAmount: '+amount+' TRX\n\nContinue to wallet confirmation?'))return;const result=await tw.trx.sendTransaction(to,Math.round(amount*1e6));const txid=result&& (result.txid||result.transaction&&result.transaction.txID);if(!result||result.result===false||!txid)throw new Error((result&&result.message)||'Wallet did not return a transaction ID. No success is assumed.');$('txStatus').textContent='Broadcast accepted by wallet provider; awaiting on-chain confirmation.';await watchTx(txid);}catch(err){walletError(err.message||'TRX transfer failed.');}finally{b.disabled=!walletState.address;}});
$('tokenForm').addEventListener('submit',async e=>{e.preventDefault();const b=$('sendToken');b.disabled=true;try{const tw=assertWallet(),contract=$('tokenContract').value.trim(),to=$('tokenTo').value.trim(),amount=$('tokenAmount').value.trim();if(!validTronAddress(tw,contract)||!validTronAddress(tw,to))throw new Error('Check the token contract and recipient TRON addresses.');if(!/^\d+$/.test(amount)||BigInt(amount)<=0n)throw new Error('Token amount must be a positive integer in base units.');if(!confirm('Review token transfer:\n\nContract: '+contract+'\nRecipient: '+to+'\nBase units: '+amount+'\n\nConfirm only if this contract is the token you intend to send.'))return;const token=await tw.contract().at(contract);const txid=await token.transfer(to,amount).send({feeLimit:100000000,callValue:0,shouldPollResponse:false});if(typeof txid!=='string'||txid.length<32)throw new Error('Wallet did not return a transaction ID. Check the wallet and network before retrying.');await watchTx(txid);}catch(err){walletError(err.message||'Token transfer failed.');}finally{b.disabled=!walletState.address;}});
$('stakeForm').addEventListener('submit',async e=>{e.preventDefault();const b=$('stakeTrx');b.disabled=true;try{const tw=assertWallet();if(!tw.transactionBuilder||typeof tw.transactionBuilder.freezeBalanceV2!=='function')throw new Error('The connected TronWeb version does not expose Stake 2.0 freezeBalanceV2. Update TronLink or use a compatible wallet.');const trx=Number($('stakeAmount').value),resource=$('stakeResource').value;if(!Number.isFinite(trx)||trx<=0)throw new Error('Enter a valid amount greater than zero.');if(!confirm('Stake '+trx+' TRX for '+resource+'? This changes resource availability and requires wallet confirmation.'))return;const sun=Math.round(trx*1e6);const tx=await tw.transactionBuilder.freezeBalanceV2(sun,resource,walletState.address);const signed=await tw.trx.sign(tx);const sent=await tw.trx.sendRawTransaction(signed);const txid=sent&&sent.txid;if(!sent||sent.result===false||!txid)throw new Error('Stake transaction was not accepted by the node.');await watchTx(txid);}catch(err){walletError(err.message||'Staking transaction failed.');}finally{b.disabled=!walletState.address;}});
$('unstakeTrx').addEventListener('click',async()=>{try{const tw=assertWallet();if(!tw.transactionBuilder||typeof tw.transactionBuilder.unfreezeBalanceV2!=='function')throw new Error('Stake 2.0 unfreeze is not supported by the connected TronWeb provider.');const raw=prompt('Enter amount to unfreeze in TRX.');if(raw===null)return;const trx=Number(raw),resource=prompt('Enter resource: ENERGY or BANDWIDTH','ENERGY');if(!Number.isFinite(trx)||trx<=0)throw new Error('Enter a valid amount.');if(!['ENERGY','BANDWIDTH'].includes(resource))throw new Error('Resource must be ENERGY or BANDWIDTH.');if(!confirm('Request unfreeze of '+trx+' TRX from '+resource+'? Unfreeze rules and waiting periods apply.'))return;const tx=await tw.transactionBuilder.unfreezeBalanceV2(Math.round(trx*1e6),resource,walletState.address);const signed=await tw.trx.sign(tx);const sent=await tw.trx.sendRawTransaction(signed);if(!sent||sent.result===false||!sent.txid)throw new Error('Unfreeze transaction was not accepted by the node.');await watchTx(sent.txid);}catch(err){walletError(err.message||'Unfreeze failed.');}});
$('trackForm').addEventListener('submit',async e=>{e.preventDefault();const id=$('txid').value.trim();if(!/^[a-fA-F0-9]{64}$/.test(id)){$('txStatus').textContent='Enter a 64-character hexadecimal transaction ID.';return;}$('txStatus').textContent='Checking transaction on the connected node…';$('txLink').innerHTML='<a style="color:var(--accent)" target="_blank" rel="noopener noreferrer" href="https://tronscan.org/#/transaction/'+encodeURIComponent(id)+'">View on TronScan ↗</a>';try{const tw=walletState.tronWeb||getTronWeb();if(!tw)throw new Error('Connect TronLink to query transaction status.');const [tx,info]=await Promise.all([tw.trx.getTransaction(id),tw.trx.getTransactionInfo(id)]);if(!tx||!tx.txID)throw new Error('Transaction not found by the connected node.');if(!info||!info.id){$('txStatus').textContent='Transaction exists but confirmation information is not yet available.';return;}$('txStatus').textContent='On-chain transaction found · '+(info.receipt&&info.receipt.result||'CONFIRMED')+' · block '+(info.blockNumber||'—')+' · fee '+(info.fee||0)+' SUN';}catch(err){$('txStatus').textContent=err.message||'Unable to retrieve transaction status.';}});
if(window.tronLink&&getTronWeb()){const tw=getTronWeb();if(tw.defaultAddress&&tw.defaultAddress.base58)setWalletConnected(tw.defaultAddress.base58,tw.fullNode&&tw.fullNode.host||'TRON',tw);}
</script></body></html>`
