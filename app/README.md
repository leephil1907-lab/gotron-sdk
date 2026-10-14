# GoTRON Console

A small web application built on the GoTRON SDK in this repository. It provides a responsive TRON account lookup UI and JSON health/account endpoints.

## Features

- Responsive account lookup interface.
- Reads public account balances from the configured TRON gRPC node using `pkg/client`.
- Validates TRON Base58 addresses before querying.
- JSON endpoints: `GET /api/health` and `GET /api/account?address=T...`.
- Security headers, request timeouts, and no-store API responses.
- No private-key entry, transaction signing, or transaction broadcasting.

## Run locally

Requires the Go version declared by the root `go.mod` (Go 1.25+).

```sh
go run ./app
```

Open http://localhost:8080.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | HTTP listen address |
| `TRON_NETWORK` | `mainnet` | Label/network preset: `mainnet`, `nile`, or `shasta` |
| `TRON_NODE` | node for selected network | Override the gRPC host:port |

Examples:

```sh
TRON_NETWORK=nile go run ./app
TRON_NODE=your-node.example:50051 go run ./app
```

Use a provider-approved endpoint and follow its API-key/TLS requirements. This sample uses the SDK's insecure gRPC transport option for compatibility with the public node presets; do not expose a plaintext gRPC connection across an untrusted network. For production, configure TLS and provider credentials supported by your node.

## Endpoints

- `GET /` — dashboard.
- `GET /api/health` — process/configuration health.
- `GET /api/account?address=<base58>` — account balance in SUN and TRX.

## Build

```sh
go build -o gotron-console ./app
./gotron-console
```

## Scope

This is an account-explorer application, not a custodial wallet. Account balance is fetched live from the configured node. A zero balance does not prove that an address has never existed. Network reachability and node-provider limits affect availability. Transaction signing, sending, staking, token indexing, authentication, and persistent history are intentionally not represented as implemented features.
