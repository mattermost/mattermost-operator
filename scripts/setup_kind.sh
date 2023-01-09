#!/usr/bin/env bash

# Ensure we use the proper kind configuration for arm64 (M1 Macs)
if [[ "$(uname -m)" == "arm64" ]]; then
    echo "arm64 detected: using kind arm64 configuration"
    KIND_CONFIG_FILE=kind-config-arm64.yaml
fi

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

if kind export kubeconfig --name "$KIND_CLUSTER" ; then
    echo "Using existing cluster"
else
    echo "Creating new Kind cluster"
    kind create cluster --name "${KIND_CLUSTER}" --config "${DIR}"/"${KIND_CONFIG_FILE}"
    kind export kubeconfig --name "$KIND_CLUSTER"
fi
