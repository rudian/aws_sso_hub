#!/bin/zsh
# usage: shoturl.sh <out.png> <width> <height> <full-url>   (works with file:// and http:// URLs)
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
"$CHROME" --headless=new --disable-gpu --hide-scrollbars --force-device-scale-factor=1 --virtual-time-budget=4000 \
  --allow-file-access-from-files --user-data-dir="$(mktemp -d)" --window-size="$2,$3" --screenshot="$1" "$4" >/dev/null 2>&1
echo "$1: $(test -s "$1" && echo ok || echo FAIL)"
