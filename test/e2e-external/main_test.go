package e2e

import (
	"os"
	"testing"

	"github.com/mattermost/mattermost-operator/test/e2e"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var k8sClient client.Client
var testEnv *envtest.Environment

func TestMain(m *testing.M) {
	log := zap.New()
	logf.SetLogger(log)

	err := setup()
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

func setup() error {
	env, err := e2e.SetupTest()
	if err != nil {
		return err
	}

	k8sClient = env.K8sClient
	testEnv = env.TestEnv

	return nil
}

func Cleanup() error {
	return testEnv.Stop()
}
