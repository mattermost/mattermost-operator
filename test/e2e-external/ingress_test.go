package e2e

import (
	"testing"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	pkgUtils "github.com/mattermost/mattermost-operator/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	instance := NewMattermostInstance(t, k8sClient, mattermost)
	defer instance.Destroy()

	instance.CreateAndWait()

	clusterMattermost := instance.Get()

	// Check entire ingress object
	assert.Equal(t, mattermost.Spec.Ingress, clusterMattermost.Spec.Ingress, "Mattermost Ingress spec should be the same as defined")

	// Check specific attributes individually, in case deep equality fails for wathever reason
	assert.Equal(t, mattermost.Spec.Ingress.Hosts, clusterMattermost.Spec.Ingress.Hosts, "Spec should have same hosts defined")
	assert.Equal(t, mattermost.Spec.Ingress.Annotations, clusterMattermost.Spec.Ingress.Annotations, "Spec should contain specified annotations")
	assert.Equal(t, mattermost.Spec.Ingress.IngressClass, clusterMattermost.Spec.Ingress.IngressClass, "Spec should contain the same ingress class")
}
