package e2e

import (
	"os"
	"path/filepath"
	"testing"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	v1beta1Minio "github.com/minio/minio-operator/pkg/apis/miniocontroller/v1beta1"
	v1alpha1MySQL "github.com/presslabs/mysql-operator/pkg/apis/mysql/v1alpha1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestMain(m *testing.M) {
	log := zap.New()
	logf.SetLogger(log)

	err := SetupTest()
	if err != nil {
		log.Error(err, "Failed to setup test")
		os.Exit(1)
	}

	defer func() {
		err := Cleanup()
		if err != nil {
			log.Error(err, "Failed to cleanup after test")
			os.Exit(1)
		}
	}()

	code := m.Run()
	os.Exit(code)
}

func SetupTest() error {
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			filepath.Join("..", "crds"),
		},
		UseExistingCluster: boolPtr(true),
	}

	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		return err
	}

	err = mmv1beta.AddToScheme(scheme.Scheme)
	if err != nil {
		return err
	}

	err = v1beta1Minio.AddToScheme(scheme.Scheme)
	if err != nil {
		return err
	}

	err = v1alpha1MySQL.SchemeBuilder.AddToScheme(scheme.Scheme)
	if err != nil {
		return err
	}

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return err
	}

	return nil
}

func Cleanup() error {
	return testEnv.Stop()
}

func boolPtr(b bool) *bool {
	return &b
}
