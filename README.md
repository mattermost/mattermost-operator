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

### 3.2 Building mattermost-operator
To start contributing to mattermost-operator you need to clone this repo to your local workspace. 

```bash
$ mkdir -p $GOPATH/src/github.com/mattermost
$ cd $GOPATH/src/github.com/mattermost
$ git clone https://github.com/mattermost/mattermost-operator
$ cd mattermost-operator
$ git checkout master
$ make build
```

### 3.3 Testing locally
Developing and testing local changes to Mattermost operator is fairly simple. For that you can deploy Kind and then apply the manifests to deploy the dependencies and the Mattermost operator as well.

You don't need to push the mattermost-operator image to DockerHub or any other registry if testing with kind. You can load the image, built with `make build-image`, directly to the Kind cluster by running the following:

```bash
$ kind load docker-image mattermost/mattermost-operator:test
```

## 4 Notes

### 4.1 Installation Size

The `spec.Size` field was modified to be treated as a write-only field. 
After adjusting values according to the size, the value of `spec.Size` is erased. 

Replicas and resource requests/limits values can be overridden manually but setting new Size will override those values again regardless if set by the previous Size or adjusted manually. 

