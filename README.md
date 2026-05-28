# bawarchi

[![CI](https://github.com/chinmayrelkar/bawarchi/actions/workflows/ci.yml/badge.svg)](https://github.com/chinmayrelkar/bawarchi/actions/workflows/ci.yml)

Generate standalone CLIs from API specs. Point `bawarchi` at an OpenAPI 3.x or
Swagger 2.0 document (file or HTTPS URL) or a `.proto` file, and it compiles a
self-contained command-line tool for that API.

- **REST** (OpenAPI 3.x / Swagger 2.0) — typed flags, request bodies, headers,
  arrays, `$ref` resolution, and standardized exit codes.
- **gRPC** (`.proto`) — generated CLIs shell out to
  [`grpcurl`](https://github.com/fullstorydev/grpcurl); TLS by default.

## Install

With Go:

```sh
go install github.com/chinmayrelkar/bawarchi/cmd/bawarchi@latest
```

Or download a prebuilt binary from the [releases page](https://github.com/chinmayrelkar/bawarchi/releases).

## Quick start

```sh
# Generate a CLI from a spec (file or https:// URL)
bawarchi add https://api.example.com/openapi.yaml

# Put it on your PATH
bawarchi install example-api

# Use it — auth and base URL come from environment variables
export EXAMPLE_API__API_KEY=sk-...
example-api --help
```

### Useful commands

| Command | Description |
|---------|-------------|
| `bawarchi add <spec>` | Generate, compile, and register a CLI (`--dry-run` to preview source, `--name`/`--base-url` to override) |
| `bawarchi list` | List generated CLIs |
| `bawarchi info <name>` | Show details for a CLI |
| `bawarchi update <name>` | Re-fetch the spec and regenerate (falls back to the cached spec if the source is offline) |
| `bawarchi install <name>` | Symlink a CLI onto your PATH |
| `bawarchi remove <name>` | Delete a CLI and its cached spec |
| `bawarchi --version` | Print the bawarchi version |

### Runtime configuration of generated CLIs

Generated CLIs read configuration from environment variables (prefix derived
from the API name):

- `NAME__API_KEY` / `NAME__TOKEN` / `NAME__CREDENTIALS` — auth, depending on the spec's security scheme
- `NAME__BASE_URL` — override the base URL
- `NAME__SERVER=<index>` — select one of a multi-server spec's predefined servers
- gRPC: `NAME__SERVER_ADDR` — override the server address

Generated REST CLIs exit `0` on success, `4` on a 4xx response, and `5` on a 5xx response.

## Development

```sh
go build ./...
go vet ./...
go test ./...
```

CI runs gofmt, vet, build, and tests on every pull request. Releases are cut by
pushing a `v*` tag, which triggers [GoReleaser](https://goreleaser.com) to build
cross-platform binaries.
