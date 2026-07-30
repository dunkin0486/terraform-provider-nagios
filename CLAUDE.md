# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A Terraform provider for Nagios XI, built on `terraform-plugin-framework`. It wraps Nagios XI's REST API (`/api/v1/config/*` and `/api/v1/system/*`), which has no official Go SDK and several real quirks documented below — those quirks are load-bearing, not legacy cruft, and must be preserved by any change to `internal/client`.

## Commands

```bash
go build ./...              # build the provider binary
go vet ./...                # static analysis, run before every commit
gofmt -l -w internal/ main.go   # format (CI does not auto-format for you)
golangci-lint run ./...     # broader lint pass (staticcheck, unused, errcheck, etc.), also enforced in CI
go test ./internal/client/...   # unit tests, no external dependencies

# Single unit test:
go test ./internal/client/... -run TestBuildURL_PUT_ServiceAppendsDescriptionSegment -v
```

A `.pre-commit-config.yaml` runs `gofmt`/`go vet`/`golangci-lint` automatically on changed files at commit time (`pre-commit install` once per clone). It does not run `go build`, unit tests, or acceptance tests.

### Acceptance tests

Acceptance tests (`internal/provider/*_test.go`) need a real Nagios XI instance and are skipped unless `TF_ACC=1` is set. The `test/docker/nagiosxi/` directory boots one in Docker:

```bash
cd test/docker/nagiosxi
docker compose up --build -d      # first boot takes ~50 min (compiles Nagios core, MRTG, plugins from source)
eval "$(./get-api-token.sh)"      # sets API_TOKEN and NAGIOS_URL

cd /path/to/repo/root
TF_ACC=1 go test -v -count=1 ./internal/provider/...                          # full suite
TF_ACC=1 go test -v -count=1 ./internal/provider/... -run TestAccHostBasic    # single test
```

`get-api-token.sh` must be run with `test/docker/nagiosxi/` as the working directory (it relies on `docker compose` finding the compose file via cwd). Always pass `-count=1` when re-running acceptance tests — Go caches test results by default, which silently skips re-hitting the live container.

**Run the full acceptance suite before opening a PR that touches `internal/client` or `internal/provider`.** CI does not do this for you — `.github/workflows/test.yml` only runs unit tests and lint, since booting the Docker instance takes ~50 minutes and `TF_ACC` is never set in CI. Every real bug found while building this provider (the `getX`-never-returns-nil bug, `free_variables` never round-tripping correctly, `2d_coords`/`3d_coords` being invalid Terraform attribute names) was caught by actually running the acceptance suite against a live instance — none of them were caught by `go build`, `go vet`, or a unit test.

The container reports itself `healthy` once installed; `docker compose ps` from `test/docker/nagiosxi/` confirms this. See `test/docker/nagiosxi/README.md` for the installer quirks that were worked around to get it booting (Rocky Linux OS-detection, php-fpm ACL support, etc.) — that layer is unrelated to the provider code itself.

### Test-driven development

New `internal/client`/`internal/provider` behavior is written test-first, not implementation-first. This is how the original framework rewrite was done (see the closed "Phase 3: TDD rewrite - ..." issues, #74-81) and it's the expected workflow for new work, not just historical practice.

For a new field/resource/data source:

1. Write the acceptance test (or unit test, for pure `internal/client` logic) first, asserting the behavior you want.
2. Run it against the live Docker instance and confirm it fails for the expected reason (missing schema attribute, unset field, etc.) — a red test that fails for the wrong reason (a typo, a config error) proves nothing.
3. Implement the minimum change to make it pass.
4. Run the full acceptance suite before opening the PR, per the rule above.

Skipping straight to implementation and writing tests afterward is a common shortcut to avoid — it's easy to end up with tests that just confirm what the code already does rather than what it's supposed to do.

### Docs

```bash
tfplugindocs generate   # regenerates docs/ from schema Description fields + examples/
```

Docs are generated, not hand-written. Never edit `docs/*.md` directly — edit the `Description` field on the relevant schema attribute in `internal/provider/`, or the example `.tf` file in `examples/`, and regenerate.

### Commit messages and releases

Releases are automated via [release-please](https://github.com/googleapis/release-please) (`.github/workflows/release-please.yml`): it parses [Conventional Commits](https://www.conventionalcommits.org/) prefixes on `main` to decide the next version and drive `CHANGELOG.md`. **PR titles/merge commits need a `fix:`/`feat:`/`feat!:` (or `BREAKING CHANGE:` footer) prefix** or they won't be picked up as release-worthy — see CONTRIBUTING.md for the full mapping. Don't hand-edit `CHANGELOG.md`; release-please owns it now.

## Architecture

Two packages, deliberately separated by an import boundary:

- **`internal/client`** — a plain Go HTTP client for Nagios XI's REST API. Zero dependency on `terraform-plugin-framework`; unit-testable standalone. One file per object type (`host.go`, `service.go`, etc.), each exporting `NewX`/`GetX`/`UpdateX`/`DeleteX` on `*Client`.
- **`internal/provider`** — depends on both `internal/client` and `terraform-plugin-framework`. One file per resource/data source, each with a `<type>Model` struct (`tfsdk` tags) and `<type>FromModel`/`modelFrom<Type>` converter functions.

### The Nagios API quirks `internal/client` encodes

These are real behaviors of the live API (verified against a running instance), not stylistic choices:

1. **Every response is HTTP 200, even on failure.** Success/error is only knowable by parsing the JSON body for a `success` or `error` key — see `response.go`'s `parseCommandResponse`, the single choke point for this.
2. **Every mutating call must be followed by `POST .../system/applyconfig`** or the write never takes effect. Each `NewX`/`UpdateX`/`DeleteX` does this itself; don't factor it out of those methods.
3. **PUT (rename) addresses the *old* name as a URL path segment**, not the new one — see `url.go`'s `buildURL`.
4. **`service` has a verb-specific compound key**: `GetService`/`UpdateService` key off `(config_name, description)`; `DeleteService` keys off `(host_name, description)` instead, where `host_name` is the full host set comma-joined into one value. This is intentional and documented in `service.go` — don't "fix" it into consistency.
5. **`authserver` DELETE uses a `/{id}` path segment**, unlike every other object type's query-param style.
6. **`authserver`'s create response only contains `{"success", "server_id"}`**, not the full object — `NewAuthServer` unmarshals that response directly into the same `*AuthServer` the caller passed in (which already has the other fields populated), rather than doing a separate GET.
7. **`authserver` has no update route at all** — `PUT system/authserver` always returns `"Unknown API endpoint."` (confirmed against a live instance, #104), not a bug in this client. `client.UpdateAuthServer` still exists (the `resource.Resource` interface requires an `Update` method), but every attribute in `resource_authserver.go`'s schema — including `enabled` — is `RequiresReplace`, so it's unreachable in practice; Terraform always plans a destroy+recreate instead.
8. **`free_variables` (custom `_`-prefixed macros) come back as dynamic top-level keys on the object itself**, not nested under a `free_variables` key — `params.go`'s `extractFreeVariables` picks these out of the raw JSON by prefix.
9. **`GetX` must return `(nil, nil)` for zero results**, never a non-nil empty struct — this was a real, previously-shipped bug (every `getX` in the pre-framework version of this provider returned a non-nil struct on not-found, which silently broke Terraform's state-clearing logic). Any new `GetX` must follow the existing pattern of checking `len(results) == 0`.
10. **`client.RetryUntilFound`** (`retry.go`) tolerates Nagios's own eventual-consistency window right after a write. Only call it from a resource's `Create`, immediately after the write — never from plain `Read`, which should reflect external deletes without a multi-second stall on every refresh.
11. **The `existsErrorFor` fallback**: if a PUT fails with `"Does the X exist?"`, `UpdateX` falls back to calling `NewX` (handles rename-after-manual-delete). All 7 object types have this — except `authserver`, whose PUT failure (`"Unknown API endpoint."`) never matches this pattern anyway, and `hostgroup`/`servicegroup`, whose server-side error text doesn't match `existsErrorFor`'s exact string either (see `projects/PROJ-001-nagios-provider-revival/research/api-quirks-reverification.md`).

### Resource pattern (`internal/provider`)

Every resource follows the same shape (see `resource_host.go` as the reference — largest schema, exercises sets/maps/bools):

- `Create`: model → client struct → `NewX` → `client.RetryUntilFound` wrapping `GetX` → populate state.
- `Read`: `GetX` (no retry) → `nil` result calls `resp.State.RemoveResource(ctx)`.
- `Update`: `state.Name.ValueString()` is the *old* identifier directly from prior state — no need for SDKv1-style `GetChange` dances.
- `Delete`: `DeleteX`.

Shared conversion helpers live in `convert.go` (`setToStrings`/`stringsToSet` for `types.Set`↔`[]string`, `mapToStrings`/`stringsMapToMap` for `free_variables`, `optionalBoolToNagios`/`nagiosToOptionalBool` for the null-vs-false distinction, `stringOrNull`). Reuse these rather than re-deriving per resource.

**Optional bools must go through `optionalBoolToNagios`**, which checks `IsNull()`/`IsUnknown()` before converting — never call `.ValueBool()` directly on an optional field when building a client struct. Reading Go's zero-value for an unset optional silently sends Nagios `"0"`, indistinguishable from an explicit `false`; this was the exact bug that motivated fixing bool handling during the framework migration.

**Terraform attribute names can't start with a digit.** Nagios's own field names `2d_coords`/`3d_coords` (host) are exposed to HCL as `coords_2d`/`coords_3d` — the wire-level JSON tag in `client.Host` stays `2d_coords`/`3d_coords`; only the `tfsdk` tag and schema key differ. This is enforced by the real `terraform` binary at plan time, not just a style preference.

Data sources reuse the resource's `modelFromX` converter directly (same package, unexported) rather than duplicating field-mapping logic.
