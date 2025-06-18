#!/usr/bin/env bash

## Run e2e tests on local machine
## Requirements:
## - kind 0.29.0
## - kustomize

set -Eeuxo pipefail

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

export KIND_CLUSTER="kind"

CLUSTER_NAME=${CLUSTER_NAME:-kind}

kind version

echo "Creating Kind cluster"
make kind-start

# shellcheck source=test/setup_test.sh
source "${DIR}"/setup_test.sh

# Deploy Mattermost Operator
make deploy

echo "Running operator e2e tests..."
go test ./test/e2e -count=1 --timeout 45m -v

echo "Running external DB and File Store e2e..."
go test ./test/e2e-external -count=1 --timeout 15m -v
