#!/usr/bin/env bash

set -o errexit


cp ./config/crd/bases/installation.mattermost.com_mattermosts.yaml charts/mattermost-operator/crds/crd-mattermosts.yaml
cp ./config/crd/bases/mattermost.com_clusterinstallations.yaml charts/mattermost-operator/crds/crd-clusterinstallations.yaml
cp ./config/crd/bases/mattermost.com_mattermostrestoredbs.yaml charts/mattermost-operator/crds/crd-mattermostrestoredbs.yaml
