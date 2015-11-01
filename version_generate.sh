#! /bin/sh

VERSION=$(git describe --tags --abbrev=0)

cat > version.go <<EOF
package main

var Version string = "$VERSION"
EOF
