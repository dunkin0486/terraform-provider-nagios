## Summary

<!-- What does this PR change, and why? -->

## Type of change

- [ ] Bug fix
- [ ] New resource / data source
- [ ] New attribute on an existing resource / data source
- [ ] Behavior change to an existing resource / data source (call this out explicitly below - this provider talks to a real external API with some real quirks, so anything that looks like a "fix" may be intentional; see CLAUDE.md)
- [ ] Docs / examples only
- [ ] Internal / tooling (CI, dependencies, refactor with no behavior change)

## Testing

- [ ] `go build ./...` and `go vet ./...` pass
- [ ] `go test ./internal/client/...` passes
- [ ] Acceptance tests pass against a live Nagios XI instance (`TF_ACC=1 go test -v -count=1 ./internal/provider/...` - see CONTRIBUTING.md for the Docker setup)
  - [ ] New/changed resources or data sources have `TestAcc*Basic` / `*CreateAfterManualDestroy` / `*UpdateName` coverage following the existing pattern

## Docs

- [ ] Schema changes are reflected in `Description` fields, and `tfplugindocs generate` has been run (no hand-edited files under `docs/`)

## Related issues

<!-- e.g. Closes #123, relates to #86 -->
