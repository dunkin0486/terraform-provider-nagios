# Nagios Network Analyzer test instance

Spins up a real Nagios Network Analyzer (NNA) instance in Docker, the same way `test/docker/nagiosxi/` does for XI - for the future `nna_*` resources tracked in #152-#162. Nagios Enterprises does not publish an official NNA container image (VM/OVA appliance or manual install onto a bare OS only), so this wraps NNA's own installer tarball.

## Status: verified

Build- and boot-tested end to end against two independent `docker compose up --build` runs, including a full `fullinstall` run and a live, unauthenticated trial activation through the same `POST /api/v1/install` path a real setup would use. See "Fixes applied" below if you're diagnosing a failure on a different host/Docker setup.

```
cd test/docker/nagios-network-analyzer
docker compose up --build -d
# tail install progress:
docker compose exec nagiosna journalctl -u nna-install.service -f
# wait for `docker compose ps` to report "healthy", then:
eval "$(./install-and-get-token.sh)"   # sets API_TOKEN and NNA_URL
curl "$NNA_URL/api/v1/license"
```

First boot took a few minutes in testing - much faster than `nagiosxi`'s ~50 minutes, since NNA's installer is package installs plus a Laravel app deploy, not a from-source compile of Nagios core/plugins/MRTG.

## Licensing: no perpetual free mode, but no cost either

Unlike XI, NNA has no perpetual unlicensed "free mode" - only a 30-day trial or a $4,995 one-time perpetual license (see #176 for the full investigation). This was the actual blocker question for this harness's existence:

**Confirmed live**: a fresh install's `"type": "trial"` activates entirely locally - no license key, activation key, or `client_id` required in the request, and `GET /api/v1/license` immediately reports `"activated": true` with a full 30-day countdown. Built and ran **two independent fresh containers** back to back; both got their own full, unblocked trial (`trial_seconds_left: 2592000`, exactly 30×24×3600 - a genuinely fresh window each time, not a shared/decrementing counter). No phone-home check ties trial eligibility to this host/network. This harness stays viable indefinitely without a paid license, the same way `test/docker/nagiosxi/` never needed one.

## Retrieving an API token

Unlike `nagiosxi`'s `fullinstall` (which seeds an admin account and API key that then need a separate "enable API access" step - see `test/docker/nagiosxi/get-api-token.sh`), NNA's `fullinstall` does **not** create an admin user or token at all. That only happens via the web install wizard's final step, `POST /api/v1/install` - `install-and-get-token.sh` drives that call directly and hands back the Sanctum API token from its response, along with the matching `NNA_URL`. This can only succeed once per container; re-running it after a successful install fails loudly rather than silently returning a stale/wrong token - see the script's own comments.

## Speeding up local runs with a prebuilt image

`docker-compose.yml` names a private GHCR package (`ghcr.io/dunkin0486/terraform-provider-nagios-test-nna`) so `docker compose up` can pull a prebuilt image instead of rebuilding - see `test/docker/nagiosxi/README.md`'s "Speeding up local runs with a prebuilt image" for the one-time setup steps (PAT, `docker login ghcr.io`, `./build-and-push.sh`, then confirming the package is **Private**). Same mechanics here; the only difference is the package name. NNA's own build is a few minutes rather than nagiosxi's ~50, so this matters less here, but it's still one less thing to wait on when iterating on `nna_*` resources.

## Fixes applied (in case your environment hits something different)

The Dockerfile/service unit were written by adapting `test/docker/nagiosxi/`'s already-proven Rocky Linux 9 base and workarounds to NNA's own installer (`fullinstall`, `libinstall.sh`), then actually running it. Two of the five issues found are identical to XI's own documented fixes; three are new to NNA:

- Same OS-detection gap as XI: `libinstall.sh`'s `set_os_info()` only checks for `centos-release`/`centos-stream-release`/`centos-linux-release` RPM names, never `rocky-release` - patched to treat `rocky-release` as CentOS-equivalent, same as `test/docker/nagiosxi/Dockerfile`.
- Same `php-fpm` ACL issue as XI: `listen.acl_users` needs POSIX ACL support the `/run` tmpfs doesn't have, failing with `ENOTSUP` - same drop-in fix (comment out `listen.acl_users`, set `listen.owner`/`listen.group` explicitly).
- **New**: NNA's `prereqs()` package list includes `supervisor`, which only exists in EPEL - and NNA's installer only auto-enables EPEL for `RedHatEnterpriseServer`+`el10`, never for a Rocky/CentOS-identified system. Installed `epel-release` directly.
- **New**: `hostname` isn't in the `ubi-init` minimal base image. `fullinstall`'s `nagiosna()` step shells out to `hostname -I` to build the app's `APP_URL` config value; without the binary, that silently becomes an empty string, `APP_URL` ends up as the bare scheme `http://`, and the later `artisan` step hard-fails with Symfony's "Invalid URI" trying to parse it.
- **New**: `fullinstall`'s `selinux()` step runs `semanage`/`restorecon`/`setsebool` unconditionally on any non-Debian/Ubuntu distro, but this container has no live SELinux policy store at all (no `/sys/fs/selinux`) - fails with "SELinux policy is not managed or store cannot be accessed." Gated that function's body on `/sys/fs/selinux` actually existing (scoped via `sed` address range to only that function - the unrelated `firewall()` step has the identical guard condition text for a different purpose, and already works correctly here).

One more oddity seen but not fixed in the Dockerfile, since it didn't block anything: the install wizard's `change_timezone.sh` call (invoked via `sudo` from within PHP) intermittently hit Laravel's 60-second subprocess timeout on the first of two test runs, despite the script itself running instantly when invoked directly (confirmed the `apache` user's `sudoers` grant and group membership are both correct). Didn't reproduce on the second full run. If this shows up reliably once real acceptance tests exist, worth a closer look at how Laravel's `Process` component invokes `sudo` in this environment - but it's unrelated to licensing/trial activation and didn't stop `POST /api/v1/install` from completing successfully either time.

## Licensing note

Do not push the built image to a public registry - check Nagios's license/EULA terms first. Build/run it locally, in private CI, or via the private GHCR package above (must stay **Private**) only.
