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
```

Run these before opening a PR. `go vet` and a build are also enforced in CI (`.github/workflows/test.yml`).

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

- Keep the PR focused; call out any behavior changes (not just refactors) explicitly in the description, since this provider talks to a real external system with quirks that are easy to accidentally "fix" into breakage.
- Include acceptance test coverage for new resources/data sources, following the existing `TestAcc<Type>Basic` / `TestAcc<Type>CreateAfterManualDestroy` / `TestAcc<Type>UpdateName` pattern.
