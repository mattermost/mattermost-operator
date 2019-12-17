# Mattermost Operator for Kubernetes ![CircleCI branch](https://img.shields.io/circleci/project/github/mattermost/mattermost-operator/master.svg) [![Community Server](https://img.shields.io/badge/Mattermost_Community-cloud_channel-blue.svg)](https://community.mattermost.com/core/channels/cloud)

## Summary
Mattermost is a scalable, open source collaboration tool. It's written in Golang and React.

This project offers a Kubernetes Operator for Mattermost to simplify deploying and managing your Mattermost instance.

Learn more about Mattermost at https://mattermost.com.

The Mattermost server source code is available at https://github.com/mattermost/mattermost-server.

## 1 Install

See the install instructions at https://docs.mattermost.com/install/install-kubernetes.html.

## 2 Restore an existing Mattermost MySQL Database
To restore an existing Mattermost MySQL Database into a new Mattermost installation using the Mattermost Operator you will need to follow these steps:

Use Case: An existing AWS RDS Database
  - First you need to dump the data using mysqldump
  - Create an EC2 instance and install MySQL
  - Restore the dump in this new database
  - Install `Percona XtraBackup`
  - Perform the backup using the `Percona XtraBackup`
    - `xtrabackup --innodb_file_per_table=1 --innodb_flush_log_at_trx_commit=2 --innodb_flush_method=O_DIRECT --innodb_log_files_in_group=2 --log_bin=/var/lib/mysql/mysql-bin --open_files_limit=65535 --innodb_buffer_pool_size=512M --innodb_log_file_size=128M --server-id=100 --backup=1 --slave-info=1 --stream=xbstream --host=127.0.0.1 --user=USER --password=PASSWORD --target-dir=~/xtrabackup_backupfiles/ | gzip - > BACKNAME.gz`
  - Upload to an AWS S3 bucket
  - Create a Mattermost Cluster, for example:
  ```
  apiVersion: mattermost.com/v1alpha1
  kind: ClusterInstallation
  metadata:
    name: example-clusterinstallation
  spec:
    ingressName: example.mattermost-example.dev
  ```
  - Create the Restore/Backup secret with the AWS credentials
  ```
  apiVersion: v1
  kind: Secret
  metadata:
    name: restore-secret
  type: Opaque
  stringData:
    AWS_ACCESS_KEY_ID: XXXXXXXXXXXX
    AWS_SECRET_ACCESS_KEY: XXXXXXXXXXXX/XXXXXXXXXXXX
    AWS_REGION: us-east-1
    S3_PROVIDER: AWS
  ```
  - Create the mattermost restore manifest to deploy
  ```
  apiVersion: mattermost.com/v1alpha1
  kind: MattermostRestoreDB
  metadata:
    name: example-mattermostrestoredb
  spec:
    initBucketURL: s3://my-sample/my-backup.gz
    mattermostClusterName: example-clusterinstallation
    mattermostDBName: mattermostdb
    mattermostDBPassword: supersecure
    mattermostDBUser: mmuser
    restoreSecret: restore-secret
  ```

If you have an machine running MySQL you just need to perform the `Percona XtraBackup` step

## 3 Developer Flow
To test the operator locally. We recommend [Kind](https://kind.sigs.k8s.io/), however, you can use Minikube or Minishift as well.

### 3.1 Prerequisites
To develop locally you will need the [Operator SDK](https://github.com/operator-framework/operator-sdk).

First, checkout and install the operator-sdk CLI:

```bash
$ mkdir -p $GOPATH/src/github.com/operator-framework
$ cd $GOPATH/src/github.com/operator-framework
$ git clone https://github.com/operator-framework/operator-sdk
$ cd operator-sdk
$ git checkout master
$ make install
```

Second, you need to make sure you have [dep](https://github.com/golang/dep) installed. 

### 3.2 Building mattermost-operator
To start contributing to mattermost-operator you need to clone this repo to your local workspace. 

```bash
$ mkdir -p $GOPATH/src/github.com/mattermost
$ cd $GOPATH/src/github.com/mattermost
$ git clone https://github.com/mattermost/mattermost-operator
$ cd mattermost-operator
$ git checkout master
$ make dep
$ make build
```

### 3.3 Testing locally
Developing and testing local changes to Mattermost operator is fairly simple. For that you can deploy Kind and then apply the manifests to deploy the dependencies and the Mattermost operator as well.

You don't need to push the mattermost-operator image to DockerHub or any other registry if testing with kind. You can load the image, built with `make build-image`, directly to the Kind cluster by running the following:

```bash
$ kind load docker-image mattermost/mattermost-operator:test
```

If you want to use [minikube](https://kubernetes.io/docs/setup/learning-environment/minikube/) for local testing, you need to push the image to the local docker registry of the minikube node to make it available during the deployment. 

You need to have minikube already up and running. 

* Enter the following command into your terminal to use minikube's docker environment:  
`eval $(minikube docker-env)`
* If you now list the available docker images in the local registry you will get the list of docker images from the minikube node
```bash
$ docker images

REPOSITORY                                TAG                 IMAGE ID            CREATED             SIZE
k8s.gcr.io/kube-proxy                     v1.16.2             8454cbe08dc9        2 months ago        86.1MB
k8s.gcr.io/kube-apiserver                 v1.16.2             c2c9a0406787        2 months ago        217MB
k8s.gcr.io/kube-controller-manager        v1.16.2             6e4bffa46d70        2 months ago        163MB
k8s.gcr.io/kube-scheduler                 v1.16.2             ebac1ae204a2        2 months ago        87.3MB
k8s.gcr.io/etcd                           3.3.15-0            b2756210eeab        3 months ago        247MB
k8s.gcr.io/coredns                        1.6.2               bf261d157914        4 months ago        44.1MB
k8s.gcr.io/kube-addon-manager             v9.0.2              bd12a212f9dc        4 months ago        83.1MB
k8s.gcr.io/kube-addon-manager             v9.0                119701e77cbc        11 months ago       83.1MB
k8s.gcr.io/kubernetes-dashboard-amd64     v1.10.1             f9aed6605b81        12 months ago       122MB
k8s.gcr.io/k8s-dns-sidecar-amd64          1.14.13             4b2e93f0133d        15 months ago       42.9MB
k8s.gcr.io/k8s-dns-kube-dns-amd64         1.14.13             55a3c5209c5e        15 months ago       51.2MB
k8s.gcr.io/k8s-dns-dnsmasq-nanny-amd64    1.14.13             6dc8ef8287d3        15 months ago       41.4MB
k8s.gcr.io/pause                          3.1                 da86e6ba6ca1        24 months ago       742kB
gcr.io/k8s-minikube/storage-provisioner   v1.8.1              4689081edb10        2 years ago         80.8MB
```
* Now is the right time to build your new mattermost image with
`make build-image`
* After the build has finishes the new image is available on the minikube node
```bash
$ docker images                  
REPOSITORY                                TAG                 IMAGE ID            CREATED             SIZE
mattermost/mattermost-operator            test                e7f1e78a130b        2 minutes ago       49.6MB
...
```