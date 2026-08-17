package e2e

import (
	"context"
	"path/filepath"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/pkg/errors"
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

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return TestEnvironment{}, err
	}

	// Provision db-credentials secret and RWX-capable storage for the standard suite
	err = provisionTestPrerequisites(k8sClient, mmNamespace)
	if err != nil {
		return TestEnvironment{}, errors.Wrap(err, "failed to provision test prerequisites")
	}

	return TestEnvironment{
		TestEnv:   testEnv,
		Cfg:       cfg,
		K8sClient: k8sClient,
	}, nil
}

func provisionTestPrerequisites(k8sClient client.Client, namespace string) error {
	ctx := context.Background()

	// Apply postgres to provision the external database
	_, err := CreateFromFile(ctx, k8sClient, namespace, "../../resources/postgres.yaml")
	if err != nil {
		return errors.Wrap(err, "failed to apply postgres")
	}

	// Apply mm-secrets to provision db-credentials secret
	_, err = CreateFromFile(ctx, k8sClient, namespace, "../../resources/mm-secrets.yaml")
	if err != nil {
		return errors.Wrap(err, "failed to apply mm-secrets")
	}

	return nil
}

func boolPtr(b bool) *bool {
	return &b
}
