---
name: bawarchi
description: Generate and use bawarchi CLIs from OpenAPI/Swagger specs or .proto files — registry commands, the env-var auth convention, and safety notes for side-effecting subcommands.
allowed-tools: "Bash"
---

# bawarchi — generate and use API CLIs

`bawarchi` turns an OpenAPI/Swagger spec or a `.proto` file into a standalone, typed CLI: every endpoint becomes a subcommand with flags generated from the spec. REST specs produce a native Go HTTP client; `.proto` files produce a CLI that shells out to `grpcurl`.

## Registry commands
```bash
bawarchi add <spec-url-or-path> [--name NAME] [--base-url URL] [--dry-run]
bawarchi list
bawarchi info <name>
bawarchi update <name> [--source SPEC] [--base-url URL]
bawarchi install <name> [--dir DIR]     # symlink onto PATH, default ~/.local/bin
bawarchi remove <name>
```
Registry state lives in `~/.bawarchi/registry.json`; generated source and binaries in `~/.bawarchi/src/<name>` and `~/.bawarchi/bin/<name>`.

## Auth and base URL convention
Each generated CLI reads its configuration from environment variables prefixed with the API name. For a spec that generates a CLI named `example-api`:
```bash
export EXAMPLE_API__API_KEY=...      # or __CREDENTIALS for basic-auth style APIs (email:key)
export EXAMPLE_API__BASE_URL=...     # optional override
example-api --help                   # prints exactly which vars this CLI expects
```
The exact variable names are derived from the spec, so check `<name> --help` rather than guessing. Set the value in a local dotfile or secret manager — never inline it in a script or commit it.

## Workflow: adding a new CLI
1. `bawarchi list` first — the API you need may already be registered.
2. `bawarchi add <spec>` — pass `--dry-run` to inspect the generated `main.go` before it compiles and registers.
3. `bawarchi install <name>` to put it on `PATH`.
4. Set the auth env var(s) it reports needing.
5. Smoke-test one read-only subcommand against the real API before relying on it in scripts or automation.

## Safety note
A generated CLI exposes the entire surface of the underlying API, including whatever create/update/delete/escalate/trigger operations the spec defines. Before wiring one into an automated workflow (a cron job, an agent, a CI step), treat any subcommand that mutates state as a real side effect — the same caution you'd apply to clicking the equivalent button in that service's UI.
