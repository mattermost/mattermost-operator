module github.com/mattermost/mattermost-operator

go 1.14

require (
	github.com/banzaicloud/k8s-objectmatcher v1.7.0
	github.com/evanphx/json-patch v4.12.0+incompatible
	github.com/go-logr/logr v1.2.2
	github.com/mattermost/blubr v0.0.0-20220302140450-2f38b057ee02
	github.com/mattn/goveralls v0.0.7
	github.com/mikefarah/yq/v3 v3.0.0-20200916054308-65cb4726048d
	github.com/minio/minio-operator v0.0.0-20200214142425-158e343f1f19
	github.com/pborman/uuid v1.2.1
	github.com/pkg/errors v0.9.1
	github.com/presslabs/mysql-operator v0.5.0-rc.2
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.0
	github.com/vrischmann/envconfig v1.3.0
	golang.org/x/net v0.0.0-20210825183410-e898025ed96a
	k8s.io/api v0.23.0
	k8s.io/apimachinery v0.23.0
	k8s.io/client-go v0.23.0
	k8s.io/code-generator v0.23.0
	k8s.io/kube-openapi v0.0.0-20211115234752-e816edb12b65
	sigs.k8s.io/controller-runtime v0.11.0
)
