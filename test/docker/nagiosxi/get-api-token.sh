#!/bin/bash
# Enables the API key for the default nagiosadmin account on a running
# nagiosxi test container and prints it, along with the NAGIOS_URL this
# provider's acceptance tests expect (see env/setup.go at the repo root).
#
# fullinstall generates the nagiosxi MySQL user's password randomly per
# install, and there's no CLI flag to hand back an API key directly (see
# README.md) - so this reads the DB credentials straight out of XI's own
# config.inc.php, then flips api_enabled on for nagiosadmin and reads back
# the api_key that fullinstall's DB import already seeded.
#
# Usage: ./get-api-token.sh [compose service name, default: nagiosxi]

set -euo pipefail

SERVICE="${1:-nagiosxi}"

DB_USER=$(docker compose exec -T "$SERVICE" php -r '$cfg=[]; include "/usr/local/nagiosxi/html/config.inc.php"; echo $cfg["db_info"]["nagiosxi"]["user"];')
DB_PASS=$(docker compose exec -T "$SERVICE" php -r '$cfg=[]; include "/usr/local/nagiosxi/html/config.inc.php"; echo $cfg["db_info"]["nagiosxi"]["pwd"];')

docker compose exec -T "$SERVICE" mysql -u "$DB_USER" -p"$DB_PASS" nagiosxi -e \
    "UPDATE xi_users SET api_enabled=1 WHERE username='nagiosadmin';" 2>/dev/null

API_TOKEN=$(docker compose exec -T "$SERVICE" mysql -u "$DB_USER" -p"$DB_PASS" nagiosxi -N -e \
    "SELECT api_key FROM xi_users WHERE username='nagiosadmin';" 2>/dev/null)

HOST_PORT=$(docker compose port "$SERVICE" 80 2>/dev/null | cut -d: -f2)

echo "export API_TOKEN=$API_TOKEN"
echo "export NAGIOS_URL=http://localhost:${HOST_PORT:-8080}/nagiosxi"
