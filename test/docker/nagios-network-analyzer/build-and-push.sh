#!/usr/bin/env bash
set -euo pipefail

# Builds the nagiosna test image and pushes it to a private GHCR package so
# `docker compose up` can pull it instead of repeating the from-source
# install on every fresh checkout/machine. Run this after any change to
# Dockerfile or nna-install.service; day-to-day `docker compose up` doesn't
# need it.
#
# Requires `docker login ghcr.io` first, with a PAT that has `write:packages`.
# See README.md's "Speeding up local runs with a prebuilt image" section for
# one-time setup (including marking the package Private on GitHub).

IMAGE="ghcr.io/dunkin0486/terraform-provider-nagios-test-nna"
TAG="${1:-latest}"

cd "$(dirname "$0")"
docker build -t "$IMAGE:$TAG" .
docker push "$IMAGE:$TAG"

echo "Pushed $IMAGE:$TAG"
echo "First push: confirm the package is set to Private at"
echo "  https://github.com/users/dunkin0486/packages/container/terraform-provider-nagios-test-nna/settings"
