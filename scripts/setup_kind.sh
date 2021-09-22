#!/usr/bin/env bash

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

if kind export kubeconfig --name "$KIND_CLUSTER" ; then
    echo "Using existing cluster"
else
    echo "Creating new Kind cluster"
    kind create cluster --name "${KIND_CLUSTER}" --config "${DIR}"/"${KIND_CONFIG_FILE}"
    kind export kubeconfig --name "$KIND_CLUSTER"
fi
