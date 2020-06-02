module github.com/mattermost/mattermost-operator

go 1.14

require (
	github.com/banzaicloud/k8s-objectmatcher v1.1.0
	github.com/go-logr/logr v0.1.0
	github.com/go-openapi/spec v0.19.4
	github.com/mattermost/blubr v0.0.0-20200113232543-f0ce67760aeb
	github.com/minio/minio-operator v0.0.0-20200214142425-158e343f1f19
	github.com/operator-framework/operator-sdk v0.17.1
	github.com/pborman/uuid v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/presslabs/mysql-operator v0.3.8
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.4.0
	golang.org/x/net v0.0.0-20200226121028-0de0cce0169b
	golang.org/x/tools v0.0.0-20200522201501-cb1345f3a375 // indirect
	k8s.io/api v0.17.4
	k8s.io/apimachinery v0.17.4
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/code-generator v0.17.4
	k8s.io/kube-openapi v0.0.0-20191107075043-30be4d16710a
	sigs.k8s.io/controller-runtime v0.5.2

)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	k8s.io/client-go => k8s.io/client-go v0.17.4 // Required by prometheus-operator
)
