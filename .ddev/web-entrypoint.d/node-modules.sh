#!/usr/bin/env bash

set -euo pipefail

sudo chown "$(id -u):$(id -g)" /var/www/html/node_modules
