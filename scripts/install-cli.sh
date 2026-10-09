#!/bin/sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
CLI_DIR=${MCTRL_CLI_DIR:-"$HOME/.local/bin"}
TARGET="$CLI_DIR/mctrl"
SOURCE="$ROOT/bin/mctrl"
test -x "$SOURCE" || { echo 'Build first: make build' >&2; exit 1; }
mkdir -p "$CLI_DIR"
if [ -L "$TARGET" ] && [ "$(readlink "$TARGET")" = "$SOURCE" ]; then
  echo "mctrl is already installed at $TARGET"
elif [ -e "$TARGET" ] || [ -L "$TARGET" ]; then
  echo "Another file already exists at $TARGET; it was preserved." >&2
  exit 1
else
  ln -s "$SOURCE" "$TARGET"
  echo "Installed mctrl at $TARGET"
fi
case ":$PATH:" in
  *":$CLI_DIR:"*) echo 'From your project directory, run: mctrl here' ;;
  *) echo "Add $CLI_DIR to your shell PATH to use: mctrl here" ;;
esac
