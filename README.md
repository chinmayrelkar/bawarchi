# bawarchi

[![CI](https://github.com/chinmayrelkar/bawarchi/actions/workflows/ci.yml/badge.svg)](https://github.com/chinmayrelkar/bawarchi/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/chinmayrelkar/bawarchi.svg)](https://pkg.go.dev/github.com/chinmayrelkar/bawarchi)
[![Release](https://img.shields.io/github/v/release/chinmayrelkar/bawarchi)](https://github.com/chinmayrelkar/bawarchi/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

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

Or download a prebuilt binary (linux/darwin/windows, amd64/arm64) from the
[releases page](https://github.com/chinmayrelkar/bawarchi/releases) — each
release includes a `checksums.txt` to verify the download.

## Quick start (REST)

```sh
# Generate a CLI from a spec (file or https:// URL)
bawarchi add https://api.example.com/openapi.yaml

# Put it on your PATH
bawarchi install example-api

# Use it — auth and base URL come from environment variables
export EXAMPLE_API__API_KEY=sk-...
example-api --help
```

## Quick start (gRPC)

```sh
bawarchi add ./greeter.proto
bawarchi install greeter

export GREETER__AUTH_TOKEN=...
greeter --help
```

Generated gRPC CLIs shell out to `grpcurl` and connect over TLS by default.
Control behavior with annotations in the `.proto` file (anywhere in the file,
as a `//` comment):

| Annotation | Effect |
|---|---|
| `// @server: host:port` | Sets the default server address (falls back to `localhost:50051` with a warning if omitted) |
| `// @service: com.example.v1` | Sets the fully-qualified gRPC service package/prefix used to build the method path |
| `// @noauth` | Marks the service as not requiring a bearer token; the generated CLI skips the auth-required check |

### Useful commands

| Command | Description |
|---------|-------------|
| `bawarchi add <spec>` | Generate, compile, and register a CLI (`--dry-run` to preview source, `--name`/`--base-url` to override) |
| `bawarchi list` | List generated CLIs |
| `bawarchi info <name>` | Show details for a CLI |
| `bawarchi update <name>` | Re-fetch the spec and regenerate (`--source` to switch spec sources, `--base-url` to override; falls back to the cached spec if the source is offline) |
| `bawarchi install <name>` | Symlink a CLI onto your PATH (`--dir` to override the install directory, default `~/.local/bin`) |
| `bawarchi remove <name>` | Delete a CLI and its cached spec |
| `bawarchi --version` | Print the bawarchi version |

### Runtime configuration of generated CLIs

Generated CLIs read configuration from environment variables (prefix derived
from the API name, e.g. `EXAMPLE_API__...`):

- `NAME__API_KEY` / `NAME__TOKEN` / `NAME__CREDENTIALS` — auth, depending on the spec's security scheme
- `NAME__BASE_URL` — override the base URL
- `NAME__SERVER=<index>` — select one of a multi-server spec's predefined servers
- gRPC: `NAME__SERVER_ADDR` — override the server address (overrides `// @server:` in the proto)

Generated REST CLIs exit `0` on success, `4` on a 4xx response, and `5` on a 5xx response.

## Development

```sh
go build ./...
go vet ./...
go test ./...
```

CI runs gofmt, vet, build, and tests on every pull request and on every push to
`main`. Once CI is green on `main`, an [auto-release workflow](.github/workflows/auto-release.yml)
automatically bumps a semver tag (`feat:` commits → minor, `BREAKING CHANGE`/`!:` → major,
everything else → patch) and runs [GoReleaser](https://goreleaser.com) to publish
cross-platform binaries — no manual tagging needed. `.github/workflows/release.yml`
remains as a manual fallback for hand-pushed tags.

## License

[MIT](LICENSE)
