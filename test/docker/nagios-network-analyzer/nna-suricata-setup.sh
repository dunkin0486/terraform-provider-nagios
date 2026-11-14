#!/bin/bash
# Post-fullinstall Suricata wiring for the NNA test harness (#248).
#
# Suricata itself is compiled into the image by the Dockerfile. What can't
# happen at build time is everything that depends on NNA's own install
# having run first: the nna user / nnacmd group (created by fullinstall),
# and the NNA database. This mirrors the permissions block from NNA's own
# ui/src/components/suricata/SuricataInstall.tsx, then makes sure the
# suricata_config row and the suricata_rulesets seed rows exist, which
# NNA's UI normally does as a side effect of an operator clicking through.
#
# Runs as root from nna-suricata-setup.service. Idempotent.
set -euo pipefail

NNA_ROOT=/var/www/html/nagiosna

# Normalizes suricata.yaml's eve-log types into the shape NNA's own YAML
# service expects (same step as the UI install page); without it
# GET suricata/config 500s with 'Undefined array key "enabled"'.
python3 /usr/local/nagiosna/scripts/canonicalize_eve_types.py

chown -R nna:nnacmd /usr/local/var/log/suricata
chmod -R 770 /usr/local/var/log/suricata

mkdir -p /usr/local/var/lib/suricata/update/sources
chown -R nna:nnacmd /usr/local/var/lib/suricata
chmod -R 2775 /usr/local/var/lib/suricata
setfacl -m d:g:nnacmd:rwx /usr/local/var/lib/suricata
setfacl -m g:nnacmd:rwx /usr/local/var/lib/suricata

mkdir -p /usr/local/var/run/suricata
chown nna:nnacmd /usr/local/var/run/suricata

chown -R root:nnacmd /usr/local/etc/suricata
chmod -R 770 /usr/local/etc/suricata
chmod 640 /usr/local/etc/suricata/*.config

# Only the packet-capture capability Suricata itself would need; this
# harness never runs Suricata against live traffic.
setcap cap_net_admin,cap_net_raw=eip /usr/local/bin/suricata || true

# Make sure the suricata_config row exists (NNA lazily creates it on first
# request in some paths, but not reliably before one) and the ruleset catalog
# is seeded from the suricata-update index the Dockerfile already downloaded.
# evelog_name is nullable but should match suricata.yaml's eve-log filename
# (eve.json, Suricata's default).
# Run as the web user so any files Laravel writes (logs, cache) aren't left
# root-owned and unwritable for php-fpm.
cd "$NNA_ROOT"
# apache's home (/usr/share/httpd) isn't writable, and tinker (psysh) insists
# on writing config/cache there - point it at a throwaway dir instead.
export HOME=/tmp XDG_CONFIG_HOME=/tmp/.config XDG_CACHE_HOME=/tmp/.cache XDG_DATA_HOME=/tmp/.local
run_artisan() { runuser -u apache -- php artisan "$@"; }
run_artisan db:seed --class='Database\Seeders\SuricataRulesetSeeder' --force
# tinker can exit 0 even when the PHP it runs throws, so confirm the write
# landed by checking for an explicit marker rather than trusting the exit code.
out=$(run_artisan tinker --execute='$c = App\Models\SuricataConfig::firstOrCreate([]); $c->evelog_name = $c->evelog_name ?: "eve.json"; $c->rulesets_seeded = 1; $c->save(); echo "SURICATA_CONFIG_OK";' )
echo "$out"
grep -q SURICATA_CONFIG_OK <<<"$out"
