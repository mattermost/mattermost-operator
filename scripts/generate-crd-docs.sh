#!/usr/bin/env bash

set -o errexit
set -o pipefail

# Docs are generated with crd-ref-docs
# https://github.com/elastic/crd-ref-docs
#
# TODO: add CI/CD for downloading crd-ref-docs and checking or auto-generating
# docs when changes are made.

CRD_PATH="apis/mattermost/v1beta1/"
CONFIG_PATH="scripts/crd-ref-docs-config.yaml"
OUTPUT_PATH="docs/mattermost_v1beta1_crd.md"
RENDERER="markdown"

check_crd-ref-docs() {
if [ -f bin/crd-ref-docs ]; then
    echo "Using crd-ref-docs in bin directory"
else
    echo "Error: Unable to find crd-ref-docs in bin"
    exit 1
fi
}

generate_docs() {
  crd-ref-docs --source-path=$CRD_PATH --config=$CONFIG_PATH --output-path=$OUTPUT_PATH --renderer=$RENDERER
}

main() {
  echo "Generating CRD docs"

  check_crd-ref-docs

  generate_docs

  echo "CRD docs successfully generated"
}

main
