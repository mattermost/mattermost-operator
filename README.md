# Mattermost Operator for Kubernetes ![CircleCI branch](https://img.shields.io/circleci/project/github/mattermost/mattermost-operator/master.svg) [![Community Server](https://img.shields.io/badge/Mattermost_Community-cloud_channel-blue.svg)](https://community.mattermost.com/core/channels/cloud)
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
$ kubectl apply -n mattermost-operator -f https://github.com/mattermost/mattermost-operator/blob/master/docs/mattermost-operator/mattermost-operator.yaml?raw=true
```

### Install Mattermost

With the above operators installed, install Mattermost using the mattermost-operator:

```
$ kubectl create -f https://github.com/minio/mattermost-operator/blob/master/docs/examples/simple.yaml?raw=true
```

The [simple.yml](https://github.com/minio/mattermost-operator/blob/master/docs/examples/simple.yaml?raw=true) configures the options available for installing Mattermost.

They are documented as follows:
 - `name`: Name of the Mattermost deployment
 - `ingressName`: The ingress name for your Mattermost deployment.


## Developer flow

To test the operator locally. We are recommend [Kind](https://kind.sigs.k8s.io/), however, you can use Minikube or Minishift as well.

### Prerequisites

- [Operator SDK](https://github.com/operator-framework/operator-sdk)

First, checkout and install the operator-sdk CLI:

```
  $ mkdir -p $GOPATH/src/github.com/operator-framework
  $ cd $GOPATH/src/github.com/operator-framework
  $ git clone https://github.com/operator-framework/operator-sdk
  $ cd operator-sdk
  $ git checkout master
  $ make dep
  $ make install
```

### Testing locally

Developing and testing local changes to the `mattermost-operator` is fairly simple. For that you can deploy Kind and then apply the manifests to deploy the dependencies and to deploy Mattermost operator as well.

You dont need to push the mattermost-operator image to the Docker Hub or any other Registry you can load the image you built using `make build-image` directly to the Kind cluster.

```
$ kind load docker-image mattermost/mattermost-operator:test
```
