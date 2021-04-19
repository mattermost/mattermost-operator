Mattermost Operatop Helm Chart
====================================================

This is the Helm chart for the Mattermost Operator. To learn more about Helm charts, [see the Helm docs](https://helm.sh/docs/). You can find more information about Mattermost Operator [here](https://github.com/mattermost/mattermost-operator/blob/master/README.md).

The Mattermost Operator source code lives [here](https://github.com/mattermost/mattermost-operator).

# 1. Prerequisites

## 1.1 Kubernetes Cluster

You need a running Kubernetes cluster v1.8+. If you do not have one, find options and installation instructions here:

https://kubernetes.io/docs/home/

## 1.2 Helm

See: https://docs.helm.sh/using_helm/#quickstart

We recommend installing Helm v3.4.0 or later.


# 2. Configuration

To start, copy [mattermost-operator/charts/mattermost-operator/values.yaml](https://github.com/mattermost/mattermost-operator/blob/master/charts/mattermost-operator/values.yaml) and name it `config.yaml`. This will be your configuration file for the Mattermost Operator chart. You can used the default values that will deploy Mattermost-Operator, Mysql-Operator and Minio-Operator together (use of mysql and minio operators is not suggested for production environments) or update accordingly.

## 2.1 Prerequisites

Before you install the Mattermost Operator Helm chart the respectice k8s namespaces need to be created. To create all required namespaces you should run:

```
kubectl create namespace mattermost-operator
kubectl create namespace mysql-operator
kubectl create namespace minio-operator
```

In case, you are not planning to deploy mysql or minio operators via the Helm chart the namespace creation is not required for those.

# 3. Install

A public repository for the Mattermost Operator Helm chart will be added soon but for now you can clone the Mattermost Operator repo, get in the chart directory and install the Helm chart by running:

```bash
helm install mattermost-operator . -n mattermost-operator
```

To run with your custom `config.yaml`, install using:

```bash
helm install mattermost-operator . -f config.yaml -n mattermost-operator
```

To upgrade an existing release, modify the `config.yaml` with your desired changes and then use:
```bash
helm upgrade -f config.yaml <your-release-name> . -n mattermost-operator
```

## 3.1 Uninstalling Mattermost Operator Helm Chart

If you are done with your deployment and want to delete it, use `helm delete <your-release-name>`. If you don't know the name of your release, use `helm ls` to find it.


# 4. Developing

If you are going to modify the helm charts, it is helpful to use `--dry-run` (doesn't do an actual deployment) and `--debug` (print the generated config files) when running `helm install`.

Helm has partial support for pulling values out of a subchart via the requirements.yaml. It also has limited support for pushing values into subcharts. It does not support using templating inside a values.yaml file.

We recommend using [kind](https://github.com/kubernetes-sigs/kind) for local development if you do not have access to Kubernetes cluster running in the cloud.
