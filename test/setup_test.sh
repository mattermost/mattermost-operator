#!/usr/bin/env bash

set -Eeuxo pipefail

# The e2e suites now provision Postgres and MinIO themselves from the manifests in
# resources/, so nothing needs to be pre-pulled here. The percona and
# mysqld-exporter images this script used to load existed only for the
# Operator-managed MySQL path, which has been removed.

kubectl get pods --all-namespaces

# Build the operator container image.
# This would build a container with tag mattermost/mattermost-operator:test,
# which is used in the e2e test setup below.
make build-image kind-load-image
sleep 5

kubectl get pods --all-namespaces

echo "Ready for testing"
