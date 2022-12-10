package e2e

import (
	"path/filepath"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	minioOperator "github.com/minio/operator/pkg/apis/minio.min.io/v2"
	v1alpha1MySQL "github.com/presslabs/mysql-operator/pkg/apis/mysql/v1alpha1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type TestEnvironment struct {
	TestEnv   *envtest.Environment
	Cfg       *rest.Config
	K8sClient client.Client
}

func SetupTest() (TestEnvironment, error) {
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			filepath.Join("..", "crds"),
		},
		UseExistingCluster: boolPtr(true),
	}

	cfg, err := testEnv.Start()
	if err != nil {
		return TestEnvironment{}, err
	}

	err = mmv1beta.AddToScheme(scheme.Scheme)
	if err != nil {
		return TestEnvironment{}, err
	}

	err = minioOperator.AddToScheme(scheme.Scheme)
	if err != nil {
		return TestEnvironment{}, err
	}

	err = v1alpha1MySQL.SchemeBuilder.AddToScheme(scheme.Scheme)
	if err != nil {
		return TestEnvironment{}, err
	}

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return TestEnvironment{}, err
	}

	return TestEnvironment{
		TestEnv:   testEnv,
		Cfg:       cfg,
		K8sClient: k8sClient,
	}, nil
}

func boolPtr(b bool) *bool {
	return &b
}
