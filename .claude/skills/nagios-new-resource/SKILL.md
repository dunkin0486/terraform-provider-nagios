---
name: nagios-new-resource
description: Add a brand-new nagios_* or nagios_nna_* resource or data source type (not a field on an existing one - see nagios-field for that). Use when asked to "add a resource for X", implement one of the tracked nna_* issues (#153-#162), or expose a Nagios XI/Network Analyzer object type this provider doesn't cover yet.
---

# nagios-new-resource: add a new resource or data source type

Encodes the "stand up a brand-new object type" loop from `CLAUDE.md`'s TDD workflow and git workflow, so the mechanical parts (worktree, scaffolding, docs, README) aren't re-derived by hand each time - only the API-specific investigation is genuinely new work per resource. Built after implementing #124 and while starting #153; see #179 (`nagios_nna_source`, the reference implementation for the NNA branch below) for a fully worked example.

**Scope**: a new object type with no existing `internal/client`/`internal/provider` coverage at all. Adding a field to a type that already has a resource is the `nagios-field` skill instead, not this skill.

This repo has two genuinely different API families - **pick the branch that matches which one you're adding to** before doing anything else. Don't blend their conventions; a new type only ever belongs to one family.

## 0. Git workflow (applies to both branches)

Per `CLAUDE.md`:
1. The issue must already exist and be linked to the "Nagios Provider Enhancements" project - the 11 `nna_*` issues (#152-#162) already are; for anything else, `gh issue create --project "Nagios Provider Enhancements" ...` first.
2. `git fetch origin main && git branch -f main origin/main`, then a **new worktree** for this issue: `git worktree add -b <type>/<issue>-<slug> ../terraform-provider-nagios-worktrees/<slug> main`.
3. Do the implementation (below), then run `/code-review` and `/security-review` against the working diff before opening the PR - fix or consciously accept each finding.
4. Open the PR with `Closes #<issue>` and a Conventional Commits title (`feat:` for a new resource/data source).
5. Post both skills' findings to the PR as inline comments once it's open, per `CLAUDE.md`.
6. **Remove the worktree once the PR has merged** (not merely opened - a review pass asking for changes needs to push more commits from it): `git worktree remove ../terraform-provider-nagios-worktrees/<slug>`. Worktrees left behind after merge are exactly how `terraform-provider-nagios-worktrees/` accumulates stale checkouts (confirmed - several sat around from already-merged PRs before this step was added).

## Running several of these in parallel

Each tracked `nna_*`/`nagios_*` resource issue is independent mechanical work - a natural fit for one background agent per issue (`Agent` tool, `isolation` not needed since step 0 already gives each its own worktree) when the user asks for several to be picked up together.

1. **Ask which model each agent should run as** - `AskUserQuestion` with the available choices, rather than defaulting silently. Per-agent model choice is a real cost/quality tradeoff across however many resources are being fanned out, worth the user's explicit call; this doesn't apply to the single-resource path above, only here.
2. Create one fresh worktree per issue (step 0.2 above, done up front for all of them) - each agent operates entirely inside its own worktree path, never touching another's.
3. **Bootstrap both the NNA and XI live instances once, outside any individual agent, and share the credentials** - `testAccPreCheck`/`testAccNNAPreCheck` (`provider_test.go`) both `t.Fatalf` (not skip) when their env vars are unset, so the full-suite gate (A5/B5) needs both regardless of which branch's resource is being added - don't read B0/A0 as "only boot the one matching my branch," even for an XI-only or NNA-only change. Don't let each parallel agent run its own `docker compose up`/install script against its own worktree's copy of the compose file, either - for NNA specifically this isn't just wasteful, it actively collides: `docker-compose.yml` binds fixed host ports (8081/8444) and Compose defaults the project name to the compose file's directory basename, which is identical (`nagios-network-analyzer`) across every worktree - concurrent `up` calls fight over the same project/ports regardless of which worktree invoked them. Boot both instances once from any one worktree, then hand every agent plain env values in its task prompt - explicitly tell each agent not to re-run the install/bootstrap steps itself. **Always hand out NNA's token as `NNA_API_KEY`, never as `API_TOKEN`** - `install-and-get-token.sh` only ever exports `API_TOKEN`, but `testAccNNAPreCheck` reads `NNA_API_KEY` specifically; this rename is required for every NNA-involving agent unconditionally (most of the currently-tracked issues are `nna_*`, so this is the common case, not an edge case that only shows up when XI is also in the mix).
   - If an instance from a prior session is already up (`docker compose ps` showing `healthy`, or an `Exited` container from a previous install), reuse or restart it (`docker compose start`) rather than rebuilding - skips the ~50 min XI install and the NNA install's one-time-only install call entirely. If the NNA install call has already been consumed (`"This action is unauthorized."` on a second `/api/v1/install`) and no token was saved, pull the existing admin's `apikey` directly from NNA's own database rather than via HTTP (NNA's `/api/v1/login` is session/CSRF-based, not a reliable path for a plain `curl`) - confirmed live:
     ```bash
     docker compose exec -T nagiosna cat /var/www/html/nagiosna/.env | grep '^DB_'   # DB_USERNAME/DB_PASSWORD for the nagiosna MySQL db
     docker compose exec -T nagiosna mysql -u <DB_USERNAME> -p'<DB_PASSWORD>' nagiosna -N \
       -e "SELECT apikey FROM users WHERE username='nagiosadmin';"
     ```
     This mirrors `get-api-token.sh`'s own approach for XI (read DB creds out of the app's own config, then query the key straight from its table) - NNA just keeps its DB creds in a Laravel `.env` instead of XI's `config.inc.php`.
   - If both XI and NNA are needed in the same shell to then hand credentials out to agents, mind a collision **not** currently documented anywhere else: `get-api-token.sh` and `install-and-get-token.sh` both name their output `API_TOKEN`, so `eval`ing both in sequence lets the second clobber the first. Capture XI's value under a different local variable immediately after its `eval` (e.g. `XI_API_TOKEN=$API_TOKEN`) before evaluating NNA's script - but that rename is only for your own bookkeeping while juggling both: still hand each agent its real required names for the tests it will actually run (an XI-testing agent needs `API_TOKEN`/`NAGIOS_URL`; an NNA-testing agent needs `NNA_API_KEY`/`NNA_URL`, per `testAccPreCheck`/`testAccNNAPreCheck` in `provider_test.go`), not the renamed variable. (Fixing this at the source - having the two scripts emit distinctly-named output - would remove the need for this callout entirely; left as a follow-up rather than done here, since it'd mean changing both scripts' output contracts, used by every single-instance workflow in this file too, not just the parallel case.)
4. Each agent does its own B1-B4 (or A1-A4) independently against the shared instance(s), up through implementation, unit tests, and its own **scoped** acceptance test (`-run TestAccNNA<Type>`/`-run TestAcc<NewType>`) - the object types are disjoint (e.g. `nna_check`/`nna_role`/`nna_nmap_profile` don't reference each other's rows), so concurrent exploration and scoped test runs are safe. Stop there and report back rather than proceeding to B5/A5.
5. **Fan in before the full acceptance suite gate (B5/A5) - don't run it concurrently.** It exercises every existing resource type against the shared instance(s), not just the new one, so parallel full-suite runs are a real source of cross-run contention (object creation/deletion racing across unrelated tests), not just slower responses. Once every agent reports its scoped step 4 green, run the full suite **sequentially, one branch at a time** (rebasing each onto the latest `main` first if an earlier one has already merged) before that branch's PR opens - whether the orchestrating session runs each in turn itself, or resumes each agent to do so once its turn comes up.

## Branch A: Nagios XI resource/data source

XI's per-object quirks are already exhaustively catalogued in `CLAUDE.md`'s "quirks `internal/client` encodes" list (always-200 responses, PUT-by-old-name, `free_variables`, etc.) - read that list before starting, since a new type will likely hit one or more of them. `host` (`internal/client/host.go`, `internal/provider/resource_host.go`) is the reference implementation: largest schema, exercises sets/maps/bools.

### A0. Bootstrap the live instance

```bash
cd test/docker/nagiosxi
docker compose ps            # confirm "healthy"
eval "$(./get-api-token.sh)" # sets API_TOKEN and NAGIOS_URL
```

First boot takes ~50 minutes if not already up - don't start this mid-task expecting a quick turnaround.

### A1. Confirm the object type's real shape against the live instance first

Before writing any code, hit the raw endpoint directly (`curl`, not through this client) to confirm:
- The actual field list, and which fields are required in practice vs. only in the docs/whitelist (see the `nagios-field` skill's step 0 - a field can be accepted on write and silently dropped on a column-less table).
- Whether it shares XI's standard conventions (always-200 body-parsed responses, PUT-by-old-name, `applyconfig` needed) or deviates like `authserver`/`user`/`timeperiod` do (`CLAUDE.md` quirks 5-7, 11, 14-15) - a new type is not guaranteed to behave like `host`.

### A2. Red: write the acceptance test first

Pattern-match the closest existing `resource_<type>_test.go` (a name-addressed CRUD type like `hostgroup` vs. an outlier like `authserver`). Follow CONTRIBUTING.md's naming convention: `TestAcc<Type>Basic`, `TestAcc<Type>CreateAfterManualDestroy`, `TestAcc<Type>UpdateName`. Confirm each is **red for the right reason** (missing schema attribute, unset state field) before implementing:

```bash
TF_ACC=1 go test -v -count=1 ./internal/provider/... -run TestAcc<NewType>
```

### A3. Implement: the touch-list

1. **`internal/client/<type>.go`** (new file) - `<Type>` struct with wire-format JSON tags, plus `New<Type>`/`Get<Type>`/`Update<Type>`/`Delete<Type>` on `*Client`, following the `NewX`/`GetX`/`UpdateX`/`DeleteX` shape every other XI type uses. `Get<Type>` must return `(nil, nil)` on zero results (quirk 9). Every mutating method calls `applyConfig` itself unless the type is confirmed to skip it like `user` (quirk 14).
2. **`internal/client/<type>_test.go`** - httptest-backed unit tests mirroring `host_test.go`'s shape (see `nagios-field`'s sibling notes, or #124's PR for four worked examples: `contact_test.go`, `contactgroup_test.go`, `hostgroup_test.go`, `servicegroup_test.go`).
3. **`internal/provider/resource_<type>.go`** (new file) - `<type>Model` struct, `Metadata`/`Schema`/`Configure`/`Create`/`Read`/`Update`/`Delete` following the pattern in "Resource pattern" in `CLAUDE.md`. Reuse `convert.go` helpers (`setToStrings`/`stringsToSet`, `mapToStrings`/`stringsMapToMap`, `optionalBoolToNagios`/`nagiosToOptionalBool`, `stringOrNull`) rather than re-deriving conversions. Watch for Terraform's digit-leading-attribute-name restriction (`CLAUDE.md`'s `2d_coords`→`coords_2d` note).
4. **`internal/provider/data_source_<type>.go`** (new file, only if a data source makes sense for this type) - reuses the resource's `modelFrom<Type>` converter directly (see `data_source_contact.go` for the reference shape: `Read` just calls the same `Get<Type>` + `modelFrom<Type>` the resource uses).
5. **`internal/provider/provider.go`** - register `New<Type>Resource` in `Resources()` and, if applicable, `New<Type>DataSource` in `DataSources()`.
6. **`internal/provider/resource_<type>_test.go`** (+ `data_source_<type>_test.go`) - the acceptance tests from A2.
7. **`examples/resources/nagios_<type>/resource.tf`** (+ `examples/data-sources/nagios_<type>/data-source.tf`) - minimal working example; `tfplugindocs` pulls these into the generated docs verbatim.

### A4. Green, format, docs, README

```bash
TF_ACC=1 go test -v -count=1 ./internal/provider/... -run TestAcc<NewType>
gofmt -l -w internal/ main.go
go vet ./...
golangci-lint run ./...
tfplugindocs generate --provider-name nagios --rendered-provider-name terraform-provider-nagios
```

The two `tfplugindocs` flags are **required** when generating from a worktree (which this always is, per step 0) - `tfplugindocs` infers the provider name from the cwd's directory basename by default, which is wrong for any worktree path. Omitting them either errors outright or silently rewrites every existing doc's `page_title`, polluting the diff. After running, `git status` on `docs/*.md` should show only the new type's doc files as untracked/new - zero changes to existing ones.

Update **`README.md`**'s "Resources and data sources" list in this same PR (it's hand-written, not generated, and has gone stale before) and `docs/index.md` if the provider-level schema changed.

### A5. Before opening the PR

```bash
TF_ACC=1 go test -v -count=1 ./internal/provider/...                      # full acceptance suite, not just the new test
go test ./internal/client/... ./internal/provider/... -cover              # check unit coverage didn't regress
```

## Branch B: Nagios Network Analyzer (`nna_*`) resource

NNA is a genuinely different API shape from XI - see `internal/client/nna/client.go`'s package doc: Bearer-token auth (not `?apikey=`), real JSON bodies (not form-encoded), meaningful HTTP status codes (not always-200), numeric-id addressing (not name/PUT-old-name), no `applyconfig` step. `internal/client/nna/source.go` + `internal/provider/resource_nna_source.go` (#152/#179) is the reference implementation - copy its shape, not XI's.

Each of the 9 remaining tracked issues (#154-#162) names its own endpoint (`POST/PUT/DELETE /api/v1/<plural-type>`) but explicitly flags the exact field list as **not yet confirmed against a live instance** - that confirmation is the genuinely new work per issue; nothing below can be templated around it.

### B0. Bootstrap the live instance

```bash
cd test/docker/nagios-network-analyzer
docker compose up --build -d   # first boot: a few minutes, much faster than nagiosxi's ~50
docker compose ps              # wait for "healthy"
eval "$(./install-and-get-token.sh)"   # sets API_TOKEN and NNA_URL - can only succeed once per container
curl "$NNA_URL/api/v1/license"          # sanity check
```

If Docker itself isn't running, `open -a Docker` and poll `docker info` until it responds before `compose up`.

### B1. Confirm the object type's real shape against the live instance first

Hit the raw endpoint with `curl -H "Authorization: Bearer $API_TOKEN"` before writing any Go. Confirm, and document as a doc comment on the new `Client`/struct methods the same way `source.go` does inline for each quirk found:
- The actual field list on create/update, and which are required in practice vs. only in validation messages (NNA's Laravel validator and its controller's raw array access don't always agree - `source.go`'s `Description`/`FlowType` fields document exactly this kind of gap).
- The **create response shape** - does it return the created object/id at all, or (like `sources`) only a bare `{"message","output"}` pair requiring a post-create list-and-match-by-name lookup via `client.RetryUntilFound`?
- The **update response shape** - bare object, or wrapped under a key like `sources`' `{"source": {...}}`?
- The **not-found shape** - confirm it's NNA's standard `404 {"message":"Resource not found for id: N"}` (`GetX` still returns `(nil, nil)` per `CLAUDE.md` quirk 9, just via a status-code check instead of an empty array).
- Whether any field can't be set through the create/update body at all and instead needs a dedicated action endpoint (`sources`' `is_active` via `/start`/`/stop` is the precedent - don't assume a group/config type won't have the same pattern for e.g. an enabled flag).
- Whether delete is idempotent on an already-gone id (confirmed true for `sources`; verify per type, don't assume).

### B2. Red: write the unit test first, then the acceptance test

Unlike XI resources, NNA's client package (`internal/client/nna`) has its own httptest-backed unit test file per type (`source_test.go`) in addition to acceptance tests - write both, unit test first since it's faster to iterate against a fake server once B1's real shape is confirmed. Mirror `source_test.go`'s coverage: auth header + JSON body shape, create's id-resolution behavior, a validation-error (422) case, get-found/not-found, update's response-unwrapping, delete idempotency, and any dedicated action endpoints.

```bash
go test ./internal/client/nna/... -run TestNew<Type> -v
TF_ACC=1 go test -v -count=1 ./internal/provider/... -run TestAccNNA<Type>
```

### B3. Implement: the touch-list

1. **`internal/client/nna/<type>.go`** (new file) - `<Type>` struct, `New<Type>`/`List<Type>s`/`Get<Type>`/`Update<Type>`/`Delete<Type>` on `*Client` per B1's confirmed shape. Reuse `idPath`, `isSuccess`, `parseError` from `client.go`/`response.go` rather than re-deriving.
2. **`internal/client/nna/<type>_test.go`** - the unit tests from B2.
3. **`internal/provider/resource_nna_<type>.go`** (new file) - mirrors `resource_nna_source.go`'s shape: `Configure` must check `pd.NNA == nil` and error with the same "Missing Nagios Network Analyzer Credentials" message (NNA credentials are optional at the provider level, unlike XI's). If the type has an id-addressed identity, `ImportState` needs the same `strconv.ParseInt` handling `resource_nna_source.go` uses (not `resource.ImportStatePassthroughID`, which doesn't type-convert).
4. **`internal/provider/provider.go`** - register `NewNNA<Type>Resource` in `Resources()`.
5. **`internal/provider/resource_nna_<type>_test.go`** - acceptance tests following `resource_nna_source_test.go`'s shape (`testAccNNAPreCheck`, `testAccNNAClient` helpers already exist in `provider_test.go` - reuse them).
6. **`examples/resources/nagios_nna_<type>/resource.tf`**.

### B4. Green, format, docs, README

Same as A4 - `gofmt`, `go vet`, `golangci-lint`, then:

```bash
tfplugindocs generate --provider-name nagios --rendered-provider-name terraform-provider-nagios
```

Update **`README.md`**'s "Resources and data sources" list in the same PR.

### B5. Before opening the PR

```bash
TF_ACC=1 go test -v -count=1 ./internal/provider/...
go test ./internal/client/... ./internal/provider/... -cover
```

If this is the 2nd or 3rd `nna_*` resource to land (i.e. a third file now duplicates the `pd.NNA == nil` credential-check block from step B3.3), check whether #180 (extract a shared NNA-credential-check helper) is now unblocked and worth doing alongside this PR rather than as a separate follow-up.
