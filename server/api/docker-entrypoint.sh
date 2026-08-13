#!/bin/sh
set -eu

if [ "$(id -u)" -eq 0 ]; then
    mkdir -p /data
    if ! setpriv --reuid=10001 --regid=10001 --clear-groups test -w /data \
        || { [ -e /data/smshub.db ] && ! setpriv --reuid=10001 --regid=10001 --clear-groups test -w /data/smshub.db; }; then
        chown -R 10001:10001 /data
    fi
    exec setpriv --reuid=10001 --regid=10001 --clear-groups "$@"
fi

exec "$@"
