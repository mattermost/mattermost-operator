# Mattermost Operator for Kubernetes ![CircleCI branch](https://img.shields.io/circleci/project/github/mattermost/mattermost-operator/master.svg) [![Community Server](https://img.shields.io/badge/Mattermost_Community-cloud_channel-blue.svg)](https://community.mattermost.com/core/channels/cloud)

## Status
The Mattermost operator is in alpha, but is being actively developed.

## Summary
Mattermost is a scalable, open source collaboration tool. It's written in Golang and React.

This project offers a Kubernetes Operator for Mattermost to simplify deploying and managing your Mattermost instance.

Learn more about Mattermost at https://mattermost.com.

The Mattermost server source code is available at https://github.com/mattermost/mattermost-server.

## 1. Prerequisites

### 1.1 Kubernetes Cluster with `kubectl`
You must have a running Kubernetes 1.11.0+ cluster. If you do not, then see the [official Kubernetes documentation](https://kubernetes.io/docs/setup/) to get one set up.

In addition you must have [kubectl](https://kubernetes.io/docs/reference/kubectl/overview/) configured for the relevant Kubernetes cluster. 

### 1.2 MySQL Operator
The presslabs/mysql-operator is required if you are not using an external database. To install the MySQL operator run:

```bash
$ kubectl create ns mysql-operator
$ kubectl apply -n mysql-operator -f https://raw.githubusercontent.com/mattermost/mattermost-operator/master/docs/mysql-operator/mysql-operator.yaml
```

### 1.3 Minio Operator
The Minio operator is required. To install the Minio operator run:

```bash
$ kubectl create ns minio-operator
$ kubectl apply -n minio-operator -f https://raw.githubusercontent.com/mattermost/mattermost-operator/master/docs/minio-operator/minio-operator.yaml
```

### 1.4 Mattermost Operator
Finally, install the Mattermost operator with:

```bash
$ kubectl create ns mattermost-operator
$ kubectl apply -n mattermost-operator -f https://raw.githubusercontent.com/mattermost/mattermost-operator/master/docs/mattermost-operator/mattermost-operator.yaml
```

## 2. Install Mattermost

### 2.1 Quick Install on Azure or AWS
If you are running Kubernetes on Azure or AWS, run the following to install Mattermost:

```bash
$ kubectl apply -f https://raw.githubusercontent.com/mattermost/mattermost-operator/master/docs/examples/simple_aws_azure.yaml
```

After a couple minutes, run:
```bash
$ kubectl get clusterinstallation
```

This will show you something similar to:

```
NAME                              STATE    IMAGE                                      VERSION   ENDPOINT
example-mattermost-installation   stable   mattermost/mattermost-enterprise-edition   5.15.0    ab8679c3387b311e9ac2a02c03e3d674-230738344.us-east-1.elb.amazonaws.com
```

Go to the endpoint listed to access your Mattermost instance.

To customize your Mattermost installation, see [section 2.3 Custom Install](#2.3-custom-install).

### 2.2 Quick Install Not on Azure or AWS
If your Kubernetes is running somewhere other than on Azure or AWS, you'll need to install an Ingress controller to access your Mattermost installation. We recommend using the [NGINX Ingress Controller](https://kubernetes.github.io/ingress-nginx/deploy/).

Once your Ingress controller is installed, run:

```bash
$ kubectl apply -f https://raw.githubusercontent.com/mattermost/mattermost-operator/master/docs/examples/simple_anywhere.yaml
```

To get the hostname or IP address to access your Mattermost installation, you need to look at the service for ingress controller. Using NGINX Ingress, you can do that with:

```bash
$ kubectl -n ingress-nginx get svc
```

### 2.3 Custom Install
There are a few options that can specified when installing Mattermost with the operator. To customize your install, first download https://raw.githubusercontent.com/mattermost/mattermost-operator/master/docs/examples/full.yaml into a local file named `mm_clusterinstallation.yaml`.

Open that up in your file editor and edit the fields under `metadata` and `spec`. The descriptions for each setting are present in that file.

Once edited to your satisfaction, save the file and run:

```bash
$ kubectl apply -f mm_clusterinstallation.yaml
```

This will install Mattermost with your desired options.

### 2.4 Uninstall Mattermost
To uninstall a Mattermost installation, simply run `kubectl delete -f` with the file you used to install Mattermost.

For example, from the AWS or Azure quick install:

```bash
$ kubectl delete -f https://raw.githubusercontent.com/mattermost/mattermost-operator/master/docs/examples/simple_aws_azure.yaml
```

For the install anywhere example:

```bash
$ kubectl delete -f https://raw.githubusercontent.com/mattermost/mattermost-operator/master/docs/examples/simple_anywhere.yaml
```

For the customized install options example:

```bash
$ kubectl delete -f mm_clusterinstallation.yaml
```

### 2.5 Restore an existing Mattermost MySQL Database
To restore an existing Mattermost MySQL Database into a new Mattermost installation using the Mattermost Operator you will need to do a few steps.

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
    name: test-restore
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
    restoreSecret: myawscreds
  ```

If you have an machine running MySQL you just need to perform the `Percona XtraBackup` step

## 3. Developer flow
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
$ make dep
$ make install
```

### 3.2 Testing locally
Developing and testing local changes to Mattermost operator is fairly simple. For that you can deploy Kind and then apply the manifests to deploy the dependencies and the Mattermost operator as well.

You don't need to push the mattermost-operator image to DockerHub or any other registry if testing with kind. You can load the image, built with `make build-image`, directly to the Kind cluster by running the following:

```bash
$ kind load docker-image mattermost/mattermost-operator:test
```
