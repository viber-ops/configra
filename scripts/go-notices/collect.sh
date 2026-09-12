#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  printf '%s\n' 'Usage: sh scripts/go-notices/collect.sh /absolute/new/output-directory' >&2
  exit 1
fi
notice_output=$1
case "$notice_output" in
  /*) ;;
  *) printf '%s\n' 'The notice output directory must be absolute.' >&2; exit 1 ;;
esac
if [ -e "$notice_output" ] || [ -L "$notice_output" ]; then
  printf '%s\n' 'Refusing to mix Go notices with an existing output directory.' >&2
  exit 1
fi
notice_version=$(go env GOVERSION)
if [ "$notice_version" != 'go1.26.7' ]; then
  printf '%s\n' 'Go notices have been reviewed only for go1.26.7; review the new toolchain before packaging it.' >&2
  exit 1
fi
if [ -n "$(go env GOEXPERIMENT)" ]; then
  printf '%s\n' 'Custom Go experiments require a separate runtime notice review.' >&2
  exit 1
fi
if [ "$(go env CGO_ENABLED)" != '0' ]; then
  printf '%s\n' 'This runtime notice collection is reviewed for CGO_ENABLED=0 only.' >&2
  exit 1
fi
case "$(go env GOOS)/$(go env GOARCH)" in
  linux/amd64|linux/arm64|darwin/amd64|darwin/arm64) ;;
  *) printf '%s\n' 'This runtime notice collection is reviewed only for the four release targets.' >&2; exit 1 ;;
esac
notice_goroot=$(go env GOROOT)
notice_script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

# Validate the entire curated input before creating any output.
while IFS= read -r notice_file; do
  case "$notice_file" in
    ''|/*|*..*|*\\*) printf '%s\n' 'Invalid Go notice manifest path.' >&2; exit 1 ;;
  esac
  test -f "$notice_goroot/$notice_file"
done < "$notice_script_dir/files.txt"

mkdir -p "$notice_output"
while IFS= read -r notice_file; do
  mkdir -p "$notice_output/$(dirname -- "$notice_file")"
  notice_destination=$notice_file
  case "$notice_file" in
    *.go|*.s|*.h) notice_destination=$notice_file.txt ;;
  esac
  cp "$notice_goroot/$notice_file" "$notice_output/$notice_destination"
done < "$notice_script_dir/files.txt"
cp "$notice_script_dir/SOURCE.md" "$notice_output/SOURCE.md"
printf '%s\n' 'Collected reviewed Go 1.26.7 runtime notices.'
