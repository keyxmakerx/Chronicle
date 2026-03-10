#!/bin/sh
# docker-entrypoint.sh -- fix bind-mount permissions, then drop to non-root.
#
# When /app/data is a bind mount, the host directory's ownership may not match
# the container's "chronicle" user. This script runs as root to ensure the data
# directory is writable, then exec's the server as the unprivileged user.

set -e

# Ensure the media subdirectory exists and is owned by chronicle.
mkdir -p /app/data/media
chown -R chronicle:chronicle /app/data

# Drop privileges and exec the main process.
exec su-exec chronicle "$@"
