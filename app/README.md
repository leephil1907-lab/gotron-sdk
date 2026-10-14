# GoTRON Console

A web application built on the GoTRON SDK in this repository. It includes a responsive TRON account explorer and a TronLink-powered wallet interface.

## Features

- Responsive account explorer powered by the GoTRON gRPC SDK.
- Public account lookup with balance shown in SUN and TRX.
- TronLink connect button, network preset checks, account-change handling, server-verified message signing, and expiring HttpOnly session cookies.
- Native TRX transfers through the connected wallet.
- TRC-20 transfers by contract address and raw token base units.
- Stake 2.0 freeze/unfreeze actions when the connected TronWeb provider supports the required methods.
- Transaction lookup and confirmation polling, plus a TronScan link.
- JSON endpoints: `GET /api/health` and `GET /api/account?address=T...`.
- HTTP security headers, bounded HTTP timeouts, and no-store API responses.
- No seed phrase or private-key fields; signing requests are delegated to the connected wallet.

## Run locally

Requires the Go version declared by the root `go.mod` (Go 1.25+).

```sh
go run ./app
```

Open http://localhost:8080 in a browser where TronLink is installed and unlocked to use wallet features.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | HTTP listen address |
| `TRON_NETWORK` | `mainnet` | Network preset: `mainnet`, `nile`, or `shasta` |
| `TRON_NODE` | preset node | Override the Go SDK gRPC host:port |
| `AUTH_COOKIE_SECURE` | `true` | Set session cookies with the Secure attribute; keep enabled behind HTTPS in production |

Examples:

```sh
TRON_NETWORK=nile go run ./app
TRON_NODE=your-node.example:50051 go run ./app
```

Use a provider-approved endpoint and follow its API-key/TLS requirements. The backend SDK client currently uses the SDK's insecure gRPC transport option for compatibility with public node presets; do not expose plaintext gRPC over an untrusted network. Configure TLS and provider credentials before production deployment.

## Wallet and transaction behavior

- Wallet integration requires TronLink and its injected `window.tronWeb` provider. It is not a WalletConnect implementation.
- Sign-in obtains a random, single-use server challenge bound to the address and configured network. The server recovers the signer from the TRON-prefixed message signature, compares the derived Base58Check address, and issues a 12-hour HttpOnly, SameSite=Strict cookie. Challenges expire after five minutes and are consumed on verification attempts. This proves wallet control; it does not authorize transactions.
- TRX, TRC-20, staking, and unfreeze operations are submitted through the wallet provider and require explicit wallet confirmation. Wallet/network provider behavior can differ by version.
- TRC-20 amounts are entered in base units; the app deliberately does not guess token decimals or token authenticity.
- Stake 2.0 methods depend on the TronWeb version and network support. Review resource type, amount, and wallet prompt before approving.
- Transaction polling is best-effort and depends on the connected wallet's node. A returned transaction ID means submission was reported by the provider, not that the transaction succeeded; inspect the transaction receipt/status.

## Endpoints

- `GET /` — dashboard.
- `GET /api/health` — process/configuration health.
- `GET /api/account?address=<base58>` — account balance in SUN and TRX.

## Build and test

```sh
go test ./...
go build -o gotron-console ./app
./gotron-console
```

The repository's complete test suite and a live wallet transaction flow must be run in a Go 1.25+ environment before production deployment.

## Scope

This application is a non-custodial wallet interface and account explorer, not a hosted key vault. It does not collect private keys or seed phrases. Sessions and challenges are held in process memory, so active sessions are lost on restart and this implementation is intended for a single instance. Use a shared, TTL-backed session store and rate limiting before multi-instance production use. A token metadata registry, address book, persistent transaction history, WalletConnect, multi-signature workflow, and production monitoring are not included.
