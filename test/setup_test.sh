#!/usr/bin/env bash

set -Eeuxo pipefail

kubectl get pods --all-namespaces

# Pre-load the e2e fixture images into the kind cluster (avoids Docker Hub pulls
# during the test run) and force containerd to unpack their layers NOW.
#
# Background: 'kind load docker-image' copies the compressed image into each
# kind node's containerd store but does NOT unpack the filesystem layers.
# Unpacking happens on first container creation and, inside the nested
# Docker-on-Docker kind environment CI uses, can take 10+ minutes for a
# 200 MB+ image.  Running throwaway pods here completes the unpack phase so the
# actual test pods start immediately.
for image in \
    "minio/minio:RELEASE.2025-05-24T17-08-30Z" \
    "postgres:15-alpine"; do
    docker pull "$image"
    kind load docker-image "$image"
done

# Run one throwaway pod per image (using 'sleep' so the pod stays alive long
# enough for 'kubectl wait --for=condition=Ready' to observe it, which only
# fires after the image is fully unpacked and the container is running).
kubectl run minio-warmup   --image=minio/minio:RELEASE.2025-05-24T17-08-30Z  --restart=Never --command -- sleep 300 &
kubectl run postgres-warmup --image=postgres:15-alpine                        --restart=Never --command -- sleep 300 &
wait

kubectl wait --for=condition=Ready pod/minio-warmup   --timeout=10m
kubectl wait --for=condition=Ready pod/postgres-warmup --timeout=10m
kubectl delete pod minio-warmup postgres-warmup --ignore-not-found=true

# Build the operator container image.
# This would build a container with tag mattermost/mattermost-operator:test,
# which is used in the e2e test setup below.
make build-image kind-load-image
sleep 5

kubectl get pods --all-namespaces

echo "Ready for testing"
