#!/bin/bash

CLONE_URL="${2:-'git@github.com:sourcegraph/vscode.git'}"
CLONE_DIR=/tmp/sourcegraph-vscode
REV=${1:-07154a7c72bf21d9841f922d6235cfc8e4eadf89} # pin to commit ID, bump as needed
REPO_DIR=$(git rev-parse --show-toplevel)
VENDOR_DIR="$REPO_DIR"/ui/vendor/node_modules/vscode

source "$REPO_DIR"/ui/scripts/lib.bash

fetchAndClean

# Extract the vscode tree.
#
# TODO(sqs): figure out how to exclude electron-{browser,main} to save space
# TODO(sqs): include extensions?
git --git-dir="$CLONE_DIR" archive --format=tar "$REV" \
	src \
	| tar x -C "$VENDOR_DIR" \
		  --exclude='**/test'
		  #--exclude='**/electron-browser' \
		  #--exclude='**/electron-main' \

# Remove vscode's mocha.d.ts because it is also included in our own
# node_modules/@types. If we don't do this, tsc reports errors:
#
#   ../../node_modules/@types/mocha/index.d.ts(38,13): error TS2300: Duplicate identifier 'suite'.
#   ../../node_modules/@types/mocha/index.d.ts(42,13): error TS2300: Duplicate identifier 'test'.
#   typings/mocha.d.ts(8,18): error TS2300: Duplicate identifier 'suite'.
#   typings/mocha.d.ts(9,18): error TS2300: Duplicate identifier 'test'.
rm "$VENDOR_DIR"/src/typings/mocha.d.ts

# Standardize CSS module import path syntax. There's no way to get
# Webpack to work with vscode's custom "vs/css!" syntax.
echo -n Munging imports...
grep -rl 'css!' "$VENDOR_DIR" | xargs -n 1 $sedi 's|import '"'"'vs/css!\([^'"'"']*\)'"'"';|import '"'"'\1.css'"'"';|g'
echo OK

echo -n Compiling TypeScript...
node_modules/.bin/tsc --skipLibCheck -p "$VENDOR_DIR"/src --module commonjs --declaration
cleanupSourceFiles
echo OK

echo 'Done! Updated vscode in' "$VENDOR_DIR"
