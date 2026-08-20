# Mattermost Operator for Kubernetes ![CircleCI branch](https://img.shields.io/circleci/project/github/mattermost/mattermost-operator/master.svg) [![Community Server](https://img.shields.io/badge/Mattermost_Community-cloud_channel-blue.svg)](https://community.mattermost.com/core/channels/cloud)

[![Mattermost](https://user-images.githubusercontent.com/7205829/137170381-fe86eef0-bccc-4fdd-8e92-b258884ebdd7.png)](https://mattermost.com)


## Summary
[Mattermost](https://mattermost.com) is an open source platform for secure collaboration across the entire software development lifecycle. It's written in Golang and React and runs as a single Linux binary with MySQL or PostgreSQL.

This repo contains a Kubernetes Operator for Mattermost to simplify deploying and managing your Mattermost instance.

The Mattermost server source code is available at https://github.com/mattermost/mattermost-server.

## Install

See the installation instructions at https://docs.mattermost.com/install/install-kubernetes.html.

## Custom Resource Documentation

You can review the full list of CRD configuration options [here](./docs/mattermost_v1beta1_crd.md).

## Upgrading to Operator v2.0.0

Version `v2.0.0` removes several long-deprecated features. Read this before upgrading.

**Removed Custom Resources**
- `ClusterInstallation` (`mattermost.com/v1alpha1`) and its automatic migration to `Mattermost`.
- `MattermostRestoreDB` (`mattermost.com/v1alpha1`).

**Removed `Mattermost` spec fields**
- `spec.database.operatorManaged` — the Operator no longer provisions a MySQL cluster.
- `spec.fileStore.operatorManaged` — the Operator no longer provisions MinIO.

`spec.database.external` and `spec.fileStore.{external,local,externalVolume}` are unchanged, and a database and file store are now **required**. A `Mattermost` that configures neither is rejected rather than silently defaulting.

**Action required before upgrading**

1. *Still using `ClusterInstallation`?* Upgrade to Operator `v1.24.x` first and let it migrate your resources to `Mattermost` (see [the migration guide](./docs/migration.md)). The migration code does not exist in `v2.0.0`, so upgrading directly leaves those objects unreadable.
2. *Using `database.operatorManaged`?* Dump the database, load it into one you manage, and set `database.external` to a secret holding its connection string.
3. *Using `fileStore.operatorManaged`?* Copy the objects out of MinIO into an S3 compatible bucket or a PVC, then set `fileStore.external` or `fileStore.local`.

Nothing in the upgrade deletes an existing `MysqlCluster` or `MinIOInstance`, so your data is not removed — but the Operator stops managing it, and Mattermost will not be pointed at it any more.

## Developer Flow
To test the operator locally. We recommend [Kind](https://kind.sigs.k8s.io/), however, you can use Minikube or Minishift as well.

### Prerequisites
To develop locally you will need the [Operator SDK](https://github.com/operator-framework/operator-sdk).

First, checkout and install the operator-sdk CLI:

```bash
mkdir -p $GOPATH/src/github.com/operator-framework
cd $GOPATH/src/github.com/operator-framework
git clone https://github.com/operator-framework/operator-sdk
cd operator-sdk
git checkout master
make install
```

If you made changes to any structs representing Custom Resources make sure to regenerate code and manifests:
```
make generate manifests
```

If generation produced any unexpected changes, clean old binaries and rerun the generation:
```
make clean generate manifests
```

> If the manifest generation is making changes to package paths this is [a known bug](https://github.com/kubernetes/gengo/issues/147) while running the previous command outside `GOPATH` or by having the `GOPATH` flag unset. Make sure you clone the repository in the appropriate folder (see below) and that your `GOPATH` environment variable is set.

### Building mattermost-operator
To start contributing to mattermost-operator you need to clone this repo to your local workspace.

```bash
mkdir -p $GOPATH/src/github.com/mattermost
cd $GOPATH/src/github.com/mattermost
git clone https://github.com/mattermost/mattermost-operator
cd mattermost-operator
git checkout master
make build
```

### Testing locally with Kind
Developing and testing local changes to Mattermost Operator is fairly simple. For that you can deploy Kind cluster.

> **NOTE:**
> You don't need to push the mattermost-operator image to DockerHub or any other registry if testing with kind. You can load the image, built with `make build-image`, directly to the Kind cluster by running the following:
> ```bash
> kind load docker-image mattermost/mattermost-operator:test
> ```

To spin up an appropriate Kind cluster and deploy dependencies, run:
```bash
make kind-start
```

After Kind cluster is up and running, build Mattermost Operator image, load it to Kind cluster and deploy it. For that, run:
```bash
make build-image kind-load-image deploy
```

### Accessing Mattermost Installation on Kind

After you create Mattermost installation using Mattermost Operator on Kind cluster,
port-forward the service to access it:
```bash
kubectl port-forward svc/[MATTERMOST_NAME] 8065:8065
```

### Running Operator locally against K8s cluster

Mattermost Operator can be run on local machine against remote a Kubernetes cluster to rapidly test changes during the development.

To run Operator locally:
- Make sure you are connected to a Kubernetes cluster.
- Install Custom Resources by running: `kubectl apply -f ./config/crd/bases`.
- Make sure Mattermost Operator **is not** running in the cluster or scale it down to 0 replicas to avoid unexpected behaviour.
- Run Operator binary: `go run .`

Be aware that running Operator locally does not verify Kubernetes manifests, RBAC rules, leader election etc.

## Notes

### Installation Size

The `spec.Size` field was modified to be treated as a write-only field.
After adjusting values according to the size, the value of `spec.Size` is erased.

Replicas and resource requests/limits values can be overridden manually but setting new Size will override those values again regardless if set by the previous Size or adjusted manually.

## Release

To release a new version of Mattermost Operator you need to:

- Have the repository up-to-date
- Have the remote upstream configured
- Have a clean repo, not pending commits and changes

As a first step of release process generate deployment manifests:
```
make yaml
```

We have a script that changes some files, commit those changes and then tag the main branch.

To run you can issue the following command:

```bash
./scripts/release.sh --tag=<DESIRED_TAG>
```

where:
- `<DESIRED_TAG>` can be 1.10.1 for example

## Why the Mattermost Operator Uses a ClusterRole

The Mattermost Operator is designed to manage and orchestrate multiple Mattermost installations within a Kubernetes cluster.  
To enable this, it requires **cluster-wide permissions**, which are granted through a `ClusterRole`.

### Design Philosophy

The operator follows a **high-privilege controller model** built around the **principle of least privilege for managed applications**.

- The operator itself requires elevated privileges at the cluster level to create and manage resources across namespaces.  
- Each Mattermost installation it provisions is isolated using its own **`ServiceAccount`** and **namespace-scoped `Role`**, ensuring that each deployment operates with minimal privileges.

This approach balances **security** and **convenience** — administrators do not need to manually create RBAC policies for each Mattermost instance.

This allows the operator to:

1. **Multi-Namespace Deployments**  
   The operator supports multiple Mattermost instances deployed across different namespaces (e.g., `dev`, `staging`, `prod`) using a single operator instance.  
   This enables platform teams to offer *Mattermost-as-a-Service* to multiple internal teams.

2. **Cluster-Scoped Resources**  
   Some Kubernetes resources managed by the operator (such as `IngressClass`) are **cluster-scoped** and cannot be managed with namespace-limited roles.

3. **Cross-Namespace Integrations**  
   The operator reads Secrets and PersistentVolumeClaims that may reside in other namespaces.  
   Managing these cross-namespace resources requires cluster-level permissions.

4. **Namespace Metadata Access**  
   The operator needs to read and interact with namespace metadata to properly configure DNS names, routing, and service discovery across installations.

5. **Operational Flexibility**  
   Operating at the cluster level simplifies management workflows.  
   Platform administrators can deploy, update, and monitor multiple Mattermost instances without deploying a separate operator in each namespace.
