module github.com/mattermost/mattermost-operator

go 1.13

require (
	cloud.google.com/go v0.52.0 // indirect
	github.com/banzaicloud/k8s-objectmatcher v1.1.0
	github.com/coreos/prometheus-operator v0.35.0 // indirect
	github.com/go-logr/logr v0.1.0
	github.com/go-openapi/spec v0.19.5
	github.com/go-openapi/swag v0.19.6 // indirect
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/json-iterator/go v1.1.9 // indirect
	github.com/mattermost/blubr v0.0.0-20200113232543-f0ce67760aeb
	github.com/minio/minio-operator v0.0.0-20200123180224-604167ea8cdb
	github.com/operator-framework/operator-sdk v0.15.1
	github.com/pborman/uuid v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/presslabs/mysql-operator v0.3.8
	github.com/prometheus/client_golang v1.3.0 // indirect
	github.com/prometheus/common v0.9.1 // indirect
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.4.0
	go.uber.org/atomic v1.5.1 // indirect
	go.uber.org/multierr v1.4.0 // indirect
	go.uber.org/zap v1.13.0 // indirect
	golang.org/x/crypto v0.0.0-20200117160349-530e935923ad // indirect
	golang.org/x/net v0.0.0-20200114155413-6afb5195e5aa
	golang.org/x/sys v0.0.0-20200122134326-e047566fdf82 // indirect
	golang.org/x/tools v0.0.0-20200124144541-e0a7ba33caea // indirect
	k8s.io/api v0.17.2
	k8s.io/apimachinery v0.16.7
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/code-generator v0.16.7
	k8s.io/kube-openapi v0.0.0-20200121204235-bf4fb3bd569c
	k8s.io/utils v0.0.0-20200122174043-1e243dd1a584 // indirect
	sigs.k8s.io/controller-runtime v0.4.0

)

// Pinned to kubernetes-1.16.7
replace (
	github.com/docker/docker => github.com/moby/moby v0.7.3-0.20190826074503-38ab9da00309
	k8s.io/api => k8s.io/api v0.16.7
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.16.7
	k8s.io/apimachinery => k8s.io/apimachinery v0.16.8-beta.0
	k8s.io/apiserver => k8s.io/apiserver v0.16.7
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.16.7
	k8s.io/client-go => k8s.io/client-go v0.16.7
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.16.7
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.16.7
	k8s.io/code-generator => k8s.io/code-generator v0.16.8-beta.0
	k8s.io/component-base => k8s.io/component-base v0.16.7
	k8s.io/cri-api => k8s.io/cri-api v0.16.8-beta.0
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.16.7
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.16.7
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.16.7
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.16.7
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.16.7
	k8s.io/kubectl => k8s.io/kubectl v0.16.7
	k8s.io/kubelet => k8s.io/kubelet v0.16.7
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.16.7
	k8s.io/metrics => k8s.io/metrics v0.16.7
	k8s.io/node-api => k8s.io/node-api v0.16.7
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.16.7
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.16.7
	k8s.io/sample-controller => k8s.io/sample-controller v0.16.7
)
