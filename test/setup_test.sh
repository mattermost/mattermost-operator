#!/usr/bin/env bash

set -Eeuxo pipefail

kubectl get pods --all-namespaces

# Pre-load the images used by the e2e test fixtures into the kind cluster so
# the pods do not pull from Docker Hub at test time. Without this, image pulls
# inside the kind node can exceed the deployment-readiness timeout.
for image in \
    "minio/minio:RELEASE.2025-05-24T17-08-30Z" \
    "postgres:15-alpine"; do
    docker pull "$image"
    kind load docker-image "$image"
done

# Build the operator container image.
# This would build a container with tag mattermost/mattermost-operator:test,
# which is used in the e2e test setup below.
make build-image kind-load-image
sleep 5

kubectl get pods --all-namespaces

echo "Ready for testing"
