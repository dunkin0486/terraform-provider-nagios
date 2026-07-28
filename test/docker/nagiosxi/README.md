# Nagios XI test instance

Spins up a real Nagios XI instance in Docker for acceptance testing this provider, since Nagios Enterprises does not publish an official Nagios XI image. It wraps Nagios's own installer tarball and runs XI unlicensed, in "free mode" (7 hosts, unlimited services, no license key needed) - enough for CRUD acceptance tests.

## Status: verified

Build- and boot-tested end to end against a real `docker compose up --build`, including a full `fullinstall` run and a live authenticated call through the same API path this provider's client uses (`GET /api/v1/objects/host?apikey=...` over plain HTTP). Getting there required several fixes on top of the original installer-script reading - see "Fixes applied" below if you're diagnosing a failure on a different host/Docker setup.

```
cd test/docker/nagiosxi
docker compose up --build -d
# tail install progress:
docker compose exec nagiosxi journalctl -u nagiosxi-install.service -f
# wait for `docker compose ps` to report "healthy", then:
eval "$(./get-api-token.sh)"   # sets API_TOKEN and NAGIOS_URL
curl "$NAGIOS_URL/api/v1/objects/host?apikey=$API_TOKEN"
```

The first boot took **~50 minutes** in testing (`fullinstall` compiles MRTG, Nagios core, the monitoring plugins, and several PHP/Perl modules from source) - budget well beyond the installer docs' own "10-15+ minutes" estimate.

## Acceptance test suite

The full suite (`TF_ACC=1 go test ./nagios`, with `API_TOKEN`/`NAGIOS_URL` from `get-api-token.sh`) has been run against this container end to end and passes (24/24). Traffic was confirmed live against the container (not cached) via the httpd access log - real `POST`/`GET`/`DELETE` calls to `/nagiosxi/api/v1/config/*`.

One occasional flake seen: `TestAccHostgroupCreateAfterManualDestroy` failing with "Provider produced inconsistent result after apply" on `nagios_host.host`. It didn't reproduce on isolated reruns or a repeat full run, and no host was left dangling afterward - most likely Nagios XI's own config-apply pipeline hasn't fully settled when `TestAccHostgroupBasic` (which runs immediately before it) and this test both use the same fixed `host_name = "test1"` in quick succession. Not caused by anything in this Docker setup, but worth knowing about if it turns up in CI.

## Retrieving an API token

`fullinstall` gets XI installed but does not create or enable an API key - historically that happens via a web-based setup wizard on first login. Turns out that's a red herring for scripting purposes: `fullinstall`'s own DB import already seeds an `api_key` value for the default `nagiosadmin` account in the `xi_users` table, it's just sitting there with `api_enabled=0`. No wizard-scraping needed - `get-api-token.sh` in this directory flips that flag on and prints the key plus the matching `NAGIOS_URL` (the DB credentials themselves are generated fresh per install, so it reads them out of XI's own `config.inc.php` rather than assuming fixed values).

## Fixes applied (in case your environment hits something different)

The original Dockerfile/service unit were written from reading Nagios's installer scripts (`fullinstall`, `functions.sh`, `init.sh`, `get-os-info`) without a container runtime available to test against. Running it for real surfaced these issues, all now fixed in `Dockerfile` / `nagiosxi-install.service`:

- `rockylinux/rockylinux:9-init` isn't a published tag - Rocky ships it as `9-ubi-init`.
- The base image's `curl-minimal` conflicts with plain `curl` - needs `--allowerasing`.
- The installer's OS detection (`get-os-info`) doesn't recognize Rocky at all (only checks for CentOS/RHEL/Oracle release RPM names), so it fails before ever reaching the already-supported el9 code path. Patched to treat `rocky-release` as CentOS-equivalent.
- The Nagios XI dependency RPM needs `perl-Params-Validate` and `python3-rrdtool`, which live in Rocky's CRB repo (disabled by default).
- `initscripts-service` (provides `service`, used by `init-mysql`), `cronie` (provides `/etc/cron.d`, needed by the mrtg subcomponent), and `openssh-server` (the daemon-enable step unconditionally enables `sshd.service`) are all missing from the minimal base image.
- `nagiosxi-install.service`'s `TimeoutStartSec=1800` (30 min) was too tight for the real from-source compile time - set to unlimited, since it's a one-shot install already guarded by `ConditionPathExists`.
- RHEL/Rocky's `php-fpm` RPM ships `listen.acl_users` in its default pool config, which requires POSIX ACL support the container's `/run` tmpfs doesn't have - it fails with `ENOTSUP`. Fixed via a `php-fpm.service` drop-in that comments that out and sets `listen.owner`/`listen.group = apache` instead (the ACL-free equivalent - without it, the socket falls back to `root:root` and httpd's `apache` user can't reach it).

## Licensing note

Do not push the built image to a public registry - check Nagios's license/EULA terms first. Build/run it locally or in private CI only.
