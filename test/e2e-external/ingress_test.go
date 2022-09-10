package e2e

import (
	"context"
	"testing"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	pkgUtils "github.com/mattermost/mattermost-operator/pkg/utils"
	"github.com/mattermost/mattermost-operator/test/e2e"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestMattermostCustomIngressOk Check that setting custom values in the mattermost ingress are
func TestMattermostCustomIngressOk(t *testing.T) {
	namespace := "e2e-test-custom-ingress"
	name := "test-mm"

	testEnv, err := SetupTestEnv(k8sClient, namespace)
	require.NoError(t, err)
	defer testEnv.CleanupFunc()

	mattermost := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: mmv1beta.MattermostSpec{
			Ingress: &mmv1beta.Ingress{
				Host: "e2e-test-ingress.mattermost.dev",
				Hosts: []mmv1beta.IngressHost{
					{
						HostName: "e2e-test-ingress-1.mattermost.dev",
					},
					{
						HostName: "e2e-test-ingress-2.mattermost.dev",
					},
				},
				Annotations: map[string]string{
					"mattermost.test": "yes",
				},
				IngressClass: pkgUtils.NewString("custom-ingress-class"),
			},
			Replicas: pkgUtils.NewInt32(1),
			FileStore: mmv1beta.FileStore{
				External: &testEnv.FileStoreConfig,
			},
			Database: mmv1beta.Database{
				External: &testEnv.DBConfig,
			},
		},
	}

	instance := e2e.NewMattermostInstance(t, k8sClient, mattermost)
	defer instance.Destroy()

	instance.CreateAndWait()

	clusterMattermost := instance.Get()

	// Check entire ingress object
	require.Equal(t, mattermost.Spec.Ingress, clusterMattermost.Spec.Ingress, "Mattermost Ingress spec should be the same as defined")

	// Check specific attributes individually, in case deep equality fails for wathever reason
	require.Equal(t, mattermost.Spec.Ingress.Hosts, clusterMattermost.Spec.Ingress.Hosts, "Spec should have same hosts defined")
	require.Equal(t, mattermost.Spec.Ingress.Annotations, clusterMattermost.Spec.Ingress.Annotations, "Spec should contain specified annotations")
	require.Equal(t, mattermost.Spec.Ingress.IngressClass, clusterMattermost.Spec.Ingress.IngressClass, "Spec should contain the same ingress class")
}

func TestMattermostIngressDisableOk(t *testing.T) {
	namespace := "e2e-test-ingress-disable"
	name := "test-mm"

	testEnv, err := SetupTestEnv(k8sClient, namespace)
	require.NoError(t, err)
	defer testEnv.CleanupFunc()

	mattermost := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: mmv1beta.MattermostSpec{
			Ingress: &mmv1beta.Ingress{
				Enabled: true,
				Host:    namespace + ".mattermost.dev",
			},
			Replicas: pkgUtils.NewInt32(1),
			FileStore: mmv1beta.FileStore{
				External: &testEnv.FileStoreConfig,
			},
			Database: mmv1beta.Database{
				External: &testEnv.DBConfig,
			},
		},
	}

	ctx := context.Background()
	instance := e2e.NewMattermostInstance(t, k8sClient, mattermost)
	defer instance.Destroy()

	instance.CreateAndWait()

	// Ensure the instance is created and with the ingress enabled and ingress object created
	clusterMattermost := instance.Get()
	require.NotNil(t, clusterMattermost.Spec.Ingress, "ingress should be defined")
	require.True(t, clusterMattermost.IngressEnabled(), "ingress should be enabled at the beginning")

	// Check that the ingress object is created
	var mmIngress networkingv1.Ingress
	err = k8sClient.Get(ctx, instance.Namespace(), &mmIngress)
	require.NoError(t, err, "Ingress should be present in cluster")
	require.Equal(t, name, mmIngress.Name)

	// Disable the ingress and update the instance
	clusterMattermost.Spec.Ingress.Enabled = false
	instance.UpdateAndWait(&clusterMattermost)

	// Ensure the ingress object has been removed
	err = k8sClient.Get(ctx, instance.Namespace(), &mmIngress)
	require.True(t, k8serrors.IsNotFound(err), "Ingress should be deleted after object update")

	// Ensure the mattermost instance has the ingress disabled
	clusterMattermost = instance.Get()
	require.False(t, clusterMattermost.IngressEnabled(), "ingress should be disabled after resource update")
}
