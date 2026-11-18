#!/bin/sh
# Linux维护者发布入口，参数说明见release.py --help；不改变NAS运行参数。
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec python3 -B "$script_dir/release.py" "$@"
