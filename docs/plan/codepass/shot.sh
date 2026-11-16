#!/bin/zsh
# usage: shot.sh <out.png> <width> <height> <query>
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
"$CHROME" --headless=new --disable-gpu --hide-scrollbars --force-device-scale-factor=1 --virtual-time-budget=4000 \
  --user-data-dir="$(mktemp -d)" --window-size="$2,$3" --screenshot="$1" "http://127.0.0.1:9245/?$4" >/dev/null 2>&1
echo "$1: $(test -s "$1" && echo ok || echo FAIL)"
