#!/usr/bin/env bash

set -o errexit  # exit immediately if a command exits with a non-zero status
set -o nounset  # exit immediately if a variable is unset
set -o pipefail # if any command in a pipeline fails, the return status is the value of the last (rightmost) command to exit with a non-zero status, or zero if all commands in the pipeline exit successfully
set -o xtrace   # print each command before executing it

if [[ "$1" == "local" ]]; then
    docker push "${OPERATOR_IMAGE}"
else
    docker buildx push "${OPERATOR_IMAGE}"
fi
