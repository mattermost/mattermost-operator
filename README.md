# Mattermost Operator for Kubernetes ![CircleCI branch](https://img.shields.io/circleci/project/github/mattermost/mattermost-operator/master.svg) ![Alpha](https://img.shields.io/badge/alfa-in%20progress-yellow.svg) [![Community Server](https://img.shields.io/badge/Mattermost_Community-cloud_channel-blue.svg)](https://community.mattermost.com/core/channels/cloud)
Mattermost Operator for Kubernetes is under construction.

## Status

The Mattermost operator is in alpha, data loss might occur.

## Summary

Mattermost is a scalale, open source collaboration tool. It's written in Golang and React.

This project offers a Kubernetes Operator for Mattermost to simplify deploying and managing your Mattermost instance.


## Getting Started

### Prerequisites

- Kubernetes cluster 1.8.0+.
- `kubectl` configured for the relevant Kubernetes cluster. (https://kubernetes.io/docs/reference/kubectl/overview/)

### Install Operators

To start Mattermost-Operator, we need to install the dependencies first.

#### MySQL-Operator
To install MySQL-Operator apply the manifests that you can find in the `docs` folder

```
$ kubectl create ns mysql-operator
$ kubectl apply -n mysql-operator -f https://github.com/mattertmost/mattermost-operator/blob/master/docs/mysql-operator/mysql-operator.yaml?raw=true
```

#### Mattermost-Operator
After the dependencies installed we need to deploy Mattermost-operator
Apply the manifests in the `docs` folder as well

```
$ kubectl create ns mattermost-operator
$ kubectl apply -n mattermost-operator -f https://github.com/mattertmost/mattermost-operator/blob/master/docs/mattermost-operator/mattermost-operator.yaml?raw=true
```

### Install Mattermost

Once Mattermost-Operator deployment is running, you can create a Mattermost cluster installation using the below command

```
$ kubectl create -f https://github.com/minio/mattermost-operator/blob/master/docs/examples/simple.yaml?raw=true
```


## Developer flow

If you want to contribute you can build and test the operator locally. We are using [Kind](https://kind.sigs.k8s.io/), but you can use Minikube or Minishift as well.

### Prerequisites

- [Operator SDK](https://github.com/operator-framework/operator-sdk)

### Makefile commands

- `make build` - compile the operator
- `make build-image` - generate the Docker image. The default image is `mattermost/mattermost-operator:test`
- `make generate` - runs the kubernetes [code-generators](https://github.com/kubernetes/code-generator) for all Custom Resource Definitions (CRD) apis under `pkg/apis/...`. Also runs the [kube-openapi](https://github.com/kubernetes/kube-openapi) OpenAPIv3 code generator for all Custom Resource Definition (CRD) API tagged fields under `pkg/apis/...`
- `make check-style` - runs govet/gofmt
- `make unitest` - run the unit tests

### Testing locally

Developing and testing local changes to the `mattermost-operator` is fairly simple. For that you can deploy Kind and then apply the manifests to deploy the dependencies and to deploy Mattermost operator as well.

You dont need to push the mattermost-operator image to the Docker Hub or any other Registry you can load the image you built using `make build-image` directly to the Kind cluster.

```
$ kind load docker-image mattermost/mattermost-operator:test
```
