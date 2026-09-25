#!/bin/sh
# Runs the cloudns binary that matches this machine. The .deb and .apk
# both install this script at /usr/bin/cloudns.
set -eu

libdir=/usr/lib/cloudns
case "$(uname -m)" in
x86_64)
	bin="${libdir}/cloudns-amd64"
	;;
aarch64)
	bin="${libdir}/cloudns-arm64"
	;;
*)
	echo "cloudns: unsupported architecture: $(uname -m)" >&2
	exit 1
	;;
esac

if [ ! -x "$bin" ]; then
	echo "cloudns: missing binary for $(uname -m): $bin" >&2
	exit 1
fi

exec "$bin" "$@"
