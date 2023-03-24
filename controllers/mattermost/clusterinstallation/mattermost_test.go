package clusterinstallation

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/mattermost/mattermost-operator/pkg/resources"
	"github.com/sirupsen/logrus"

	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	blubr "github.com/mattermost/blubr"
	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	mattermostmysql "github.com/mattermost/mattermost-operator/pkg/components/mysql"
	"github.com/mattermost/mattermost-operator/pkg/database"
	operatortest "github.com/mattermost/mattermost-operator/test"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestCheckMattermost(t *testing.T) {
	// Setup logging for the reconciler so we can see what happened on failure.
	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logSink = logSink.WithName("test.opr")
	logger := logr.New(logSink)
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
			Version:     operatortest.LatestStableMattermostVersion,
			IngressName: "foo.mattermost.dev",
		},
	}

	s := prepareSchema(t, scheme.Scheme)
	s.AddKnownTypes(mattermostv1alpha1.GroupVersion, ci)
	c := fake.NewClientBuilder().Build()
	r := &ClusterInstallationReconciler{
		Client:         c,
		Scheme:         s,
		Log:            logger,
		MaxReconciling: 5,
		Resources:      resources.NewResourceHelper(c, s),
	}

	err := prepAllDependencyTestResources(r.Client, ci)
	require.NoError(t, err)

	t.Run("service", func(t *testing.T) {
		err = r.checkMattermostService(ci, ci.Name, ci.Name, logger)
		assert.NoError(t, err)

		found := &corev1.Service{}
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Labels = nil
		modified.Annotations = nil
		modified.Spec = corev1.ServiceSpec{}

		err = r.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		err = r.checkMattermostService(ci, ci.Name, ci.Name, logger)
		require.NoError(t, err)
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Spec.Selector, found.Spec.Selector)
		assert.Equal(t, original.Spec.Ports, found.Spec.Ports)
	})

	t.Run("service account", func(t *testing.T) {
		err = r.checkMattermostSA(ci, ci.Name, logger)
		assert.NoError(t, err)

		found := &corev1.ServiceAccount{}
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		err = r.Client.Delete(context.TODO(), found)
		require.NoError(t, err)
		err = r.checkMattermostSA(ci, ci.Name, logger)
		require.NoError(t, err)
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
	})

	t.Run("role", func(t *testing.T) {
		err = r.checkMattermostRole(ci, ci.Name, logger)
		assert.NoError(t, err)

		found := &rbacv1.Role{}
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Rules = nil

		err = r.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		err = r.checkMattermostRole(ci, ci.Name, logger)
		require.NoError(t, err)
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Rules, found.Rules)
	})

	t.Run("role binding", func(t *testing.T) {
		err = r.checkMattermostRoleBinding(ci, ci.Name, ci.Name, logger)
		assert.NoError(t, err)

		found := &rbacv1.RoleBinding{}
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Subjects = nil

		err = r.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		err = r.checkMattermostRoleBinding(ci, ci.Name, ci.Name, logger)
		require.NoError(t, err)
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Subjects, found.Subjects)
	})

	t.Run("ingress no tls", func(t *testing.T) {
		ci.Spec.UseIngressTLS = false
		err = r.checkMattermostIngress(ci, ci.Name, ci.Spec.IngressName, ci.Spec.IngressAnnotations, logger)
		assert.NoError(t, err)

		found := &v1beta1.Ingress{}
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)
		require.Nil(t, found.Spec.TLS)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Labels = nil
		modified.Annotations = nil
		modified.Spec = v1beta1.IngressSpec{}

		err = r.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		err = r.checkMattermostIngress(ci, ci.Name, ci.Spec.IngressName, ci.Spec.IngressAnnotations, logger)
		require.NoError(t, err)
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetAnnotations(), found.GetAnnotations())
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Spec.Rules, found.Spec.Rules)
	})

	t.Run("ingress with tls", func(t *testing.T) {
		ci.Spec.UseIngressTLS = true
		ci.Spec.IngressAnnotations = map[string]string{
			"kubernetes.io/ingress.class": "nginx-test",
			"test-ingress":                "blabla",
		}

		err = r.checkMattermostIngress(ci, ci.Name, ci.Spec.IngressName, ci.Spec.IngressAnnotations, logger)
		assert.NoError(t, err)

		found := &v1beta1.Ingress{}
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)
		require.NotNil(t, found.Spec.TLS)
		require.NotNil(t, found.Annotations)
		assert.Contains(t, found.Annotations, "kubernetes.io/ingress.class")

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Labels = nil
		modified.Annotations = nil
		modified.Spec = v1beta1.IngressSpec{}

		err = r.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		err = r.checkMattermostIngress(ci, ci.Name, ci.Spec.IngressName, ci.Spec.IngressAnnotations, logger)
		require.NoError(t, err)
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetAnnotations(), found.GetAnnotations())
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Spec.Rules, found.Spec.Rules)
		assert.Equal(t, original.Spec.TLS, original.Spec.TLS)
	})

	t.Run("deployment", func(t *testing.T) {
		updateName := "mattermost-update-check"
		now := metav1.Now()
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      updateName,
				Namespace: ci.GetNamespace(),
			},
			Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: mattermostv1alpha1.MattermostAppContainerName, Image: ci.GetImageName()},
					},
				},
			}},
			Status: batchv1.JobStatus{
				Succeeded:      1,
				CompletionTime: &now,
			},
		}
		err = r.Client.Create(context.TODO(), job)
		require.NoError(t, err)

		err = r.checkMattermostDeployment(ci, ci.Name, ci.Spec.IngressName, ci.Name, ci.GetImageName(), logger)
		assert.NoError(t, err)

		// TODO: uncomment when enabling back the db setup job
		//dbSetupJob := &batchv1.Job{}
		//err = r.Client.Get(context.TODO(), types.NamespacedName{Name: mattermost.SetupJobName, Namespace: ciNamespace}, dbSetupJob)
		//require.NoError(t, err)
		//require.Equal(t, 1, len(dbSetupJob.Spec.Template.Spec.Containers))
		//require.Equal(t, ci.GetImageName(), dbSetupJob.Spec.Template.Spec.Containers[0].Image)
		//_, containerFound := findContainer(mattermost.WaitForDBSetupContainerName, dbSetupJob.Spec.Template.Spec.InitContainers)
		//require.False(t, containerFound)

		found := &appsv1.Deployment{}
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, ci.Name, found.Spec.Template.Spec.ServiceAccountName)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Labels = nil
		newReplicas := int32(0)
		modified.Spec.Replicas = &newReplicas
		modified.Spec.Template.Spec.Containers[0].Image = "not-mattermost:latest"

		err = r.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		err = r.checkMattermostDeployment(ci, ci.Name, ci.Spec.IngressName, ci.Name, ci.GetImageName(), logger)
		require.NoError(t, err)
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetLabels(), found.GetLabels())
		assert.Equal(t, original.Spec.Replicas, found.Spec.Replicas)
		assert.Equal(t, original.Spec.Template, found.Spec.Template)
	})

	t.Run("final check", func(t *testing.T) {
		t.Run("database secret", func(t *testing.T) {
			dbSecret := &corev1.Secret{}
			err = r.Client.Get(context.TODO(), types.NamespacedName{Name: mattermostmysql.DefaultDatabaseSecretName(ciName), Namespace: ciNamespace}, dbSecret)
			require.NoError(t, err)

			dbInfo := database.GenerateDatabaseInfoFromSecret(dbSecret)
			require.NoError(t, dbInfo.IsValid())
		})
	})

	t.Run("restart update job", func(t *testing.T) {
		// create deployment
		err = r.checkMattermostDeployment(ci, ci.Name, ci.Spec.IngressName, ci.Name, ci.GetImageName(), logger)
		assert.NoError(t, err)

		// create update job with invalid image
		updateName := "mattermost-update-check"
		now := metav1.Now()
		invalidUpdateJob := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      updateName,
				Namespace: ci.GetNamespace(),
			},
			Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: mattermostv1alpha1.MattermostAppContainerName, Image: "invalid-image"},
					},
				},
			}},
			Status: batchv1.JobStatus{
				Succeeded:      1,
				CompletionTime: &now,
			},
		}
		err := r.Client.Create(context.TODO(), invalidUpdateJob)
		require.NoError(t, err)

		// should delete update job and create new and return error
		newImage := "mattermost/new-image"
		err = r.checkMattermostDeployment(ci, ci.Name, ci.Spec.IngressName, ci.Name, newImage, logger)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Restarted update image job")

		// get new job, assert new image and change status to completed
		restartedUpdateJob := batchv1.Job{}
		updateJobKey := types.NamespacedName{Namespace: ci.GetNamespace(), Name: updateJobName}
		err = r.Client.Get(context.TODO(), updateJobKey, &restartedUpdateJob)
		require.NoError(t, err)
		assert.Equal(t, newImage, restartedUpdateJob.Spec.Template.Spec.Containers[0].Image)
		assert.Equal(t, int32(0), restartedUpdateJob.Status.Succeeded)

		restartedUpdateJob.Status = batchv1.JobStatus{
			Succeeded:      1,
			CompletionTime: &now,
		}

		err = r.Client.Update(context.TODO(), &restartedUpdateJob)
		require.NoError(t, err)

		// should succeed now
		err = r.checkMattermostDeployment(ci, ci.Name, ci.Spec.IngressName, ci.Name, newImage, logger)
		assert.NoError(t, err)
	})
}

func TestCheckMattermostExternalDB(t *testing.T) {
	// Setup logging for the reconciler so we can see what happened on failure.
	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logSink = logSink.WithName("test.opr")
	logger := logr.New(logSink)
	logf.SetLogger(logger)

	ciName := "foo"
	ciNamespace := "default"
	replicas := int32(4)
	externalDBSecretName := "externalDB"
	ci := &mattermostv1alpha1.ClusterInstallation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ciName,
			Namespace: ciNamespace,
			UID:       types.UID("test"),
		},
		Spec: mattermostv1alpha1.ClusterInstallationSpec{
			Replicas:    replicas,
			Image:       "mattermost/mattermost-enterprise-edition",
			Version:     operatortest.LatestStableMattermostVersion,
			IngressName: "foo.mattermost.dev",
			Database: mattermostv1alpha1.Database{
				Secret: externalDBSecretName,
			},
		},
	}

	s := prepareSchema(t, scheme.Scheme)
	s.AddKnownTypes(mattermostv1alpha1.GroupVersion, ci)
	c := fake.NewClientBuilder().Build()
	r := &ClusterInstallationReconciler{
		Client:              c,
		Log:                 logger,
		Scheme:              s,
		MaxReconciling:      5,
		RequeueOnLimitDelay: 0,
		Resources:           resources.NewResourceHelper(c, s),
	}

	err := prepAllDependencyTestResources(r.Client, ci)
	require.NoError(t, err)

	externalDBSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalDBSecretName,
			Namespace: ciNamespace,
		},
		Data: map[string][]byte{
			"DB_CONNECTION_STRING": []byte("mysql://test"),
		},
	}
	err = r.Client.Create(context.TODO(), externalDBSecret)
	require.NoError(t, err)

	t.Run("service", func(t *testing.T) {
		err = r.checkMattermostService(ci, ci.Name, ci.Name, logger)
		assert.NoError(t, err)

		found := &corev1.Service{}
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Labels = nil
		modified.Annotations = nil
		modified.Spec = corev1.ServiceSpec{}

		err = r.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		err = r.checkMattermostService(ci, ci.Name, ci.Name, logger)
		require.NoError(t, err)
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Spec.Selector, found.Spec.Selector)
		assert.Equal(t, original.Spec.Ports, found.Spec.Ports)
	})

	t.Run("ingress", func(t *testing.T) {
		err = r.checkMattermostIngress(ci, ci.Name, ci.Spec.IngressName, ci.Spec.IngressAnnotations, logger)
		assert.NoError(t, err)

		found := &v1beta1.Ingress{}
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Labels = nil
		modified.Annotations = nil
		modified.Spec = v1beta1.IngressSpec{}

		err = r.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		err = r.checkMattermostIngress(ci, ci.Name, ci.Spec.IngressName, ci.Spec.IngressAnnotations, logger)
		require.NoError(t, err)
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetAnnotations(), found.GetAnnotations())
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Spec.Rules, original.Spec.Rules)
	})

	t.Run("deployment", func(t *testing.T) {
		updateName := "mattermost-update-check"
		now := metav1.Now()
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      updateName,
				Namespace: ci.GetNamespace(),
			},
			Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: mattermostv1alpha1.MattermostAppContainerName, Image: ci.GetImageName()},
					},
				},
			}},
			Status: batchv1.JobStatus{
				Succeeded:      1,
				CompletionTime: &now,
			},
		}
		err := r.Client.Create(context.TODO(), job)
		require.NoError(t, err)

		err = r.checkMattermostDeployment(ci, ci.Name, ci.Spec.IngressName, ci.Name, ci.GetImageName(), logger)
		assert.NoError(t, err)

		found := &appsv1.Deployment{}
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Labels = nil
		newReplicas := int32(0)
		modified.Spec.Replicas = &newReplicas
		modified.Spec.Template.Spec.Containers[0].Image = "not-mattermost:latest"

		err = r.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		err = r.checkMattermostDeployment(ci, ci.Name, ci.Spec.IngressName, ci.Name, ci.GetImageName(), logger)
		require.NoError(t, err)
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.Labels, found.Labels)
		assert.Equal(t, original.Spec.Replicas, found.Spec.Replicas)
		assert.Equal(t, original.Spec.Template, found.Spec.Template)
	})

	t.Run("final check", func(t *testing.T) {
		t.Run("default database secret should be missing", func(t *testing.T) {
			dbSecret := &corev1.Secret{}
			err := r.Client.Get(context.TODO(), types.NamespacedName{Name: mattermostmysql.DefaultDatabaseSecretName(ciName), Namespace: ciNamespace}, dbSecret)
			require.Error(t, err)
		})
	})
}
