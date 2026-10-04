#!/bin/bash
# Completes Nagios Network Analyzer's web install wizard non-interactively
# (unlike nagiosxi's fullinstall, NNA's fullinstall does not create an admin
# user or API token on its own - that only happens via this HTTP step) and
# prints the resulting API token, along with the NNA_URL this provider's
# future acceptance tests would expect.
#
# The install response hands back a working Sanctum API token directly - no
# separate "enable API access" step like nagiosxi needs (see
# test/docker/nagiosxi/get-api-token.sh). Confirmed live (#176): "type":
# "trial" activates entirely locally, no license key/activation key/
# client_id required, and a fresh container's trial is not blocked by any
# prior trial on the same host/network.
#
# This can only succeed once per container - NNA's InstallPostRequest
# rejects a second call once installed ("This action is unauthorized").
# Re-running this script against an already-installed container prints an
# error explaining that instead of a token.
#
# Usage: ./install-and-get-token.sh [compose service name, default: nagiosna]

set -euo pipefail

SERVICE="${1:-nagiosna}"
ADMIN_PASSWORD="${NNA_ADMIN_PASSWORD:-ChangeMe123!}"
ADMIN_EMAIL="${NNA_ADMIN_EMAIL:-nagiosadmin@example.com}"

HOST_PORT=$(docker compose port "$SERVICE" 80 2>/dev/null | cut -d: -f2)
NNA_URL="http://localhost:${HOST_PORT:-8081}"

RESPONSE=$(curl -s --max-time 70 -X POST "$NNA_URL/api/v1/install" \
    -H "Content-Type: application/json" \
    -d "{\"type\":\"trial\",\"password\":\"${ADMIN_PASSWORD}\",\"email\":\"${ADMIN_EMAIL}\",\"language\":\"en\",\"timezone\":\"UTC\",\"theme\":\"default\"}")

API_TOKEN=$(echo "$RESPONSE" | jq -r '.user.apikey // empty')

if [ -z "$API_TOKEN" ]; then
    echo "# Install call did not return an API token. Raw response:" >&2
    echo "$RESPONSE" >&2
    echo "#" >&2
    echo "# If this container was already installed (a previous run of this" >&2
    echo "# script, or manual UI setup), the install endpoint refuses a second" >&2
    echo "# call. POST $NNA_URL/api/v1/login does NOT work as a plain curl" >&2
    echo "# fallback here - confirmed live, it's session/CSRF-based (a" >&2
    echo "# correct username/password still errors: \"Session store not set" >&2
    echo "# on request\"). Instead, read the existing admin's apikey straight" >&2
    echo "# out of NNA's own database:" >&2
    echo "#   docker compose exec -T $SERVICE cat /var/www/html/nagiosna/.env | grep '^DB_'" >&2
    echo "#   docker compose exec -T $SERVICE mysql -u <DB_USERNAME> -p'<DB_PASSWORD>' nagiosna -N \\" >&2
    echo "#     -e \"SELECT apikey FROM users WHERE username='nagiosadmin';\"" >&2
    exit 1
fi

echo "export API_TOKEN=$API_TOKEN"
echo "export NNA_URL=$NNA_URL"
