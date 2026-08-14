#!/usr/bin/env bash

## Create the mysql operator
# Apply Namespace if already exists
helm repo add bitpoke https://helm-charts.bitpoke.io
helm repo update
helm install mysql-operator bitpoke/mysql-operator --namespace mysql-operator --create-namespace --set "extraArgs={--mysql-versions-to-image=5.7.26=percona:5.7.35}" --version v0.6.2
