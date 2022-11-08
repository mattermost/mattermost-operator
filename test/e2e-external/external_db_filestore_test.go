package e2e

import (
	"context"
	"testing"
	"time"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	ptrUtil "github.com/mattermost/mattermost-operator/pkg/utils"
	operatortest "github.com/mattermost/mattermost-operator/test"
	"github.com/mattermost/mattermost-operator/test/e2e"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func Test_MattermostExternalServices(t *testing.T) {
	namespace := "e2e-test-external-db-file-store"

	testEnv, err := SetupTestEnv(k8sClient, namespace)
	require.NoError(t, err)
	defer testEnv.CleanupFunc()

	mattermost := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mm",
			Namespace: namespace,
		},
		Spec: mmv1beta.MattermostSpec{
			Version: operatortest.PreviousStableMattermostVersion,
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
	checkVersionUpgrade(t, mattermost)
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

func checkVersionUpgrade(t *testing.T, mattermost *mmv1beta.Mattermost) {
	mmNamespaceName := types.NamespacedName{Namespace: mattermost.Namespace, Name: mattermost.Name}

	newMattermost := &mmv1beta.Mattermost{}
	err := k8sClient.Get(context.TODO(), mmNamespaceName, newMattermost)
	require.NoError(t, err)

	// Upgrade to new version
	newMattermost.Spec.Version = operatortest.LatestStableMattermostVersion
	err = k8sClient.Update(context.TODO(), newMattermost)
	require.NoError(t, err)

	// Wait for reconciliation start.
	err = e2e.WaitForMattermostToReconcile(t, k8sClient, mmNamespaceName, 3*time.Minute)
	require.NoError(t, err)

	// Wait for mattermost to be stable again.
	err = e2e.WaitForMattermostStable(t, k8sClient, mmNamespaceName, 3*time.Minute)
	require.NoError(t, err)

	var mmDeployment appsv1.Deployment
	err = k8sClient.Get(context.TODO(), mmNamespaceName, &mmDeployment)
	require.NoError(t, err)
	// check if deployment has the new version
	require.Equal(t, "mattermost/mattermost-enterprise-edition:"+operatortest.LatestStableMattermostVersion,
		mmDeployment.Spec.Template.Spec.Containers[0].Image)
}
