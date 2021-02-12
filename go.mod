module github.com/mattermost/mattermost-operator

go 1.14

require (
	github.com/banzaicloud/k8s-objectmatcher v1.4.1
	github.com/go-logr/logr v0.1.0
	github.com/go-openapi/spec v0.19.9
	github.com/mattermost/blubr v0.0.0-20200113232543-f0ce67760aeb
	github.com/mattn/goveralls v0.0.7
	github.com/mikefarah/yq/v3 v3.0.0-20200916054308-65cb4726048d
	github.com/minio/minio-operator v0.0.0-20200214142425-158e343f1f19
	github.com/pborman/uuid v1.2.1
	github.com/pkg/errors v0.9.1
	github.com/presslabs/mysql-operator v0.4.0
	github.com/stretchr/testify v1.7.0
	github.com/vrischmann/envconfig v1.3.0
	golang.org/x/net v0.0.0-20200625001655-4c5254603344
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
	k8s.io/api v0.18.15
	k8s.io/apimachinery v0.18.15
	k8s.io/client-go v0.18.15
	k8s.io/code-generator v0.18.15
	k8s.io/kube-openapi v0.0.0-20200410145947-61e04a5be9a6
	sigs.k8s.io/controller-runtime v0.6.5
)
