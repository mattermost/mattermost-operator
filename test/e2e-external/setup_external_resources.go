package e2e

import (
	"context"
	"log"
	"os/exec"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TestEnv struct {
	DBConfig        mmv1beta.ExternalDatabase
	FileStoreConfig mmv1beta.ExternalFileStore

	CleanupFunc func()
}

type cleanupFunctions []func()

func (cf cleanupFunctions) Cleanup() {
	// Run in reverse order -- the same as defer.
	for i := len(cf) - 1; i >= 0; i-- {
		cf[i]()
	}
}

// SetupTestEnv sets up Minio and Postgres in given namespace.
func SetupTestEnv(k8sClient client.Client, namespace string) (TestEnv, error) {
	ctx := context.Background()
	var cleanupFuncs cleanupFunctions

	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}
	err := k8sClient.Create(ctx, ns)
	if err != nil {
		return TestEnv{}, errors.Wrap(err, "failed to create namespace")
	}
	cleanupNamespace := func() {
		_ = k8sClient.Delete(context.Background(), ns)
	}
	cleanupFuncs = append(cleanupFuncs, cleanupNamespace)

	cleanupPostgres, err := CreateFromFile(ctx, k8sClient, namespace, "../../resources/postgres.yaml")
	if err != nil {
		cleanupFuncs.Cleanup()
		return TestEnv{}, errors.Wrap(err, "failed to apply Postgres")
	}
	cleanupFuncs = append(cleanupFuncs, cleanupPostgres)

	cmd := exec.Command("kubectl", "minio", "init")
	if cmd.Err != nil {
		return TestEnv{}, errors.Wrap(cmd.Err, "failed to init minio")
	}

	cleanupFuncs = append(cleanupFuncs, func() {
		cmd := exec.Command("kubectl", "minio", "delete")
		if cmd.Err != nil {
			log.Printf("error deleting minio: %s", err.Error())
		}
	})

	cleanupSecrets, err := CreateFromFile(ctx, k8sClient, namespace, "../../resources/mm-secrets.yaml")
	if err != nil {
		cleanupFuncs.Cleanup()
		return TestEnv{}, errors.Wrap(err, "failed to apply MM secrets")
	}
	cleanupFuncs = append(cleanupFuncs, cleanupSecrets)

	return TestEnv{
		DBConfig: mmv1beta.ExternalDatabase{Secret: "db-credentials"},
		FileStoreConfig: mmv1beta.ExternalFileStore{
			URL:    "minio:9000",
			Bucket: "test-bucket",
			Secret: "file-store-credentials",
		},
		CleanupFunc: cleanupFuncs.Cleanup,
	}, nil
}
