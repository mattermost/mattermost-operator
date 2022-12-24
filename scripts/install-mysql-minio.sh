#!/usr/bin/env bash

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

## Create the mysql operator
# Apply Namespace if already exists
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: mysql-operator
EOF
kubectl apply -n mysql-operator -f "${DIR}"/../docs/mysql-operator/mysql-operator.yaml

## Create the minio operator
KERNEL_NAME=$(uname | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
if [[ "${ARCH}" == "x86_64" ]]; then
  ARCH=amd64
fi
mkdir -p bin
curl "https://github.com/minio/operator/releases/download/v${MINIO_OPERATOR_VERSION}/kubectl-minio_${MINIO_OPERATOR_VERSION}_${KERNEL_NAME}_${ARCH}" -L -o bin/kubectl-minio
chmod +x bin/kubectl-minio
PATH=$(pwd)/bin:$PATH
export PATH
kubectl minio init
