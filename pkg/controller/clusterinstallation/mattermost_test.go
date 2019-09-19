package clusterinstallation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-operator/pkg/apis"
	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	logmo "github.com/mattermost/mattermost-operator/pkg/log"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func TestCheckMattermost(t *testing.T) {
	// Setup logging for the reconciler so we can see what happened on failure.
	logger := logmo.InitLogger()
	logger = logger.WithName("test.opr")
	logf.SetLogger(logger)

	ciName := "foo"
	ciNamespace := "default"
	replicas := int32(4)
	ci := &mattermostv1alpha1.ClusterInstallation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ciName,
			Namespace: ciNamespace,
			UID:       types.UID("test"),
		},
		Spec: mattermostv1alpha1.ClusterInstallationSpec{
			Replicas:    replicas,
			Image:       "mattermost/mattermost-enterprise-edition",
			Version:     "5.14.0",
			IngressName: "foo.mattermost.dev",
		},
	}

	apis.AddToScheme(scheme.Scheme)
	s := scheme.Scheme
	s.AddKnownTypes(mattermostv1alpha1.SchemeGroupVersion, ci)
	r := &ReconcileClusterInstallation{client: fake.NewFakeClient(), scheme: s}

	err := prepAllDependencyTestResources(r, ci)
	require.NoError(t, err)

	t.Run("service", func(t *testing.T) {
		err = r.checkMattermostService(ci, ci.Name, ci.Name, logger)
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
		err = r.checkMattermostService(ci, ci.Name, ci.Name, logger)
		require.NoError(t, err)
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original, found)
	})

	t.Run("ingress", func(t *testing.T) {
		err = r.checkMattermostIngress(ci, ci.Name, ci.Spec.IngressName, ci.Spec.IngressAnnotations, logger)
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
		err = r.checkMattermostIngress(ci, ci.Name, ci.Spec.IngressName, ci.Spec.IngressAnnotations, logger)
		require.NoError(t, err)
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original, found)
	})

	t.Run("deployment", func(t *testing.T) {
		updateName := "mattermost-update-check"
		now := metav1.Now()
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      updateName,
				Namespace: ci.GetNamespace(),
			},
			Status: batchv1.JobStatus{
				Succeeded:      1,
				CompletionTime: &now,
			},
		}
		err := r.client.Create(context.TODO(), job)
		require.NoError(t, err)

		err = r.checkMattermostDeployment(ci, ci.Name, ci.Spec.IngressName, ci.GetImageName(), logger)
		assert.NoError(t, err)

		found := &appsv1.Deployment{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Labels = nil
		newReplicas := int32(0)
		modified.Spec.Replicas = &newReplicas
		modified.Spec.Template.Spec.Containers[0].Image = "not-mattermost:latest"

		err = r.client.Update(context.TODO(), modified)
		require.NoError(t, err)
		err = r.checkMattermostDeployment(ci, ci.Name, ci.Spec.IngressName, ci.GetImageName(), logger)
		require.NoError(t, err)
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.Labels, found.Labels)
		assert.Equal(t, original.Spec.Replicas, found.Spec.Replicas)
		assert.Equal(t, original.Spec.Template, found.Spec.Template)
	})
}
