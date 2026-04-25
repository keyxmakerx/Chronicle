#!/bin/sh
# docker-entrypoint.sh -- fix bind-mount permissions, then run the server.
#
# When running as root (default), this script fixes /app/data ownership and
# drops to the "chronicle" user via su-exec.
# When running as non-root (e.g. Cosmos Cloud sets user: chronicle), it
# creates subdirectories if writable and runs the server directly.
#
# For bind mounts with non-root user, ensure the host directory is owned by
# the container's UID:  chown -R $(id -u chronicle):$(id -g chronicle) /path/to/data

set -e

if [ "$(id -u)" = "0" ]; then
    # Running as root: ensure dirs exist, fix ownership, drop privileges.
    # /app/data/backups is created so the in-process pre-migration backup
    # (PreMigrationBackup, BACKUP_DIR=/app/data/backups by default) and the
    # operator scripts/backup.sh have a writable destination on first boot.
    mkdir -p /app/data/media /app/data/backups /app/foundry-module
    chown -R chronicle:chronicle /app/data /app/foundry-module
    exec su-exec chronicle "$@"
else
    # Running as non-root (platform-enforced user).
    # Try to create media + backups dirs; if it fails, the bind mount host
    # dir needs its ownership fixed (see comment above).
    if ! mkdir -p /app/data/media /app/data/backups 2>/dev/null; then
        echo "WARNING: Cannot create /app/data/media or /app/data/backups -- bind mount not writable by UID $(id -u)." >&2
        echo "Fix: chown -R $(id -u):$(id -g) <host-data-dir>" >&2
    fi
    exec "$@"
fi
