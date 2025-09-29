package e2e

import (
	"context"
	"testing"
	"time"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	ptrUtil "github.com/mattermost/mattermost-operator/pkg/utils"
	"github.com/mattermost/mattermost-operator/test/e2e"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestMattermostExternalServices(t *testing.T) {
	t.Log("Running tests with external Mattermost Services")

	t.Run("mattermost operator ready", func(t *testing.T) {
		err := waitForDeploymentWithDefaultTimes(t, k8sClient, mattermostOperatorNamespacedName, 1)
		require.NoError(t, err)
	})

	t.Run("mattermost base test", func(t *testing.T) {
		mattermostBaseTest(t)
	})

	t.Run("mattermost upgrade test", func(t *testing.T) {
		mattermostUpgradeTest(t)
	})

	t.Run("mattermost size test", func(t *testing.T) {
		mattermostSizeTest(t)
	})

	t.Run("mattermost ingress test", func(t *testing.T) {
		mattermostIngressTest(t)
	})
}

func mattermostBaseTest(t *testing.T) {
	namespace := "e2e-test-external-services"

	testEnv, err := SetupTestEnv(k8sClient, namespace)
	require.NoError(t, err)
	defer testEnv.CleanupFunc()

	mattermost := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mm",
			Namespace: namespace,
		},
		Spec: mmv1beta.MattermostSpec{
			Ingress: &mmv1beta.Ingress{
				Host: "e2e-test-example.mattermost.dev",
			},
			Replicas: ptrUtil.NewInt32(1),
			FileStore: mmv1beta.FileStore{
				External: &testEnv.FileStoreConfig,
			},
			Database: mmv1beta.Database{
				External: &testEnv.DBConfig,
			},
		},
	}

	expectValidMattermostInstance(t, mattermost)
}

func expectValidMattermostInstance(t *testing.T, mattermost *mmv1beta.Mattermost) {
	mmNamespaceName := types.NamespacedName{Namespace: mattermost.Namespace, Name: mattermost.Name}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := k8sClient.Create(ctx, mattermost)
	require.NoError(t, err)

	err = e2e.WaitForMattermostStable(t, k8sClient, mmNamespaceName, 3*time.Minute)
	require.NoError(t, err)

	// TODO: Run some basic Mattermost functionality test here
	// this most likely needs to be done from inside the cluster
	// by running some job.
}
