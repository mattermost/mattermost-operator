#!/usr/bin/env bash

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

## Create the mysql operator
# Apply Namespace if already exists
helm repo add bitpoke https://helm-charts.bitpoke.io
helm repo update 
helm install mysql-operator bitpoke/mysql-operator --namespace mysql-operator --create-namespace --set "extraArgs={--mysql-versions-to-image=5.7.26=percona:5.7.35}" --version v0.6.2

## Create the minio operator
# Apply Namespace if already exists
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: minio-operator
EOF
kubectl apply -n minio-operator -f "${DIR}"/../docs/minio-operator/minio-operator.yaml
