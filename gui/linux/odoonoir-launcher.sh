#!/bin/sh
# Odoonoir launcher — ensures required env vars for WebKitGTK
export HOME="${HOME:-$(getent passwd $(whoami) | cut -d: -f6)}"
export WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1
export GDK_BACKEND=x11
exec /usr/local/bin/odoonoir "$@"
