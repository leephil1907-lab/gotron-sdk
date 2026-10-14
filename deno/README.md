# TRON Console — Deno Deploy

A Deno-native TRON wallet application. It is a separate deployable implementation; the original Go SDK remains intact.

## Features

- TronLink connect/disconnect, network checks, account-change handling and balance refresh.
- Server-issued wallet authentication challenge, TronWeb `verifyMessageV2` signature verification, single-use nonce consumption through Deno KV, and expiring HttpOnly/SameSite session cookies.
- TRX account lookup, TRC-20 transfers, TRX transfers, Stake 2.0 freeze/unfreeze where the wallet provider supports them, transaction receipt lookup/polling.
- TRON-inspired dark UI, responsive dashboard, TRON red accents, SVG app icon, web manifest, Open Graph/Twitter metadata, security headers, API health endpoint.
- Uses the TRON HTTP API; no Go runtime or gRPC client is required on Deno Deploy.

## Deno Deploy setup

1. Push this branch and import the repository into [Deno Deploy](https://console.deno.com/).
2. Set the app's entrypoint to `deno/main.ts` (or deploy the `deno/` directory as the project root).
3. Provision and assign a **Deno KV** database to the app. KV stores one-time challenges and sessions with expiration; without an assigned database, authentication cannot persist.
4. Set environment variables in the Deno Deploy dashboard:
   - `TRON_NETWORK=nile` for initial testnet use; set `mainnet` only after testnet checks pass.
   - `TRON_NODE=https://nile.trongrid.io` (optional override).
   - `TRONGRID_API_KEY` as a secret if your provider requires one.
   - `AUTH_COOKIE_SECURE=true` (default; retain this behind HTTPS).
5. Test on a fresh TronLink wallet connected to the same network before considering mainnet.

The code uses Deno Deploy's managed Deno KV integration. See Deno's official [KV deployment guide](https://docs.deno.com/deploy/reference/deno_kv/) and [environment variable docs](https://docs.deno.com/deploy/reference/env_vars_and_contexts/).

## Local run

```sh
cd deno
deno task dev
```

Deno will prompt for the `npm:tronweb` dependency the first time. Local KV defaults to a local database; for the local server allow network, env and read permissions as in `deno.json`.

## Deploy

With the current Deno Deploy CLI authenticated and an app selected:

```sh
cd deno
deno deploy
```

The app can also be connected to GitHub in the Deno Deploy dashboard for automatic deployments.

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `TRON_NETWORK` | `nile` | `nile`, `shasta`, or `mainnet` |
| `TRON_NODE` | Network-specific TRON HTTP endpoint | Endpoint for public node API |
| `TRONGRID_API_KEY` | empty | Optional TronGrid API key; stored server-side |
| `AUTH_COOKIE_SECURE` | enabled | Set `false` only for local HTTP development |

## Important notes

- Wallet signing and transfers are initiated in the user's TronLink wallet; the server never receives private keys or seed phrases.
- Authentication verifies a server-generated challenge and consumes it atomically in KV. Sessions are stored by hash of the random session token and expire after 12 hours.
- The app does not use the session as transaction authorization. Every transaction still requires an explicit wallet confirmation.
- TRC-20 amount input is an integer in base units. Token decimals differ by contract; verify contract and decimals independently.
- Stake 2.0 support depends on TronWeb and network support. Test with test assets before mainnet.
- Public TRON endpoints can rate-limit requests. Configure an approved provider/API key for production.
- This implementation is added as a feature branch; it has not been deployed to a Deno Deploy account from this repository connection and live wallet testing remains required.
