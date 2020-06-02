#!/usr/bin/env bash

set -Eeuxo pipefail

readonly REPO_ROOT="${REPO_ROOT:-$(git rev-parse --show-toplevel)}"

run_ct_container() {
    echo 'Running testing container...'
    docker run --rm --interactive --detach --network host --name test-cont \
        --volume "$(pwd):/go/src/github.com/mattermost/mattermost-operator" \
        --workdir "/go/src/github.com/mattermost/mattermost-operator" \
        "golang:1.13" \
        cat
    echo
}

docker_exec() {
    docker exec --interactive test-cont "$@"
}


run_kind() {
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
    kind create cluster --config test/kind-config.yaml --wait 5m
    echo

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

    # Move the operator container inside Kind container so that the image is
    # available to the docker in docker environment.
    # Copy the image to the cluster to make a bit more fast to start
    docker pull quay.io/presslabs/mysql-operator:0.3.3
    docker pull quay.io/presslabs/mysql-operator-sidecar:0.3.3
    docker pull quay.io/presslabs/mysql-operator-orchestrator:0.3.3
    docker pull minio/k8s-operator:1.0.7

    kind load docker-image quay.io/presslabs/mysql-operator:0.3.3
    kind load docker-image quay.io/presslabs/mysql-operator-sidecar:0.3.3
    kind load docker-image quay.io/presslabs/mysql-operator-orchestrator:0.3.3
    kind load docker-image minio/k8s-operator:1.0.7
    sleep 10

    # Create a namespace for testing operator.
    # This is needed because the service account created using
    # deploy/service_account.yaml has a static namespace. Creating operator in
    # other namespace will result in permission errors.
    kubectl create ns mattermost-operator

    # Create the mysql operator
    kubectl create ns mysql-operator
    kubectl apply -n mysql-operator -f docs/mysql-operator/mysql-operator.yaml
    sleep 10
    # Create the minio operator
    kubectl create ns minio-operator
    kubectl apply -n minio-operator -f docs/minio-operator/minio-operator.yaml
    sleep 10

    kubectl get pods --all-namespaces
    # Build the operator container image.
    # This would build a container with tag mattermost/mattermost-operator:test,
    # which is used in the e2e test setup below.
    make build-image
    kind load docker-image mattermost/mattermost-operator:test
    sleep 5

    kubectl get pods --all-namespaces
    echo "Ready for testing"
    # NOTE: Append this test command with `|| true` to debug by inspecting the
    # resource details. Also comment `defer ctx.Cleanup()` in the cluster to
    # avoid resouce cleanup.
    echo "Starting Operator Testing..."
    docker_exec operator-sdk test local ./test/e2e --debug --verbose --operator-namespace mattermost-operator --kubeconfig /root/.kube/config --go-test-flags -timeout=30m

    echo "Done Testing!"
}

main "$@"
