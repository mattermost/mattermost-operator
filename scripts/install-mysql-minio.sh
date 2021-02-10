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
# Apply Namespace if already exists
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: minio-operator
EOF
kubectl apply -n minio-operator -f "${DIR}"/../docs/minio-operator/minio-operator.yaml
