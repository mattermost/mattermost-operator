package e2e

import (
	"context"
	"path/filepath"

	"github.com/pkg/errors"

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
	// test/crds held the MinIO and MySQL operator CRDs. Both integrations are gone,
	// so only this repository's own CRDs remain.
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
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

// SetupMattermostPrerequisites creates the database and file store a Mattermost
// now requires. The Operator used to provision both itself via the MySQL and
// MinIO operators; since it no longer does, the suite has to supply them.
//
// It applies the same fixtures the e2e-external suite uses: a Postgres
// Deployment, a standalone MinIO acting as an external S3 endpoint, and the
// db-credentials and file-store-credentials Secrets. The returned function
// removes them again.
func SetupMattermostPrerequisites(ctx context.Context, k8sClient client.Client, namespace string) (func(), error) {
	var cleanups []func()

	cleanupAll := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}

	for _, fixture := range []string{
		filepath.Join("..", "..", "resources", "postgres.yaml"),
		filepath.Join("..", "..", "resources", "minio.yaml"),
		filepath.Join("..", "..", "resources", "mm-secrets.yaml"),
	} {
		cleanup, err := CreateFromFile(ctx, k8sClient, namespace, fixture)
		if err != nil {
			cleanupAll()
			return func() {}, errors.Wrapf(err, "failed to apply %s", fixture)
		}
		cleanups = append(cleanups, cleanup)
	}

	return cleanupAll, nil
}
