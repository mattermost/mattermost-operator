package mattermost

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/mattermost/mattermost-operator/pkg/resources"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"

	rbacv1 "k8s.io/api/rbac/v1"

	blubr "github.com/mattermost/blubr"
	mattermostmysql "github.com/mattermost/mattermost-operator/pkg/components/mysql"
	operatortest "github.com/mattermost/mattermost-operator/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	logger, _, reconciler := setupTestDeps(t)

	recStatus := reconcileStatus{
		Status: false,
		Error:  nil,
	}

	mmName := "foo"
	mmNamespace := "default"
	replicas := int32(4)
	mm := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmName,
			Namespace: mmNamespace,
			UID:       types.UID("test"),
		},
		Spec: mmv1beta.MattermostSpec{
			Replicas:    &replicas,
			Image:       "mattermost/mattermost-enterprise-edition",
			Version:     operatortest.LatestStableMattermostVersion,
			IngressName: "foo.mattermost.dev",
		},
	}

	currentMMStatus := &mmv1beta.MattermostStatus{}

	dbInfo, fileStoreInfo := fixedDBAndFileStoreInfo(t, mm)

	var err error

	t.Run("service", func(t *testing.T) {
		recStatus = reconciler.checkMattermostService(mm, currentMMStatus, logger)
		assert.NoError(t, recStatus.Error)

		found := &corev1.Service{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Labels = nil
		modified.Annotations = nil
		modified.Spec = corev1.ServiceSpec{}

		err = reconciler.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		recStatus = reconciler.checkMattermostService(mm, currentMMStatus, logger)
		require.NoError(t, recStatus.Error)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Spec.Selector, found.Spec.Selector)
		assert.Equal(t, original.Spec.Ports, found.Spec.Ports)
	})

	t.Run("service account", func(t *testing.T) {
		recStatus = reconciler.checkMattermostSA(mm, logger)
		assert.NoError(t, recStatus.Error)

		found := &corev1.ServiceAccount{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		err = reconciler.Client.Delete(context.TODO(), found)
		require.NoError(t, err)
		recStatus = reconciler.checkMattermostSA(mm, logger)
		require.NoError(t, recStatus.Error)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
	})

	t.Run("role", func(t *testing.T) {
		recStatus = reconciler.checkMattermostRole(mm, logger)
		assert.NoError(t, recStatus.Error)

		found := &rbacv1.Role{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Rules = nil

		err = reconciler.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		recStatus = reconciler.checkMattermostRole(mm, logger)
		require.NoError(t, recStatus.Error)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Rules, found.Rules)
	})

	t.Run("role binding", func(t *testing.T) {
		recStatus = reconciler.checkMattermostRoleBinding(mm, logger)
		assert.NoError(t, recStatus.Error)

		found := &rbacv1.RoleBinding{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Subjects = nil

		err = reconciler.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		recStatus = reconciler.checkMattermostRoleBinding(mm, logger)
		require.NoError(t, recStatus.Error)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Subjects, found.Subjects)
	})

	t.Run("ingress no tls", func(t *testing.T) {
		mm.Spec.UseIngressTLS = false
		recStatus = reconciler.checkMattermostIngress(mm, logger)
		assert.NoError(t, recStatus.Error)

		found := &v1beta1.Ingress{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)
		require.Nil(t, found.Spec.TLS)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Labels = nil
		modified.Annotations = nil
		modified.Spec = v1beta1.IngressSpec{}

		err = reconciler.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		recStatus = reconciler.checkMattermostIngress(mm, logger)
		require.NoError(t, recStatus.Error)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetAnnotations(), found.GetAnnotations())
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Spec.Rules, found.Spec.Rules)
	})

	t.Run("ingress with tls", func(t *testing.T) {
		mm.Spec.UseIngressTLS = true
		mm.Spec.IngressAnnotations = map[string]string{
			"kubernetes.io/ingress.class": "nginx-test",
			"test-ingress":                "blabla",
		}

		recStatus = reconciler.checkMattermostIngress(mm, logger)
		assert.NoError(t, recStatus.Error)

		found := &v1beta1.Ingress{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotEmpty(t, found)
		require.NotNil(t, found.Spec.TLS)
		require.NotNil(t, found.Annotations)
		assert.Contains(t, found.Annotations, "kubernetes.io/ingress.class")

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Labels = nil
		modified.Annotations = nil
		modified.Spec = v1beta1.IngressSpec{}

		err = reconciler.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		recStatus = reconciler.checkMattermostIngress(mm, logger)
		require.NoError(t, recStatus.Error)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetAnnotations(), found.GetAnnotations())
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Spec.Rules, found.Spec.Rules)
		assert.Equal(t, original.Spec.TLS, original.Spec.TLS)
	})

	t.Run("ingress disabled", func(t *testing.T) {
		recStatus = reconciler.checkMattermostIngress(mm, logger)
		assert.NoError(t, recStatus.Error)

		found := &v1beta1.Ingress{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotEmpty(t, found)

		mm.Spec.Ingress = &mmv1beta.Ingress{Enabled: false}

		recStatus = reconciler.checkMattermostIngress(mm, logger)
		require.NoError(t, recStatus.Error)

		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.Error(t, err)
		assert.True(t, k8sErrors.IsNotFound(err))
	})

	t.Run("deployment", func(t *testing.T) {
		updateName := "mattermost-update-check"
		now := metav1.Now()
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      updateName,
				Namespace: mm.GetNamespace(),
			},
			Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: mmv1beta.MattermostAppContainerName, Image: mm.GetImageName()},
					},
				},
			}},
			Status: batchv1.JobStatus{
				Succeeded:      1,
				CompletionTime: &now,
			},
		}
		err = reconciler.Client.Create(context.TODO(), job)
		require.NoError(t, err)

		recStatus = reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		assert.NoError(t, recStatus.Error)

		//dbSetupJob := &batchv1.Job{}
		//err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mattermost.SetupJobName, Namespace: mmNamespace}, dbSetupJob)
		//require.NoError(t, err)
		//require.Equal(t, 1, len(dbSetupJob.Spec.Template.Spec.Containers))
		//require.Equal(t, mm.GetImageName(), dbSetupJob.Spec.Template.Spec.Containers[0].Image)
		//_, containerFound := findContainer(mattermost.WaitForDBSetupContainerName, dbSetupJob.Spec.Template.Spec.InitContainers)
		//require.False(t, containerFound)

		found := &appsv1.Deployment{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, mm.Name, found.Spec.Template.Spec.ServiceAccountName)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Labels = nil
		newReplicas := int32(0)
		modified.Spec.Replicas = &newReplicas
		modified.Spec.Template.Spec.Containers[0].Image = "not-mattermost:latest"

		err = reconciler.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		recStatus = reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		require.NoError(t, recStatus.Error)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetLabels(), found.GetLabels())
		assert.Equal(t, original.Spec.Replicas, found.Spec.Replicas)
		assert.Equal(t, original.Spec.Template, found.Spec.Template)
	})

	t.Run("restart update job", func(t *testing.T) {
		// create deployment
		recStatus = reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		assert.NoError(t, recStatus.Error)

		// create update job with invalid image
		updateName := "mattermost-update-check"
		now := metav1.Now()
		invalidUpdateJob := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      updateName,
				Namespace: mm.GetNamespace(),
			},
			Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: mmv1beta.MattermostAppContainerName, Image: "invalid-image"},
					},
				},
			}},
			Status: batchv1.JobStatus{
				Succeeded:      1,
				CompletionTime: &now,
			},
		}
		err := reconciler.Client.Create(context.TODO(), invalidUpdateJob)
		require.NoError(t, err)

		// should delete update job and create new and return error
		newImage := "mattermost/new-image"
		mm.Spec.Image = newImage

		recStatus = reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		assert.Error(t, recStatus.Error)
		assert.Contains(t, recStatus.Error, "Restarted update image job")

		// get new job, assert new image and change status to completed
		restartedUpdateJob := batchv1.Job{}
		updateJobKey := types.NamespacedName{Namespace: mm.GetNamespace(), Name: resources.UpdateJobName}
		err = reconciler.Client.Get(context.TODO(), updateJobKey, &restartedUpdateJob)
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s:%s", newImage, mm.Spec.Version), restartedUpdateJob.Spec.Template.Spec.Containers[0].Image)
		assert.Equal(t, int32(0), restartedUpdateJob.Status.Succeeded)

		restartedUpdateJob.Status = batchv1.JobStatus{
			Succeeded:      1,
			CompletionTime: &now,
		}

		err = reconciler.Client.Update(context.TODO(), &restartedUpdateJob)
		require.NoError(t, err)

		// should succeed now
		recStatus = reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		assert.NoError(t, recStatus.Error)
	})
}

func TestCheckMattermostExternalDBAndFileStore(t *testing.T) {
	logger, _, reconciler := setupTestDeps(t)

	recStatus := reconcileStatus{
		Status: false,
		Error:  nil,
	}

	mmName := "foo"
	mmNamespace := "default"
	replicas := int32(4)
	externalDBSecretName := "externalDB"
	mm := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmName,
			Namespace: mmNamespace,
			UID:       types.UID("test"),
		},
		Spec: mmv1beta.MattermostSpec{
			Replicas:    &replicas,
			Image:       "mattermost/mattermost-enterprise-edition",
			Version:     operatortest.LatestStableMattermostVersion,
			IngressName: "foo.mattermost.dev",
			Database: mmv1beta.Database{
				External: &mmv1beta.ExternalDatabase{
					Secret: externalDBSecretName,
				},
			},
			FileStore: mmv1beta.FileStore{
				External: &mmv1beta.ExternalFileStore{
					URL:    "s3.amazon.com",
					Bucket: "my-bucket",
					Secret: "fileStoreSecret",
				},
			},
		},
	}
	currentMMStatus := &mmv1beta.MattermostStatus{}

	dbInfo, err := mattermostApp.NewExternalDBConfig(mm, corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "dbSecret"},
		Data: map[string][]byte{
			"DB_CONNECTION_STRING": []byte("postgres://my-postgres:5432"),
		},
	})
	require.NoError(t, err)

	fileStoreInfo, err := mattermostApp.NewExternalFileStoreInfo(mm, corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "fileStoreSecret"},
		Data: map[string][]byte{
			"accesskey": []byte("my-key"),
			"secretkey": []byte("my-secret"),
		},
	})
	require.NoError(t, err)

	externalDBSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalDBSecretName,
			Namespace: mmNamespace,
		},
		Data: map[string][]byte{
			"DB_CONNECTION_STRING": []byte("mysql://test"),
		},
	}
	err = reconciler.Client.Create(context.TODO(), externalDBSecret)
	require.NoError(t, err)

	t.Run("service", func(t *testing.T) {
		recStatus = reconciler.checkMattermostService(mm, currentMMStatus, logger)
		assert.NoError(t, recStatus.Error)

		found := &corev1.Service{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Labels = nil
		modified.Annotations = nil
		modified.Spec = corev1.ServiceSpec{}

		err = reconciler.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		recStatus = reconciler.checkMattermostService(mm, currentMMStatus, logger)
		require.NoError(t, recStatus.Error)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Spec.Selector, found.Spec.Selector)
		assert.Equal(t, original.Spec.Ports, found.Spec.Ports)
	})

	t.Run("ingress", func(t *testing.T) {
		recStatus = reconciler.checkMattermostIngress(mm, logger)
		assert.NoError(t, recStatus.Error)

		found := &v1beta1.Ingress{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Labels = nil
		modified.Annotations = nil
		modified.Spec = v1beta1.IngressSpec{}

		err = reconciler.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		recStatus = reconciler.checkMattermostIngress(mm, logger)
		require.NoError(t, recStatus.Error)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
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
				Namespace: mm.GetNamespace(),
			},
			Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: mmv1beta.MattermostAppContainerName, Image: mm.GetImageName()},
					},
				},
			}},
			Status: batchv1.JobStatus{
				Succeeded:      1,
				CompletionTime: &now,
			},
		}
		err := reconciler.Client.Create(context.TODO(), job)
		require.NoError(t, err)

		recStatus = reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		assert.NoError(t, recStatus.Error)

		// TODO: uncomment when enabling back the db setup job
		//dbSetupJob := &batchv1.Job{}
		//err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mattermost.SetupJobName, Namespace: ciNamespace}, dbSetupJob)
		//require.NoError(t, err)
		//require.Equal(t, 1, len(dbSetupJob.Spec.Template.Spec.Containers))
		//require.Equal(t, ci.GetImageName(), dbSetupJob.Spec.Template.Spec.Containers[0].Image)
		//_, containerFound := findContainer(mattermost.WaitForDBSetupContainerName, dbSetupJob.Spec.Template.Spec.InitContainers)
		//require.False(t, containerFound)

		found := &appsv1.Deployment{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Labels = nil
		newReplicas := int32(0)
		modified.Spec.Replicas = &newReplicas
		modified.Spec.Template.Spec.Containers[0].Image = "not-mattermost:latest"

		err = reconciler.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		recStatus = reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		require.NoError(t, recStatus.Error)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.Labels, found.Labels)
		assert.Equal(t, original.Spec.Replicas, found.Spec.Replicas)
		assert.Equal(t, original.Spec.Template, found.Spec.Template)
	})

	t.Run("final check", func(t *testing.T) {
		t.Run("default database secret should be missing", func(t *testing.T) {
			dbSecret := &corev1.Secret{}
			err := reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mattermostmysql.DefaultDatabaseSecretName(mmName), Namespace: mmNamespace}, dbSecret)
			require.Error(t, err)
		})
	})
}

func TestSpecialCases(t *testing.T) {
	logger, _, reconciler := setupTestDeps(t)

	mmName := "foo"
	mmNamespace := "default"
	mm := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmName,
			Namespace: mmNamespace,
			UID:       types.UID("test"),
		},
		Spec: mmv1beta.MattermostSpec{
			UseServiceLoadBalancer: true,
		},
	}
	currentMMStatus := &mmv1beta.MattermostStatus{}

	t.Run("service - copy ClusterIP for LoadBalancer service", func(t *testing.T) {
		// Create the service
		err := reconciler.checkMattermostService(mm, currentMMStatus, logger)
		require.NoError(t, err.Error)

		service := &corev1.Service{}
		err.Error = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, service)
		require.NoError(t, err.Error)

		service.Spec.ClusterIPs = []string{"10.10.10.10", "10.10.10.11"}
		service.Spec.ClusterIP = "10.10.10.10"
		err.Error = reconciler.Client.Update(context.TODO(), service)
		require.NoError(t, err.Error)

		mm.Spec.ResourceLabels = map[string]string{"myLabel": "test"}
		err = reconciler.checkMattermostService(mm, currentMMStatus, logger)
		require.NoError(t, err.Error)

		modified := &corev1.Service{}
		err.Error = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, modified)
		require.NoError(t, err.Error)
		assert.Equal(t, service.Spec.ClusterIPs, modified.Spec.ClusterIPs)
		assert.Equal(t, service.Spec.ClusterIP, modified.Spec.ClusterIP)
	})
}

const (
	exposePortPatch = `[
	{
		"op":"add",
		"path":"/spec/template/spec/containers/0/ports/-",
		"value":{"containerPort":8443, "name":"calls", "protocol":"UDP"}
	}
]`
	invalidPatch = `[{"op": "add", "path": "/metadata/something/whatever"}]`

	addServicePortPatch = `[
    {
		"op":"add",
		"path":"/spec/ports/-",
		"value": {"name": "calls", "port": 8443, "protocol": "UDP"}
	}
]`
)

func Test_Patches(t *testing.T) {
	mmName := "foo"
	mmNamespace := "default"
	baseMM := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmName,
			Namespace: mmNamespace,
			UID:       types.UID("test"),
		},
		Spec: mmv1beta.MattermostSpec{
			UseServiceLoadBalancer: true,
		},
	}

	t.Run("deployment patch", func(t *testing.T) {
		for _, testCase := range []struct {
			description string
			patch       *mmv1beta.ResourcePatch
			assertFn    func(t *testing.T, deploy *appsv1.Deployment, mmStatus mmv1beta.MattermostStatus)
		}{
			{
				description: "apply patch",
				patch: &mmv1beta.ResourcePatch{
					Deployment: &mmv1beta.Patch{
						Patch: exposePortPatch,
					},
				},
				assertFn: func(t *testing.T, deploy *appsv1.Deployment, mmStatus mmv1beta.MattermostStatus) {
					mmContainer := mmv1beta.GetMattermostAppContainer(deploy.Spec.Template.Spec.Containers)
					assert.Equal(t, 3, len(mmContainer.Ports))
					assert.Equal(t, "calls", mmContainer.Ports[2].Name)
					assert.Equal(t, int32(8443), mmContainer.Ports[2].ContainerPort)
					assert.Equal(t, corev1.ProtocolUDP, mmContainer.Ports[2].Protocol)

					assert.Empty(t, mmStatus.ResourcePatch.DeploymentPatch.Error)
					assert.True(t, mmStatus.ResourcePatch.DeploymentPatch.Applied)
				},
			},
			{
				description: "do not apply if disabled",
				patch: &mmv1beta.ResourcePatch{
					Deployment: &mmv1beta.Patch{
						Disable: true,
						Patch:   exposePortPatch,
					},
				},
				assertFn: func(t *testing.T, deploy *appsv1.Deployment, mmStatus mmv1beta.MattermostStatus) {
					mmContainer := mmv1beta.GetMattermostAppContainer(deploy.Spec.Template.Spec.Containers)
					assert.Equal(t, 2, len(mmContainer.Ports))

					assert.Nil(t, mmStatus.ResourcePatch)
				},
			},
			{
				description: "continue without patch on error",
				patch: &mmv1beta.ResourcePatch{
					Deployment: &mmv1beta.Patch{
						Patch: invalidPatch,
					},
				},
				assertFn: func(t *testing.T, deploy *appsv1.Deployment, mmStatus mmv1beta.MattermostStatus) {
					mmContainer := mmv1beta.GetMattermostAppContainer(deploy.Spec.Template.Spec.Containers)
					assert.Equal(t, 2, len(mmContainer.Ports))

					assert.NotEmpty(t, mmStatus.ResourcePatch.DeploymentPatch.Error)
					assert.False(t, mmStatus.ResourcePatch.DeploymentPatch.Applied)
				},
			},
		} {
			t.Run(testCase.description, func(t *testing.T) {
				logger, fakeClient, reconciler := setupTestDeps(t)
				mm := baseMM.DeepCopy()
				mmStatus := mmv1beta.MattermostStatus{}

				mm.Spec.ResourcePatch = testCase.patch

				dbInfo, fileStoreInfo := fixedDBAndFileStoreInfo(t, mm)

				err := reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, &mmStatus, logger)
				require.NoError(t, err.Error)

				deployKey := types.NamespacedName{Name: mmName, Namespace: mmNamespace}
				deploy := appsv1.Deployment{}
				err.Error = fakeClient.Get(context.Background(), deployKey, &deploy)
				require.NoError(t, err.Error)

				testCase.assertFn(t, &deploy, mmStatus)
			})
		}
	})

	t.Run("service patch", func(t *testing.T) {
		for _, testCase := range []struct {
			description string
			patch       *mmv1beta.ResourcePatch
			assertFn    func(t *testing.T, deploy *corev1.Service, mmStatus mmv1beta.MattermostStatus)
		}{
			{
				description: "apply patch",
				patch: &mmv1beta.ResourcePatch{
					Service: &mmv1beta.Patch{
						Patch: addServicePortPatch,
					},
				},
				assertFn: func(t *testing.T, svc *corev1.Service, mmStatus mmv1beta.MattermostStatus) {
					assert.Equal(t, 3, len(svc.Spec.Ports))
					assert.Equal(t, "calls", svc.Spec.Ports[2].Name)
					assert.Equal(t, int32(8443), svc.Spec.Ports[2].Port)
					assert.Equal(t, corev1.ProtocolUDP, svc.Spec.Ports[2].Protocol)

					assert.Empty(t, mmStatus.ResourcePatch.ServicePatch.Error)
					assert.True(t, mmStatus.ResourcePatch.ServicePatch.Applied)
				},
			},
			{
				description: "do not apply if disabled",
				patch: &mmv1beta.ResourcePatch{
					Service: &mmv1beta.Patch{
						Disable: true,
						Patch:   exposePortPatch,
					},
				},
				assertFn: func(t *testing.T, svc *corev1.Service, mmStatus mmv1beta.MattermostStatus) {
					assert.Equal(t, 2, len(svc.Spec.Ports))

					assert.Nil(t, mmStatus.ResourcePatch)
				},
			},
			{
				description: "continue without patch on error",
				patch: &mmv1beta.ResourcePatch{
					Service: &mmv1beta.Patch{
						Patch: invalidPatch,
					},
				},
				assertFn: func(t *testing.T, svc *corev1.Service, mmStatus mmv1beta.MattermostStatus) {
					assert.Equal(t, 2, len(svc.Spec.Ports))

					assert.NotEmpty(t, mmStatus.ResourcePatch.ServicePatch.Error)
					assert.False(t, mmStatus.ResourcePatch.ServicePatch.Applied)
				},
			},
		} {
			t.Run(testCase.description, func(t *testing.T) {
				logger, fakeClient, reconciler := setupTestDeps(t)
				mm := baseMM.DeepCopy()
				mmStatus := mmv1beta.MattermostStatus{}

				mm.Spec.ResourcePatch = testCase.patch

				err := reconciler.checkMattermostService(mm, &mmStatus, logger)
				require.NoError(t, err.Error)

				deployKey := types.NamespacedName{Name: mmName, Namespace: mmNamespace}
				svc := corev1.Service{}
				err.Error = fakeClient.Get(context.Background(), deployKey, &svc)
				require.NoError(t, err.Error)

				testCase.assertFn(t, &svc, mmStatus)
			})
		}
	})
}

func setupTestDeps(t *testing.T) (logr.Logger, client.Client, *MattermostReconciler) {
	// Setup logging for the reconciler, so we can see what happened on failure.
	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logSink = logSink.WithName("test.opr")
	logger := logr.New(logSink)
	logf.SetLogger(logger)

	s := prepareSchema(t, scheme.Scheme)
	s.AddKnownTypes(mmv1beta.GroupVersion, &mmv1beta.Mattermost{})
	c := fake.NewFakeClient()
	r := &MattermostReconciler{
		Client:         c,
		Scheme:         s,
		Log:            logger,
		MaxReconciling: 5,
		Resources:      resources.NewResourceHelper(c, s),
	}
	return logger, c, r
}

func fixedDBAndFileStoreInfo(t *testing.T, mm *mmv1beta.Mattermost) (mattermostApp.DatabaseConfig, *mattermostApp.FileStoreInfo) {
	dbInfo, err := mattermostApp.NewMySQLDBConfig(corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "dbSecret"},
		Data: map[string][]byte{
			"ROOT_PASSWORD": []byte("root-pass"),
			"USER":          []byte("user"),
			"PASSWORD":      []byte("pass"),
			"DATABASE":      []byte("db"),
		},
	})
	require.NoError(t, err)

	fileStoreInfo := mattermostApp.NewOperatorManagedFileStoreInfo(mm, "fileStoreSecret", "http://minio:9000")
	require.NoError(t, err)

	return dbInfo, fileStoreInfo
}
