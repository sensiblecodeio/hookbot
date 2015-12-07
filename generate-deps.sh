#!/bin/bash -eu

TARGET=$1
SOURCE=$2

# Take arguments as stdin
golist() {
  PROG=$1
  xargs go list -f "$PROG"
}

# Remove $PWD prefix
make_relative() {
  sed --quiet 's|^'$(pwd)/'||p'
}

# Prepend target: and target.deps:
prepend_targets() {
  sed --quiet 's|^|'${TARGET}': |p;s|^'${TARGET}':|'${TARGET}.deps':|p'
}

ALL_IMPORTS='{{.ImportPath}}{{"\n"}}{{range .Deps}}{{.}}{{"\n"}}{{end}}'
NON_STDLIB='{{if not .Standard}}{{.ImportPath}}{{end}}'
GO_SOURCES='{{with $p := .}}{{range .GoFiles}}{{$p.Dir}}/{{.}}{{"\n"}}{{end}}{{end}}'

set -o pipefail

echo "$SOURCE" |
  golist "$ALL_IMPORTS" |
  golist "$NON_STDLIB" |
  golist "$GO_SOURCES" |
  make_relative |
  prepend_targets
