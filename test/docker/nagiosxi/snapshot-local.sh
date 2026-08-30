#!/usr/bin/env bash
set -euo pipefail

# Commits a fully-installed nagiosxi container to a local-only image tag, so
# a later `docker compose up` can boot from post-install state instead of
# repeating the ~10-50 minute `fullinstall` run. See the "Speeding up local
# runs from a post-install snapshot" section in README.md.
#
# Unlike build-and-push.sh's image, this tag is NEVER pushed to any
# registry: fullinstall bakes live secrets into the container filesystem
# (the nagiosxi MySQL user's password, config.inc.php's DB credentials, the
# nagiosadmin API key) that would be byte-identical across every container
# started from a shared image. Keeping this local-only keeps that exposure
# limited to the one machine that ran the install.
#
# Usage: ./snapshot-local.sh [compose service name, default: nagiosxi]

SERVICE="${1:-nagiosxi}"
LOCAL_TAG="nagiosxi-postinstall:local"
# Every service fullinstall enables on el9 (internal/../nagiosxi-install.service's
# 15-chkconfigalldaemons: nagios npcd ntpd mysqld crond httpd sshd, plus
# php-fpm/postfix) except sshd, which nagiosxi-install.service already
# disables permanently after install.
STATEFUL_SERVICES="nagios npcd crond httpd php-fpm postfix mysqld"

cd "$(dirname "$0")"

CONTAINER=$(docker compose ps -q "$SERVICE")
if [ -z "$CONTAINER" ]; then
    echo "No running '$SERVICE' container - start one first with 'docker compose up -d'." >&2
    exit 1
fi

if ! docker exec "$CONTAINER" test -f /var/lib/nagiosxi/.installed; then
    echo "fullinstall hasn't finished yet on this container (no /var/lib/nagiosxi/.installed)." >&2
    echo "Tail progress with: docker compose exec $SERVICE journalctl -u nagiosxi-install.service -f" >&2
    exit 1
fi

# `docker commit` only freezes the container's processes for the duration of
# the snapshot, it doesn't flush them - committing mysqld live risks capturing
# an unclean InnoDB shutdown (stale mysqld.pid, crash-recovery on next boot),
# so stop everything stateful first and restart it after, regardless of
# whether the commit succeeds, to leave this container serving normally again.
# shellcheck disable=SC2086
docker exec "$CONTAINER" systemctl stop $STATEFUL_SERVICES
# shellcheck disable=SC2086
trap 'docker exec "$CONTAINER" systemctl start $STATEFUL_SERVICES' EXIT

docker commit "$CONTAINER" "$LOCAL_TAG"

echo "Committed $LOCAL_TAG from container $CONTAINER"
echo "Boot a fresh, already-installed instance with:"
echo "  NAGIOSXI_IMAGE=$LOCAL_TAG docker compose up -d --force-recreate"
echo "Do not push $LOCAL_TAG to any registry - see the comment at the top of this script."
