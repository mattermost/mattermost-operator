#!/usr/bin/env bash

set -Eeuxo pipefail

readonly REPO_ROOT="${REPO_ROOT:-$(git rev-parse --show-toplevel)}"

export KIND_CLUSTER="kind"

run_ct_container() {
    echo 'Running testing container...'
    docker run --rm --interactive --detach --network host --name test-cont \
        --volume "$(pwd):/go/src/github.com/mattermost/mattermost-operator" \
        --workdir "/go/src/github.com/mattermost/mattermost-operator" \
        "golang:1.23" \
        cat
    echo
}

docker_exec() {
    docker exec --interactive test-cont "$@"
}


run_kind() {
    KIND_VERSION="${KIND_VERSION:=v0.22.0}"
    echo "Download kind binary..."
    curl -sSLo kind https://github.com/kubernetes-sigs/kind/releases/download/"${KIND_VERSION}"/kind-linux-amd64
    chmod +x kind
    sudo mv kind /usr/local/bin/kind

    kind version

    echo "Download kubectl..."
    curl -sSLo kubectl https://storage.googleapis.com/kubernetes-release/release/"${K8S_VERSION}"/bin/linux/amd64/kubectl
    chmod +x kubectl
    sudo cp kubectl /usr/local/bin/
    docker cp kubectl test-cont:/usr/local/bin/
    echo

    echo "Create Kubernetes cluster with kind..."
    make kind-start

    echo 'Copying kubeconfig to container...'
    kind get kubeconfig
    docker_exec mkdir /root/.kube
    docker cp ~/.kube/config test-cont:/root/.kube/config
    docker_exec kubectl cluster-info
    echo

    kubectl get all --all-namespaces

    echo 'Cluster ready!'
}

install_operator-sdk() {
    echo "Install operator-sdk"
    MACHINE="$(uname -m)"
    curl -Lo build/operator-sdk https://github.com/operator-framework/operator-sdk/releases/download/"${SDK_VERSION}"/operator-sdk-"${SDK_VERSION}"-"${MACHINE}"-linux-gnu
    chmod +x build/operator-sdk
    docker cp build/operator-sdk test-cont:/usr/local/bin/
    echo
}

cleanup() {
    echo 'Removing test container...'
    docker kill test-cont > /dev/null 2>&1
    echo 'Removing Kind Cluster...'
    kind delete cluster

    echo 'Done!'
}

main() {
    run_ct_container
    trap cleanup EXIT

    run_kind

    install_operator-sdk

    source ./test/setup_test.sh

    ## Github Actions hotfix
    ## https://github.com/actions/runner-images/issues/6775
    docker_exec git config --global --add safe.directory /go/src/github.com/mattermost/mattermost-operator
    docker_exec make deploy

    echo "Starting Operator Testing..."
    docker_exec go test ./test/e2e -timeout 50m -v

    echo "Starting External DB and File Store tests..."
    docker_exec go test ./test/e2e-external -timeout 20m -v

    echo "Done Testing!"
}

main "$@"
