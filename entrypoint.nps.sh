#!/bin/sh

set -e

CONF_DIR="/conf"
CONF_FILE="$CONF_DIR/sysuahb.conf"

if [ ! -d "$CONF_DIR" ]; then
    mkdir -p "$CONF_DIR"
fi

if [ ! -f "$CONF_FILE" ]; then
    cp /sysuahb.conf.sample "$CONF_FILE"
    echo "[entrypoint] generated default config at $CONF_FILE"
    echo "[entrypoint] please change web_username / web_password before exposing the web UI"
fi

exec /sysuahb "$@"
