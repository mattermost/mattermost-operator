package bluegreen

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-operator/pkg/apis"
	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	logmo "github.com/mattermost/mattermost-operator/pkg/log"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func TestCheckBlueGreen(t *testing.T) {
	// Setup logging for the reconciler so we can see what happened on failure.
	logger := logmo.InitLogger()
	logger = logger.WithName("test.opr")
	logf.SetLogger(logger)

	ciName := "foo-green"
	ciNamespace := "default"
	ci := &mattermostv1alpha1.BlueGreen{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ciName,
			Namespace: ciNamespace,
			UID:       types.UID("test"),
		},
		Spec: mattermostv1alpha1.BlueGreenSpec{
			InstallationName:    "mm-foo",
			Version:     "5.11.0",
			IngressName: "foo.mattermost.dev",
		},
	}

	mattermostName := "foo"
	mattermostNamespace := "default"
	replicas := int32(4)
	mattermost := &mattermostv1alpha1.ClusterInstallation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mattermostName,
			Namespace: mattermostNamespace,
			UID:       types.UID("test"),
		},
		Spec: mattermostv1alpha1.ClusterInstallationSpec{
			Replicas:    replicas,
			Image:       "mattermost/mattermost-enterprise-edition",
			Version:     "5.11.0",
			IngressName: "foo.mattermost.dev",
		},
	}

	apis.AddToScheme(scheme.Scheme)
	s := scheme.Scheme
	s.AddKnownTypes(mattermostv1alpha1.SchemeGroupVersion, ci)
	r := &ReconcileBlueGreen{client: fake.NewFakeClient(), scheme: s}

	t.Run("service", func(t *testing.T) {
		err := r.checkBlueGreenService(ci, mattermost, logger)
		assert.NoError(t, err)

		found := &corev1.Service{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Labels = nil
		modified.Annotations = nil
		modified.Spec = corev1.ServiceSpec{}

		err = r.client.Update(context.TODO(), modified)
		require.NoError(t, err)
		err = r.checkBlueGreenService(ci, mattermost, logger)
		require.NoError(t, err)
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original, found)
	})

	t.Run("ingress", func(t *testing.T) {
		err := r.checkBlueGreenIngress(ci, logger)
		assert.NoError(t, err)

		found := &v1beta1.Ingress{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Labels = nil
		modified.Annotations = nil
		modified.Spec = v1beta1.IngressSpec{}

		err = r.client.Update(context.TODO(), modified)
		require.NoError(t, err)
		err = r.checkBlueGreenIngress(ci, logger)
		require.NoError(t, err)
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original, found)
	})
}
