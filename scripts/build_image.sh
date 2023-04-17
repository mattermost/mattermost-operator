#!/usr/bin/env bash

set -o errexit  # exit immediately if a command exits with a non-zero status
set -o nounset  # exit immediately if a variable is unset
set -o pipefail # if any command in a pipeline fails, the return status is the value of the last (rightmost) command to exit with a non-zero status, or zero if all commands in the pipeline exit successfully
set -o xtrace   # print each command before executing it

DOCKER_COMMAND=("build")
EXTRA_FLAGS=("--no-cache")

# if the PUSH_TAG environment variable is provided, tag the image with it as well.
if [[ -n "${PUSH_TAG:-}" ]]; then
    EXTRA_FLAGS+=("-t ${OPERATOR_IMAGE}:${PUSH_TAG}")
fi

# If the image is going to be built using buildx
if [[ "$1" == "buildx" ]]; then
    DOCKER_COMMAND=("buildx" "build")
    EXTRA_FLAGS=("--no-cache" "--platform linux/amd64,linux/arm64" "--push")

    # Set up buildx builder if it's not already set up
    if [[ $(docker buildx ls | grep -c operator-builder) -eq 0 ]]; then
        docker buildx create --use --name operator-builder
    fi
fi

docker "${DOCKER_COMMAND[@]}" \
    --build-arg BUILD_IMAGE="${BUILD_IMAGE}" \
    --build-arg BASE_IMAGE="${BASE_IMAGE}" \
    . -f Dockerfile \
    -t "${OPERATOR_IMAGE}" \
    "${EXTRA_FLAGS[@]}"
