#!/bin/bash

# Updates submodules

git submodule foreach git fetch
# Assumes that all submodules use the master branch
git submodule foreach git rebase origin/master master

go list -f $'{{range .Deps}}{{.}}\n{{end}}' github.com/sensiblecodeio/hanoverd | \
	xargs go list -f $'{{if not .Standard}}{{.ImportPath}}\n{{end}}' | \
	egrep '/hookbot/vendor/' > dependencies
