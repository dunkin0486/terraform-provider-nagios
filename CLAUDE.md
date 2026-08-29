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

### Coverage goal

Codecov gates PRs on `internal/client` coverage, but only reports (doesn't gate) on `internal/provider` — the split, not a blanket rule, is intentional (`codecov.yml`):

- **`internal/client`** is exercised entirely by unit tests in CI (`httptest`, no external dependency), so the number Codecov sees is the real number. Both `project.client` and `patch.client` use `target: auto`/`threshold: 0.5%` — a ratchet against the PR's base commit, not a fixed floor: a PR fails only if it drops `internal/client` coverage (project-wide or on its own new/changed lines) below what it was before, never for landing under some fixed percentage regardless of baseline.
- **`internal/provider`** stays `informational: true` for both `project` and `patch`. It's primarily verified by `TF_ACC=1` acceptance tests, which never run in CI (see below) and so never count toward Codecov's number — gating on it would either block PRs that are properly acceptance-tested but have modest *unit* coverage, or incentivize padding with low-value unit tests just to satisfy the gate.

Treat 80% unit coverage (`go test ./internal/client/... ./internal/provider/... -cover`) as the goal on `internal/provider` too, even though nothing enforces it there — new provider logic should still come with the unit tests the TDD workflow below already asks for. Baseline at the time this was written: `internal/client` ~57%, `internal/provider` ~39%.

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

**Adding a new resource or data source also means updating `README.md`'s "Resources and data sources" list** — that list is hand-written (unlike `docs/*.md`) and has gone stale before (missing `nagios_timeperiod`/`nagios_command` and four data sources until a dedicated cleanup PR caught it). Update it in the same PR that adds the resource/data source, not as a follow-up.

### Commit messages and releases

Releases are automated via [release-please](https://github.com/googleapis/release-please) (`.github/workflows/release-please.yml`): it parses [Conventional Commits](https://www.conventionalcommits.org/) prefixes on `main` to decide the next version and drive `CHANGELOG.md`. **PR titles/merge commits need a `fix:`/`feat:`/`feat!:` (or `BREAKING CHANGE:` footer) prefix** or they won't be picked up as release-worthy — see CONTRIBUTING.md for the full mapping. Don't hand-edit `CHANGELOG.md`; release-please owns it now.

### Git workflow

`main` gets merged (and released) frequently by work happening outside any one session — release-please alone lands a version-bump commit on every merge. A local `main` branch is stale the moment it's not actively re-synced, and branching off a stale `main` produces a branch that's silently missing recent history (committed files disappear from the working tree on checkout, already-merged changes look like new diffs, etc.).

- **Every PR must have a linked GitHub issue before it merges — create the issue first, even for small or short-lived work.** File it (and link it to the "Nagios Provider Enhancements" project per the bullet below) before branching, then reference it in the PR body (`Closes #N`/`Refs #N`) so the link is in place from the PR's first open, not added retroactively. This applies even to work you intend to close out immediately after the PR merges — the issue is what leaves a permanent record of the motivation once both it and the PR are closed; skipping it because the change looks small is exactly the case this is meant to catch.
- **Do a new git worktree for each unit of work**, rather than reusing one working directory across unrelated branches. This is what actually eliminates the stale-base problem above: each worktree tracks its own branch against a freshly-fetched base, so switching between unrelated efforts (a feature branch, a docs fix, a CI tweak) can't cross-contaminate working-tree state the way repeated `git checkout -b` in one directory can.
- **Always fetch and fast-forward `main` before branching for new work** — `git fetch origin main && git branch -f main origin/main` (or an equivalent pull) — even if `main` was updated recently in this same session. Don't assume the local ref is current.
- **Link new issues to the "Nagios Provider Enhancements" project on creation** — `gh issue create --project "Nagios Provider Enhancements" ...` — for new resource-type work or fix tracking, rather than filing the issue and adding it to the board as a separate step (or not at all).
- **Run `/code-review` and `/security-review` against the working diff before opening a PR**, on every branch — not just ones touching `internal/client`/`internal/provider`. This is in addition to, not instead of, the full acceptance suite requirement above for provider-code changes. Fix or consciously accept each finding before pushing the PR, rather than opening it and iterating on review comments afterward.
- **Post both skills' findings to the PR as inline comments on the flagged line** once it's open (e.g. `/code-review --comment`, or an equivalent `gh api` line comment) — every finding from both runs, not just the ones left unfixed. This gives the PR a visible record of what was caught (and, for fixed findings, that it was caught pre-merge rather than never), instead of that trail only existing in the session that ran the review.

## Architecture

Two packages, deliberately separated by an import boundary:

- **`internal/client`** — a plain Go HTTP client for Nagios XI's REST API. Zero dependency on `terraform-plugin-framework`; unit-testable standalone. One file per object type (`host.go`, `service.go`, etc.), each exporting `NewX`/`GetX`/`UpdateX`/`DeleteX` on `*Client`.
- **`internal/provider`** — depends on both `internal/client` and `terraform-plugin-framework`. One file per resource/data source, each with a `<type>Model` struct (`tfsdk` tags) and `<type>FromModel`/`modelFrom<Type>` converter functions.

### The Nagios API quirks `internal/client` encodes

These are real behaviors of the live API (verified against a running instance), not stylistic choices:

1. **Every response is HTTP 200, even on failure.** Success/error is only knowable by parsing the JSON body for a `success` or `error` key — see `response.go`'s `parseCommandResponse`, the single choke point for this.
2. **Every mutating call must be followed by `POST .../system/applyconfig`** or the write never takes effect. Each `NewX`/`UpdateX`/`DeleteX` does this itself; don't factor it out of those methods.
3. **PUT (rename) addresses the *old* name as a URL path segment**, not the new one — see `url.go`'s `buildURL`.
4. **`service` keys each verb differently**: `GetService` keys off `config_name` alone (no description filter); `UpdateService` is compound-keyed off `(config_name, description)`; `DeleteService` keys off `(host_name, description)` instead, where `host_name` is the full host set comma-joined into one value. This is intentional and documented in `service.go` — don't "fix" it into consistency.
5. **`authserver` DELETE uses a `/{id}` path segment**, unlike every other object type's query-param style.
6. **`authserver`'s create response only contains `{"success", "server_id"}`**, not the full object — `NewAuthServer` unmarshals that response directly into the same `*AuthServer` the caller passed in (which already has the other fields populated), rather than doing a separate GET.
7. **`authserver` has no update route at all** — `PUT system/authserver` always returns `"Unknown API endpoint."` (confirmed against a live instance, #104), not a bug in this client. `client.UpdateAuthServer` still exists (the `resource.Resource` interface requires an `Update` method), but every attribute in `resource_authserver.go`'s schema — including `enabled` — is `RequiresReplace`, so it's unreachable in practice; Terraform always plans a destroy+recreate instead.
8. **`free_variables` (custom `_`-prefixed macros) come back as dynamic top-level keys on the object itself**, not nested under a `free_variables` key — `params.go`'s `extractFreeVariables` picks these out of the raw JSON by prefix.
9. **`GetX` must return `(nil, nil)` for zero results**, never a non-nil empty struct — this was a real, previously-shipped bug (every `getX` in the pre-framework version of this provider returned a non-nil struct on not-found, which silently broke Terraform's state-clearing logic). Any new `GetX` must follow the existing pattern of checking `len(results) == 0`.
10. **`client.RetryUntilFound`** (`retry.go`) tolerates Nagios's own eventual-consistency window right after a write. Only call it from a resource's `Create`, immediately after the write — never from plain `Read`, which should reflect external deletes without a multi-second stall on every refresh.
11. **The `existsErrorFor` fallback**: if a PUT fails with `"Does the X exist?"`, `UpdateX` falls back to calling `NewX` (handles rename-after-manual-delete). All 8 object types have this — except `authserver`, whose PUT failure (`"Unknown API endpoint."`) never matches this pattern anyway, and `hostgroup`/`servicegroup`, whose server-side error text doesn't match `existsErrorFor`'s exact string either (see `projects/PROJ-001-nagios-provider-revival/research/api-quirks-reverification.md`). `timeperiod` is unreachable for a third, distinct reason: its PUT response never parses as JSON at all (a stray PHP `print_r()` dump is prepended to the body — confirmed against a live instance), so `parseCommandResponse` fails before `existsErrorFor` ever gets a chance to inspect an error string, parseable or not. Unlike `authserver`'s and `hostgroup`/`servicegroup`'s clean-but-mismatched error text, there's no error string here to mismatch.
12. **`get()` doesn't route through `parseCommandResponse`** the way POST/PUT/DELETE do (see quirk 1) — in principle an error response could be indistinguishable from a genuine zero-result GET. Confirmed against a live instance (#89): an auth failure (bad/missing API key) returns a JSON *object* (`{"error":"..."}`), never an empty array, and `json.Unmarshal` into a `[]X` slice already fails loudly on that shape rather than silently decoding to zero results — so `GetX`'s `len(results)==0` check can't confuse the two for this verified case (see `TestGetHost_ErrorObjectResponseIsNotSilentlyTreatedAsNotFound`). `authserver` and `user` decode into a `{"records", "<type>":[...]}` envelope struct instead (quirks 6 and 14), which has no `error` field of its own — an `{"error":"..."}` body's stray key used to be silently ignored by `json.Unmarshal` rather than causing a decode failure, masking the same auth failure as a zero-result GET. Confirmed against a live instance (#189) that `system/authserver` and `system/user` return the identical `{"error":"..."}` shape host's GET does for both a bad and a missing API key — so this wasn't just a theoretical gap. `response.go`'s `envelopeError` closes it: `GetAuthServer`/`GetUser` check the raw body for a top-level `error` key before trusting an envelope unmarshal as authoritative (see `TestGetAuthServer_ErrorObjectResponseIsNotSilentlyTreatedAsNotFound` / `TestGetUser_ErrorObjectResponseIsNotSilentlyTreatedAsNotFound`). Any future envelope-shaped `GetX` should call `envelopeError` the same way rather than trusting `len(Entries)==0` alone.
13. **Computed+Default bool/string fields going `null` on an empty GET response is not reachable via this provider's own `Create`/`Update`** (also investigated for #89): `register` (host/service), `is_volatile`/`active_checks_enabled`/`passive_checks_enabled` (service), and `enabled`/`security_level` (authserver) are all `Computed`+`Default` in their schemas, so `terraform-plugin-framework` already has a concrete (non-null) planned value before `Create`/`Update` ever runs, and `optionalBoolToNagios`/`m.X.ValueString()` always sends it explicitly — confirmed against a live instance that Nagios echoes these back correctly when sent explicitly, even if the HCL config never set them. The risk is real only for a value that reaches Nagios *without* ever being explicitly sent by this client (e.g. an object created outside Terraform, then imported) — see quirk 12's sibling finding on `authserver.enabled` coming back as a bare JSON number in exactly that scenario. `user.enabled` is the same `Computed`+`Default` shape and gets the same `UnmarshalJSON` normalization as `authserver.enabled`, proactively, since it wasn't separately confirmed live either way for `user`.
14. **`user` (`system/user`) makes no `applyconfig` call at all**, unlike quirk 2's stated invariant — confirmed against a live instance (#174): XI-panel login/admin accounts are stored in Nagios's own app DB, not part of the monitoring core config `applyconfig` regenerates, and changes were visible immediately via GET across independent requests without it. `NewUser`/`UpdateUser`/`DeleteUser` in `user.go` are the only `NewX`/`UpdateX`/`DeleteX` trio that skip it entirely. Also unlike every other object type, `GetUser` can't filter server-side at all — Nagios silently ignores `system/user`'s own `username=` GET filter (only `user_id` filters), so `GetUser` fetches the full unfiltered user list and scans it client-side by username.
15. **`password`, `auth_level`, `force_pw_change`, `auth_type`, and `auth_server_id` on `user` are permanently write-only** — Nagios accepts them on create/update but never returns any of them from a GET under any field name (#174), unlike every other object type's fields, which all round-trip. `resource_user.go`'s `modelFromUser` deliberately never assigns these five fields, leaving whatever `Create`/`Update` last wrote in state untouched on every `Read` — a one-way apply with no drift detection, not the usual `GetX`-populates-everything contract every other resource follows. `nagios_user` import can never populate these five attributes; they read back as unknown until set explicitly in configuration. Also unlike every other `UpdateX`'s PUT, `UpdateUser`'s PUT parameters (including `password`) go entirely through the URL query string per this client's usual PUT convention (`buildURL` + `setURLParams(...).Encode()`, no body) — `client.go`'s `redactSensitiveParams` (not just `apikey` as before `user` existed) keeps that plaintext password out of any transport-error diagnostic. That still leaves the password traveling in plaintext over the wire to Nagios XI itself on every PUT that includes it (a real risk for anything logging full request URIs, e.g. an access log or intermediate proxy) - `resource_user.go`'s `Update` mitigates this by clearing `u.Password` (which `setURLParams` then omits entirely) whenever the plan's password matches the prior state's, so an unrelated field edit no longer resends the real password; it's still sent on an apply that actually rotates it, which is unavoidable given Nagios's PUT only accepts parameters this way.

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
