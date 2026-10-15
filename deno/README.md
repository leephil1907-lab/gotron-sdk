# TRON Console — Deno Deploy

A Deno-native TRON wallet application. It is a separate deployable implementation; the original Go SDK remains intact.

## Features

- TronLink connect/disconnect, network checks, account-change handling and balance refresh.
- Server-issued wallet authentication challenge, TronWeb `verifyMessageV2` signature verification, single-use nonce consumption through Deno KV, and expiring HttpOnly/SameSite session cookies.
- TRX account lookup, TRC-20 transfers, TRX transfers, Stake 2.0 freeze/unfreeze where the wallet provider supports them, transaction receipt lookup/polling.
- A dedicated `/presale` landing-page experience inspired by modern token-sale layouts, adapted for TRON mainnet and TronLink with responsive hero, countdown, purchase panel, tokenomics, participation steps, and FAQ. Presale checkout remains disabled until verified sale configuration is supplied.
- TRON-inspired dark UI, responsive dashboard, TRON red accents, SVG app icon, web manifest, Open Graph/Twitter metadata, security headers, API health endpoint.
- Uses the TRON HTTP API; no Go runtime or gRPC client is required on Deno Deploy.

## Deno Deploy setup

1. Push this branch and import the repository into [Deno Deploy](https://console.deno.com/).
2. Set the app's entrypoint to `deno/main.ts` (or deploy the `deno/` directory as the project root).
3. Provision and assign a **Deno KV** database to the app. KV stores one-time challenges and sessions with expiration; without an assigned database, authentication cannot persist.
4. Set environment variables in the Deno Deploy dashboard:
   - `TRONGRID_API_KEY` as a secret if your provider requires one. The app is locked to `https://api.trongrid.io` and TRON mainnet; network and node overrides are intentionally disabled.
   - `AUTH_COOKIE_SECURE=true` (default; retain this behind HTTPS).
5. Connect TronLink to TRON mainnet only. The app blocks recognized testnet/local-node endpoints. Use a wallet with funds you can afford to transact with, and verify every transaction in TronLink.

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
| `TRON_NETWORK` | Fixed to `mainnet` | Not configurable; app is mainnet-only |
| `TRON_NODE` | `https://api.trongrid.io` | Fixed official mainnet API endpoint; overrides are disabled |
| `TRONGRID_API_KEY` | empty | Optional TronGrid API key; stored server-side |
| `AUTH_COOKIE_SECURE` | enabled | Set `false` only for local HTTP development |

## Mainnet-only operation

- The app hard-codes the TRON mainnet network and official TronGrid mainnet endpoint. There is no testnet selector or environment-variable override.
- Wallet connection and transaction actions accept only the official `api.trongrid.io` wallet node. This strict node allowlist blocks testnet and arbitrary custom-node connections; still confirm TronLink is on mainnet before approving anything.

## Important notes

- Wallet signing and transfers are initiated in the user's TronLink wallet; the server never receives private keys or seed phrases.
- Authentication verifies a server-generated challenge and consumes it atomically in KV. Sessions are stored by hash of the random session token and expire after 12 hours.
- The app does not use the session as transaction authorization. Every transaction still requires an explicit wallet confirmation.
- TRC-20 amount input is an integer in base units. Token decimals differ by contract; verify contract and decimals independently.
- Stake 2.0 support depends on TronWeb and mainnet protocol support. Staking and unfreezing act on mainnet assets; review all details and costs in TronLink before signing.
- Public TRON endpoints can rate-limit requests. Configure an approved provider/API key for production.
- This implementation is added as a feature branch; it has not been deployed to a Deno Deploy account from this repository connection and live wallet testing remains required.
