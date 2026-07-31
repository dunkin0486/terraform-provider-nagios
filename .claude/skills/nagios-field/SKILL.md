---
name: nagios-field
description: Add a new field to an existing nagios_* resource (host, service, hostgroup, servicegroup, etc.) following this repo's TDD workflow. Use when asked to "add field X to resource Y", add a Nagios attribute to a resource/data source, or expose a new Nagios API parameter in the provider. Not for adding a whole new resource type - that's higher-variance, per #106.
---

# nagios-field: add a field to an existing resource

Encodes the "add a new field to an existing resource" loop from `CLAUDE.md`'s TDD workflow so it's followed the same way every time, instead of being re-derived by hand. Built while implementing #106 (5 fields across host/service/hostgroup/servicegroup) and #109 (`host.parents`) - see those PRs for worked examples of every step below.

**Scope**: field-level additions to an existing `internal/client`/`internal/provider` object type only. A brand-new resource type is out of scope for this skill - each new object type tends to surface its own API quirk (see `CLAUDE.md`'s quirks list), so it doesn't fit a rigid template and needs case-by-case treatment instead.

## 0. Before touching code: verify the field is real, not just whitelisted

A parameter appearing in Nagios XI's PHP `api_config_create_cfg_*`/`api_config_edit_cfg_*` whitelist is **not proof it's functional**. `service.parents` (#105) was accepted on write with `"success"` and no error, but silently dropped - `tbl_service` has no column to store it, unlike `tbl_host`. This was only caught by a live write-then-read round-trip; a static whitelist read said nothing about it.

Before implementing, confirm (against the live Docker instance, not by reading source):
1. The field can be written via a raw `POST`/`PUT` to the relevant `config/<type>` endpoint and the response reports success.
2. A subsequent `GET` on the same object actually returns the value you wrote.

If step 2 fails, stop and report it (like #105 did) rather than shipping a no-op field.

## 1. Bootstrap the live instance

```bash
cd test/docker/nagiosxi
docker compose ps            # confirm the nagiosxi service reports "healthy"
eval "$(./get-api-token.sh)" # sets API_TOKEN and NAGIOS_URL
```

`get-api-token.sh` finds the compose file via cwd - it must be run with `test/docker/nagiosxi/` as the working directory. If you can't `cd` there (e.g. running from the repo root), point it at the compose file instead of changing directory:

```bash
COMPOSE_FILE=test/docker/nagiosxi/docker-compose.yml ./test/docker/nagiosxi/get-api-token.sh
```

If the container isn't up yet, `docker compose up --build -d` from `test/docker/nagiosxi/` - first boot takes ~50 minutes (compiles Nagios core, MRTG, plugins from source), so don't start this mid-task expecting a quick turnaround.

## 2. Red: write the acceptance test first

Find the most similar existing field on the same resource in `internal/provider/resource_<type>_test.go` and pattern-match its acceptance test (a scalar string/bool field vs. a set/list field vs. a `free_variables`-style dynamic field all look different - copy the closest analogue, not just the nearest test in the file). The new test should assert:
- The field can be set on create and shows up in state.
- It round-trips correctly on read (`terraform plan` produces no diff after apply).
- Updating it in place is reflected on the next apply.

Run just that test against the live instance:

```bash
TF_ACC=1 go test -v -count=1 ./internal/provider/... -run TestAcc<NewTestName>
```

Confirm it's **red for the right reason** - a missing schema attribute or an unset field in state, not a typo or config error in the test itself. A red test that fails for the wrong reason proves nothing (per `CLAUDE.md`).

## 3. Implement: the touch-list

For a field on object type `<type>` (host, service, hostgroup, servicegroup, ...), the mechanical change touches:

1. **`internal/client/<type>.go`** - add the field to the `<Type>` struct with its Nagios wire-format JSON tag (e.g. Nagios's `2d_coords` stays `2d_coords` in the JSON tag even though the Terraform-facing name differs - see step 3 below).
2. **`internal/provider/resource_<type>.go`**:
   - `<type>Model` struct: add the `tfsdk`-tagged model field.
   - Schema: add the attribute definition (with a `Description`, since docs are generated from it - never hand-write `docs/*.md`).
   - `<type>FromModel`: convert the new model field into the client struct field.
   - `modelFrom<Type>`: convert the new client struct field back into the model field.
3. **`internal/provider/data_source_<type>.go`** - add the matching schema attribute, if a data source exists for this type. Data sources reuse the resource's `modelFrom<Type>` converter directly, so no separate conversion logic is needed here - just the schema entry.
4. **`internal/provider/resource_<type>_test.go`** - the acceptance test written in step 2.

Reuse the existing helpers in `convert.go` rather than re-deriving conversions per field:
- `setToStrings`/`stringsToSet` for `types.Set` <-> `[]string` (e.g. `parents`, group membership lists).
- `mapToStrings`/`stringsMapToMap` for `free_variables`-style maps.
- `optionalBoolToNagios`/`nagiosToOptionalBool` for optional bools - **always** go through this helper, never call `.ValueBool()` directly on an optional field. Reading Go's zero-value for an unset optional silently sends Nagios `"0"`, indistinguishable from an explicit `false`.
- `stringOrNull` for optional strings.

Two naming gotchas from `CLAUDE.md` to check before picking attribute names:
- Terraform attribute names can't start with a digit - Nagios's own `2d_coords`/`3d_coords` become `coords_2d`/`coords_3d` in the schema/`tfsdk` tag (the JSON wire tag stays as Nagios sends it).
- If the field is a dynamic `_`-prefixed macro rather than a fixed field, it belongs under the existing `free_variables` handling (`params.go`'s `extractFreeVariables`), not as a new named attribute.

## 4. Green, format, docs

```bash
TF_ACC=1 go test -v -count=1 ./internal/provider/... -run TestAcc<NewTestName>   # confirm green
gofmt -l -w internal/ main.go
go vet ./...
tfplugindocs generate   # regenerates docs/ from the Description field - never edit docs/*.md by hand
```

## 5. Before opening the PR

Run the **full** acceptance suite, not just the new test - CI doesn't do this (it only runs unit tests + lint; `TF_ACC` is never set in CI):

```bash
TF_ACC=1 go test -v -count=1 ./internal/provider/...
```

Every real bug in this provider's history (`getX`-never-nil, `free_variables` round-tripping, `2d_coords` naming, `service.parents`) was caught this way, not by `go build`/`go vet`/unit tests. Also check unit coverage on any client/provider package touched:

```bash
go test ./internal/client/... ./internal/provider/... -cover
```

Baseline to work toward: `internal/client` ~57%, `internal/provider` ~39% (not a hard gate - Codecov is informational - but treat it as a goal per `CLAUDE.md`).

Finally, give the PR title a Conventional Commits prefix (`feat:`, `fix:`, etc.) - release-please parses it to drive the changelog and version bump.
