#!/bin/sh
set -eu
package_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
if [ ! -x "$package_dir/ddev-manager" ]; then
  echo "Run this installer from an extracted companion release package." >&2
  exit 1
fi
exec "$package_dir/ddev-manager" install "$@"
