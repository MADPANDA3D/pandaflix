#!/bin/sh
# Pandaflix installer entry point (run from the repository root).
#
#   sh apk-install.sh          # install runtime deps + latest release binary
#   sh apk-install.sh --build  # build this checkout instead of downloading
#
# See scripts/install.sh for details.
exec sh "$(dirname "$0")/scripts/install.sh" "$@"