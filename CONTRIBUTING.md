# Contributing

## Requirements

- Go 1.25+
- [Terraform](https://developer.hashicorp.com/terraform/install) (for acceptance tests, which drive the real `terraform` binary against the compiled provider)
- Docker, for the local Nagios XI test instance

## Development

```bash
go build ./...
go vet ./...
gofmt -l -w internal/ main.go
golangci-lint run ./...
```

Run these before opening a PR. `go build`/`go vet`/`golangci-lint` are also enforced in CI (`.github/workflows/test.yml`).

### Pre-commit hooks (optional but recommended)

A [pre-commit](https://pre-commit.com) config is provided to catch formatting/vet/lint issues at commit time instead of in CI:

```bash
pip install pre-commit   # or: brew install pre-commit
pre-commit install       # runs the hooks automatically on every commit from then on
```

This runs `gofmt`, `go vet`, and `golangci-lint` on changed files, plus basic file hygiene (trailing whitespace, end-of-file newlines). It does not run `go build` or any tests — those still happen in CI and before a PR (see below).

## Testing

Unit tests (`internal/client`) need nothing external:

```bash
go test ./internal/client/...
```

Acceptance tests (`internal/provider`) exercise the full provider through the real Terraform binary against a live Nagios XI instance. Boot one locally with the Docker Compose setup in `test/docker/nagiosxi/`:

```bash
cd test/docker/nagiosxi
docker compose up --build -d   # first boot compiles Nagios core from source, ~50 minutes
eval "$(./get-api-token.sh)"   # sets API_TOKEN / NAGIOS_URL for the run below

cd ../../..
TF_ACC=1 go test -v -count=1 ./internal/provider/...
```

`get-api-token.sh` must be run from `test/docker/nagiosxi/` (it relies on `docker compose` finding the compose file via the working directory). See `test/docker/nagiosxi/README.md` for details on the test instance itself.

Acceptance tests create and destroy real objects in Nagios. They're safe to run repeatedly against the same instance — each test generates randomized names and cleans up after itself (`CheckDestroy`).

## Adding or changing a resource

Every resource has a client-side file in `internal/client/` and a provider-side file in `internal/provider/`. See `CLAUDE.md` for the API quirks the client layer has to account for, and the conversion helpers in `internal/provider/convert.go` that both layers rely on — reuse those rather than re-deriving field-by-field mapping logic per resource.

After changing a resource's schema (adding/removing/redescribing an attribute), regenerate docs:

```bash
tfplugindocs generate
```

Docs are generated from schema `Description` fields and the example `.tf` files in `examples/` — don't hand-edit `docs/*.md`.

## Pull requests

- **Run the acceptance test suite before opening a PR** (see Testing above) if your change touches `internal/client` or `internal/provider` at all. CI only runs unit tests and lint - it does not boot the Docker Nagios instance or set `TF_ACC`, so acceptance failures will not be caught automatically. This is not optional for resource/client changes: several real bugs in this provider (the `getX`-never-returns-nil bug, the `free_variables` round-trip bug, the `2d_coords`/`3d_coords` invalid-attribute-name bug) were only ever caught by actually running the suite against a live instance, not by build/vet/unit tests.
- Keep the PR focused; call out any behavior changes (not just refactors) explicitly in the description, since this provider talks to a real external system with quirks that are easy to accidentally "fix" into breakage.
- Include acceptance test coverage for new resources/data sources, following the existing `TestAcc<Type>Basic` / `TestAcc<Type>CreateAfterManualDestroy` / `TestAcc<Type>UpdateName` pattern.
- Fill out the PR template's Testing checklist honestly - it exists specifically to make sure the acceptance suite was actually run, not skipped.
