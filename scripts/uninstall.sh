#!/bin/sh
set -eu
package_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
if [ -x "$package_dir/ddev-manager" ]; then
  exec "$package_dir/ddev-manager" uninstall
fi
case "$(uname -s)" in
  Linux) installed="$HOME/.local/share/cortier-ddev-manager/ddev-manager" ;;
  Darwin) installed="$HOME/Library/Application Support/Cortier DDEV Manager/ddev-manager" ;;
  *) echo "Supported systems: Linux and macOS" >&2; exit 1 ;;
esac
if [ ! -x "$installed" ]; then
  echo "Companion executable not found. Extract a release package and run its uninstall.sh." >&2
  exit 1
fi
exec "$installed" uninstall
