package e2e

import (
	"context"
	"path/filepath"
	"time"

	"github.com/pkg/errors"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
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

	return TestEnvironment{
		TestEnv:   testEnv,
		Cfg:       cfg,
		K8sClient: k8sClient,
	}, nil
}

func boolPtr(b bool) *bool {
	return &b
}

// SetupMattermostPrerequisites creates the database and file store Mattermost
// requires. It applies the same fixtures the e2e-external suite uses: a Postgres
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

	// CreateFromFile returns as soon as the objects are created, not once they
	// serve traffic, so wait for both Deployments before any Mattermost is created.
	//
	// This is not merely tidiness. mm-secrets.yaml carries DB_CONNECTION_STRING but
	// no DB_CONNECTION_CHECK_URL, and the Operator only injects the database
	// readiness init container when that key is present. Nothing else blocks on the
	// database, so without this wait the Mattermost pods start against a Postgres
	// that is not listening and recover only via crash-loop backoff.
	for _, deployment := range []string{"postgresql", "minio"} {
		if err := waitForDeploymentAvailable(ctx, k8sClient, namespace, deployment); err != nil {
			cleanupAll()
			return func() {}, errors.Wrapf(err, "%s deployment never became available", deployment)
		}
	}

	return cleanupAll, nil
}

// waitForDeploymentAvailable blocks until the named Deployment reports at least
// one available replica.
func waitForDeploymentAvailable(ctx context.Context, k8sClient client.Client, namespace, name string) error {
	return wait.PollUntilContextTimeout(ctx, 5*time.Second, 5*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			var deployment appsv1.Deployment
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &deployment)
			if err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				return false, err
			}

			return deployment.Status.AvailableReplicas >= 1, nil
		})
}
