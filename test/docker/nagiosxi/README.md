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

## Speeding up local runs with a prebuilt image

`docker build` itself is fast - the `~50 minutes` is `fullinstall` running at container *boot*, not part of the image build (see `Dockerfile`'s and `nagiosxi-install.service`'s comments). This prebuilt image only saves the build-time cost of re-extracting the installer tarball and pulling OS packages fresh on every machine/checkout - see the next section for actually skipping the `fullinstall` boot-time cost.

`docker-compose.yml` names a private GHCR package (`ghcr.io/dunkin0486/terraform-provider-nagios-test-nagiosxi`) so later runs can pull a prebuilt (pre-install) image instead of rebuilding it.

One-time setup:

1. Create a classic PAT with `write:packages` scope (add `read:packages` too if you'll pull from a different machine) at https://github.com/settings/tokens.
2. `echo "$GITHUB_TOKEN" | docker login ghcr.io -u dunkin0486 --password-stdin`
3. `./build-and-push.sh` — builds and pushes `:latest`.
4. Follow the script's printed link and confirm the package is set to **Private** — required by the licensing note below; GHCR doesn't reliably default a freshly-pushed package to private.

After that, plain `docker compose up -d` (no `--build`) pulls the image when you're logged in and a newer one exists, and transparently falls back to building locally (same as today) if you're logged out, on a machine without registry access, or before the first push. Re-run `./build-and-push.sh` whenever `Dockerfile` or `nagiosxi-install.service` changes — it's not wired into CI or any automatic trigger.

## Speeding up local runs from a post-install snapshot

The prebuilt image above still runs the full `fullinstall` on first boot (~10-50 minutes depending on host/network - measured ~10m26s on one machine, install CPU time alone was ~9m13s). For local dev iteration that wants a clean-slate instance without re-paying that cost every time (scoped in #168), `snapshot-local.sh` commits an already-installed container to a **local-only** image tag instead:

```
docker compose up -d                                    # normal boot, wait for install to finish
./snapshot-local.sh                                      # commits the running container to nagiosxi-postinstall:local
NAGIOSXI_IMAGE=nagiosxi-postinstall:local docker compose up -d --force-recreate   # ~10s to healthy, already installed
```

Since `docker-compose.yml` mounts no data volumes, each `--force-recreate` starts from the snapshot's exact post-install state again (any hosts/services created by a previous test run are discarded, not accumulated) - this is a clean slate on every boot, it just isn't a *fresh install* on every boot.

**This tag must never be pushed to any registry, including the private GHCR package above.** Unlike the pre-install image, `fullinstall` bakes live secrets into the filesystem - the `nagiosxi` MySQL user's password, `config.inc.php`'s DB credentials, the `nagiosadmin` API key - and every container started from a shared image would carry the exact same values (confirmed: hashing those credentials from two containers derived from the same snapshot produced identical hashes). Keeping the snapshot local-only keeps that exposure to the one machine that ran the install. Secret rotation on first boot from a snapshot (so it could be shared beyond one machine) is deferred - see #210.

Re-run `./snapshot-local.sh` whenever you want to refresh the snapshot (e.g. after `Dockerfile`/`nagiosxi-install.service` changes, or to capture a newer XI release) - like `build-and-push.sh`, it's a manual step, not wired into any automatic trigger.

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

Do not push the built image to a public registry - check Nagios's license/EULA terms first. Build/run it locally, in private CI, or via the private GHCR package above (must stay **Private**) only. The `nagiosxi-postinstall:local` snapshot from the section above is stricter still: never push it anywhere, even to that private package - see that section for why.
