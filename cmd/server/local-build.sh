#!/usr/bin/env bash

echo "Building server locally. This is for testing purposes only."
echo

cd $(dirname "${BASH_SOURCE[0]}")/../..
export GOBIN=$PWD/cmd/server/.bin
set -ex

if [[ -z "${SKIP_PRE_BUILD-}" ]]; then
	./cmd/server/pre-build.sh
fi


# Keep in sync with build.sh
go install -tags dist \
   sourcegraph.com/sourcegraph/sourcegraph/cmd/server \
   sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend \
   sourcegraph.com/sourcegraph/sourcegraph/cmd/github-proxy \
   sourcegraph.com/sourcegraph/sourcegraph/cmd/gitserver \
   sourcegraph.com/sourcegraph/sourcegraph/cmd/indexer \
   sourcegraph.com/sourcegraph/sourcegraph/cmd/query-runner \
   sourcegraph.com/sourcegraph/sourcegraph/cmd/symbols \
   sourcegraph.com/sourcegraph/sourcegraph/cmd/repo-updater \
   sourcegraph.com/sourcegraph/sourcegraph/cmd/searcher \
   sourcegraph.com/sourcegraph/sourcegraph/cmd/lsp-proxy

cat > $GOBIN/syntect_server <<EOF
#!/bin/sh
# Pass through all possible ROCKET env vars
docker run --name=syntect_server --rm -p9238:9238  \
-e QUIET \
-e ROCKET_ENV \
-e ROCKET_ADDRESS \
-e ROCKET_PORT \
-e ROCKET_WORKERS \
-e ROCKET_LOG \
-e ROCKET_SECRET_KEY \
-e ROCKET_LIMITS \
sourcegraph/syntect_server
EOF
chmod +x $GOBIN/syntect_server
