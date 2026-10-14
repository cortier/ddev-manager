#!/usr/bin/env bash

set -euo pipefail

cd /var/www/html

lock_hash="$(sha256sum package-lock.json | cut -d ' ' -f 1)"
installed_hash="$(cat node_modules/.ddev-package-lock.sha256 2>/dev/null || true)"
if [[ "$lock_hash" != "$installed_hash" ]]; then
    npm ci
    printf '%s\n' "$lock_hash" > node_modules/.ddev-package-lock.sha256
fi

exec npm run storybook:ddev
