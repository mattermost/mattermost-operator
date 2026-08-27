package mattermost

import (
	"context"
	"fmt"
	"testing"

	pkgUtils "github.com/mattermost/mattermost-operator/pkg/utils"

	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/mattermost/mattermost-operator/pkg/resources"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"

	rbacv1 "k8s.io/api/rbac/v1"

	blubr "github.com/mattermost/blubr"
	operatortest "github.com/mattermost/mattermost-operator/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestCheckMattermost(t *testing.T) {
	logger, _, reconciler := setupTestDeps(t)

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
		err = reconciler.checkMattermostService(mm, currentMMStatus, logger)
		assert.NoError(t, err)

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
		err = reconciler.checkMattermostService(mm, currentMMStatus, logger)
		require.NoError(t, err)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Spec.Selector, found.Spec.Selector)
		assert.Equal(t, original.Spec.Ports, found.Spec.Ports)
	})

	t.Run("recreate service on type change", func(t *testing.T) {
		err = reconciler.checkMattermostService(mm, currentMMStatus, logger)
		assert.NoError(t, err)

		found := &corev1.Service{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		original := found.DeepCopy()

		mm.Spec.UseServiceLoadBalancer = true
		// Subtests below share this Mattermost, and UseServiceLoadBalancer suppresses
		// Ingress and HTTPRoute reconciliation, so keep the flag scoped to this case.
		defer func() { mm.Spec.UseServiceLoadBalancer = false }()

		err = reconciler.checkMattermostService(mm, currentMMStatus, logger)
		require.NoError(t, err)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, corev1.ServiceTypeLoadBalancer, found.Spec.Type)
	})

	t.Run("service account", func(t *testing.T) {
		err = reconciler.checkMattermostSA(mm, logger)
		assert.NoError(t, err)

		found := &corev1.ServiceAccount{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		err = reconciler.Client.Delete(context.TODO(), found)
		require.NoError(t, err)
		err = reconciler.checkMattermostSA(mm, logger)
		require.NoError(t, err)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		err = reconciler.Client.Delete(context.TODO(), found)
		require.NoError(t, err)

		mm.Spec.FileStore.External = &mmv1beta.ExternalFileStore{
			UseServiceAccount: true,
		}
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"eks.amazonaws.com/role-arn": "asd",
				},
				Name:      mmName,
				Namespace: mmNamespace,
			},
		}
		err = reconciler.Client.Create(context.TODO(), sa)
		require.NoError(t, err)
		err = reconciler.checkMattermostSA(mm, logger)
		assert.NoError(t, err)
		found = &corev1.ServiceAccount{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.Equal(t, "asd", found.Annotations["eks.amazonaws.com/role-arn"])
		err = reconciler.Client.Delete(context.TODO(), sa)
		require.NoError(t, err)

		mm.Spec.FileStore.External = nil

		err = reconciler.checkMattermostSA(mm, logger)
		assert.NoError(t, err)
		found = &corev1.ServiceAccount{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		err = reconciler.Client.Delete(context.TODO(), sa)
		require.NoError(t, err)

		mm.Spec.FileStore.External = &mmv1beta.ExternalFileStore{
			UseServiceAccount: false,
		}

		err = reconciler.checkMattermostSA(mm, logger)
		assert.NoError(t, err)
		found = &corev1.ServiceAccount{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
	})

	t.Run("role", func(t *testing.T) {
		err = reconciler.checkMattermostRole(mm, logger)
		assert.NoError(t, err)

		found := &rbacv1.Role{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Rules = nil

		err = reconciler.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		err = reconciler.checkMattermostRole(mm, logger)
		require.NoError(t, err)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Rules, found.Rules)
	})

	t.Run("role binding", func(t *testing.T) {
		err = reconciler.checkMattermostRoleBinding(mm, logger)
		assert.NoError(t, err)

		found := &rbacv1.RoleBinding{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		original := found.DeepCopy()
		modified := found.DeepCopy()
		modified.Subjects = nil

		err = reconciler.Client.Update(context.TODO(), modified)
		require.NoError(t, err)
		err = reconciler.checkMattermostRoleBinding(mm, logger)
		require.NoError(t, err)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Subjects, found.Subjects)
	})

	t.Run("ingress no tls", func(t *testing.T) {
		mm.Spec.UseIngressTLS = false
		err = reconciler.checkMattermostIngress(mm, logger)
		assert.NoError(t, err)

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
		err = reconciler.checkMattermostIngress(mm, logger)
		require.NoError(t, err)
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

		err = reconciler.checkMattermostIngress(mm, logger)
		assert.NoError(t, err)

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
		err = reconciler.checkMattermostIngress(mm, logger)
		require.NoError(t, err)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetAnnotations(), found.GetAnnotations())
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Spec.Rules, found.Spec.Rules)
		assert.Equal(t, original.Spec.TLS, original.Spec.TLS)
	})

	t.Run("ingress disabled", func(t *testing.T) {
		err = reconciler.checkMattermostIngress(mm, logger)
		assert.NoError(t, err)

		found := &v1beta1.Ingress{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotEmpty(t, found)

		mm.Spec.Ingress = &mmv1beta.Ingress{Enabled: false}

		err = reconciler.checkMattermostIngress(mm, logger)
		require.NoError(t, err)

		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.Error(t, err)
		assert.True(t, k8sErrors.IsNotFound(err))
	})

	t.Run("deployment", func(t *testing.T) {
		updateJobName := "mattermost-update-check"

		recStatus, err := reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		assert.NoError(t, err)

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

		// Update job should be launched, we do not expect error
		recStatus, err = reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		require.NoError(t, err)
		assert.False(t, recStatus.ResourcesReady)

		// Assert the job was created
		job := &batchv1.Job{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: updateJobName, Namespace: mmNamespace}, job)
		require.NoError(t, err)

		// Assert owner references are set here
		assert.NotEmpty(t, job.ObjectMeta.OwnerReferences)
		assert.Equal(t, "Mattermost", job.ObjectMeta.OwnerReferences[0].Kind)
		assert.Equal(t, mmName, job.ObjectMeta.OwnerReferences[0].Name)
		assert.Equal(t, "installation.mattermost.com/v1beta1", job.ObjectMeta.OwnerReferences[0].APIVersion)

		recStatus, err = reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		assert.NoError(t, err)
		assert.False(t, recStatus.ResourcesReady)

		// Set job status to succeeded so that test can proceed
		now := metav1.Now()
		job.Status = batchv1.JobStatus{
			Succeeded:      1,
			CompletionTime: &now,
		}
		err = reconciler.Client.Status().Update(context.TODO(), job)
		require.NoError(t, err)

		// Job is marked as succeeded, should proceed now.
		recStatus, err = reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		require.NoError(t, err)
		assert.True(t, recStatus.ResourcesReady)

		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.Spec.Replicas, found.Spec.Replicas)
		assert.Equal(t, original.Spec.Template, found.Spec.Template)
	})

	t.Run("restart update job", func(t *testing.T) {
		// create deployment
		recStatus, err := reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		assert.NoError(t, err)
		assert.True(t, recStatus.ResourcesReady)

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
		err = reconciler.Client.Create(context.TODO(), invalidUpdateJob)
		require.NoError(t, err)

		// should delete update job and create new and return error
		newImage := "mattermost/new-image"
		mm.Spec.Image = newImage

		// check deployment - update job is not completed, therefore resource is not ready
		recStatus, err = reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		assert.NoError(t, err)
		assert.False(t, recStatus.ResourcesReady)

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
		recStatus, err = reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		assert.NoError(t, err)
	})
}

func TestCheckMattermostAWSLoadBalancer(t *testing.T) {
	logger, _, reconciler := setupTestDeps(t)

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
			Replicas: &replicas,
			Image:    "mattermost/mattermost-enterprise-edition",
			Version:  operatortest.LatestStableMattermostVersion,
		},
	}

	currentMMStatus := &mmv1beta.MattermostStatus{}

	var err error

	t.Run("service", func(t *testing.T) {
		mm.Spec.AWSLoadBalancerController = &mmv1beta.AWSLoadBalancerController{
			Enabled: true,
			Hosts: []mmv1beta.IngressHost{
				{
					HostName: "test.example.com",
				},
			},
		}

		err = reconciler.checkMattermostService(mm, currentMMStatus, logger)
		assert.NoError(t, err)

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
		err = reconciler.checkMattermostService(mm, currentMMStatus, logger)
		require.NoError(t, err)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Spec.Selector, found.Spec.Selector)
		assert.Equal(t, original.Spec.Ports, found.Spec.Ports)
		assert.Equal(t, corev1.ServiceTypeNodePort, found.Spec.Type)
	})

	t.Run("ingress with tls", func(t *testing.T) {
		mm.Spec.AWSLoadBalancerController = &mmv1beta.AWSLoadBalancerController{
			Enabled: true,
			Hosts: []mmv1beta.IngressHost{
				{
					HostName: "test.example.com",
				},
			},
		}
		mm.Spec.AWSLoadBalancerController.CertificateARN = "test-arn"

		err = reconciler.checkMattermostIngress(mm, logger)
		assert.NoError(t, err)

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
		err = reconciler.checkMattermostIngress(mm, logger)
		require.NoError(t, err)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetAnnotations(), found.GetAnnotations())
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Spec.Rules, found.Spec.Rules)
		assert.Contains(t, found.Annotations, "alb.ingress.kubernetes.io/scheme")
		assert.Contains(t, found.Annotations, "alb.ingress.kubernetes.io/certificate-arn")
		assert.Contains(t, found.Annotations, "alb.ingress.kubernetes.io/ssl-redirect")
		assert.Contains(t, found.Annotations, "alb.ingress.kubernetes.io/listen-ports")
	})

	t.Run("ingress with specific ingress class", func(t *testing.T) {
		mm.Spec.AWSLoadBalancerController = &mmv1beta.AWSLoadBalancerController{
			Enabled: true,
			Hosts: []mmv1beta.IngressHost{
				{
					HostName: "test.example.com",
				},
			},
		}
		mm.Spec.AWSLoadBalancerController.IngressClassName = "testClass"

		err = reconciler.checkMattermostIngress(mm, logger)
		assert.NoError(t, err)

		err = reconciler.checkMattermostIngressClass(mm, logger)
		assert.NoError(t, err)

		found := &v1beta1.Ingress{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		assert.Equal(t, found.Spec.IngressClassName, pkgUtils.NewString("testClass"))

		foundIngressClass := &v1beta1.IngressClass{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, foundIngressClass)
		require.Error(t, err)
	})

	t.Run("ingress with custom annotations", func(t *testing.T) {
		mm.Spec.AWSLoadBalancerController = &mmv1beta.AWSLoadBalancerController{
			Enabled: true,
			Hosts: []mmv1beta.IngressHost{
				{
					HostName: "test.example.com",
				},
			},
		}
		mm.Spec.AWSLoadBalancerController.Annotations = map[string]string{
			"test": "test",
		}

		err = reconciler.checkMattermostIngress(mm, logger)
		assert.NoError(t, err)

		found := &v1beta1.Ingress{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		assert.Contains(t, found.Annotations, "alb.ingress.kubernetes.io/scheme")
		assert.Contains(t, found.Annotations, "alb.ingress.kubernetes.io/listen-ports")
		assert.Contains(t, found.Annotations, "test")
	})

	t.Run("disable and enable aws load balancer", func(t *testing.T) {
		mm.Spec.AWSLoadBalancerController.Enabled = false
		err = reconciler.checkMattermostIngress(mm, logger)
		assert.NoError(t, err)

		err = reconciler.checkMattermostIngressClass(mm, logger)
		assert.NoError(t, err)

		found := &v1beta1.Ingress{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)

		assert.Contains(t, found.Annotations, "kubernetes.io/ingress.class")

		foundIngressClass := &v1beta1.IngressClass{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, foundIngressClass)
		require.Error(t, err)

		mm.Spec.AWSLoadBalancerController = &mmv1beta.AWSLoadBalancerController{
			Enabled: true,
			Hosts: []mmv1beta.IngressHost{
				{
					HostName: "test.example.com",
				},
			},
		}

		err = reconciler.checkMattermostIngress(mm, logger)
		assert.NoError(t, err)

		err = reconciler.checkMattermostIngressClass(mm, logger)
		assert.NoError(t, err)

		modified := &v1beta1.Ingress{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, modified)
		require.NoError(t, err)
		require.NotNil(t, modified)

		assert.Contains(t, modified.Annotations, "alb.ingress.kubernetes.io/scheme")
		assert.Contains(t, modified.Annotations, "alb.ingress.kubernetes.io/listen-ports")
		modifiedIngressClass := &v1beta1.IngressClass{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, modifiedIngressClass)
		require.NoError(t, err)
	})
}

func TestCheckMattermostUpdateJob(t *testing.T) {
	logger, _, reconciler := setupTestDeps(t)

	mmName := "foo"
	mmNamespace := "default"
	replicas := int32(4)
	extraLabels := map[string]string{
		"app":   "test",
		"owner": "test",
		"foo":   "bar",
	}
	extraAnnotations := map[string]string{
		"owner": "test",
		"foo":   "bar",
	}
	updateJobSpec := mmv1beta.UpdateJob{
		ExtraLabels:      extraLabels,
		ExtraAnnotations: extraAnnotations,
	}

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
			UpdateJob:   &updateJobSpec,
		},
	}

	currentMMStatus := &mmv1beta.MattermostStatus{}

	dbInfo, fileStoreInfo := fixedDBAndFileStoreInfo(t, mm)

	t.Run("update job extra labels and annotations", func(t *testing.T) {
		updateJobName := "mattermost-update-check"

		recStatus, err := reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		assert.NoError(t, err)
		assert.True(t, recStatus.ResourcesReady)

		found := &appsv1.Deployment{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, mm.Name, found.Spec.Template.Spec.ServiceAccountName)

		modified := found.DeepCopy()
		modified.Spec.Template.Spec.Containers[0].Image = "not-mattermost:latest"

		err = reconciler.Client.Update(context.TODO(), modified)
		require.NoError(t, err)

		// Update job should be launched, we do not expect error
		recStatus, err = reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		require.NoError(t, err)
		assert.False(t, recStatus.ResourcesReady)

		// Assert the job was created
		job := &batchv1.Job{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: updateJobName, Namespace: mmNamespace}, job)
		require.NoError(t, err)

		// Assert labels are set here
		for k, v := range extraLabels {
			assert.Equal(t, job.Spec.Template.ObjectMeta.Labels[k], v)
		}
		// Validate we aren't overriding the app label
		assert.Equal(t, job.Spec.Template.ObjectMeta.Labels["app"], updateJobName)
		// Assert annotations are set here
		for k, v := range extraAnnotations {
			assert.Equal(t, job.Spec.Template.ObjectMeta.Annotations[k], v)
		}
	})
}

func TestCheckMattermostUpdateJobDisabled(t *testing.T) {
	logger, _, reconciler := setupTestDeps(t)

	mmName := "foo"
	mmNamespace := "default"
	replicas := int32(4)
	updateJobSpec := mmv1beta.UpdateJob{
		Disabled: true,
	}

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
			UpdateJob:   &updateJobSpec,
		},
	}

	currentMMStatus := &mmv1beta.MattermostStatus{}

	dbInfo, fileStoreInfo := fixedDBAndFileStoreInfo(t, mm)

	t.Run("no update job", func(t *testing.T) {
		updateJobName := "mattermost-update-check"

		recStatus, err := reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		assert.NoError(t, err)
		assert.True(t, recStatus.ResourcesReady)

		found := &appsv1.Deployment{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, mm.Name, found.Spec.Template.Spec.ServiceAccountName)

		modified := found.DeepCopy()
		modified.Spec.Template.Spec.Containers[0].Image = "not-mattermost:latest"

		err = reconciler.Client.Update(context.TODO(), modified)
		require.NoError(t, err)

		// Update job should NOT be launched, we do not expect error
		recStatus, err = reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		require.NoError(t, err)
		assert.True(t, recStatus.ResourcesReady)

		// Assert the job was NOT created
		job := &batchv1.Job{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: updateJobName, Namespace: mmNamespace}, job)
		require.Error(t, err)
		require.True(t, k8sErrors.IsNotFound(err), "expected not found error when getting job")
		require.Equal(t, job, &batchv1.Job{})
	})
}

func TestCheckMattermostExternalDBAndFileStore(t *testing.T) {
	logger, _, reconciler := setupTestDeps(t)

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

	// An external database is the fixed choice for these tests; operator-managed
	// An external database is required; configure database.external.
	if mm.Spec.Database.External == nil {
		mm.Spec.Database.External = &mmv1beta.ExternalDatabase{Secret: "dbSecret"}
	}

	dbInfo, err := mattermostApp.NewExternalDBConfig(mm, corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "dbSecret"},
		Data: map[string][]byte{
			"DB_CONNECTION_STRING": []byte("postgres://my-postgres:5432"),
		},
	})
	require.NoError(t, err)

	fileStoreInfo, err := mattermostApp.NewExternalFileStoreInfo(mm, &corev1.Secret{
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
		err = reconciler.checkMattermostService(mm, currentMMStatus, logger)
		assert.NoError(t, err)

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
		err = reconciler.checkMattermostService(mm, currentMMStatus, logger)
		require.NoError(t, err)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Spec.Selector, found.Spec.Selector)
		assert.Equal(t, original.Spec.Ports, found.Spec.Ports)
	})

	t.Run("ingress", func(t *testing.T) {
		err = reconciler.checkMattermostIngress(mm, logger)
		assert.NoError(t, err)

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
		err = reconciler.checkMattermostIngress(mm, logger)
		require.NoError(t, err)
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

		recStatus, err := reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		assert.NoError(t, err)
		assert.Equal(t, true, recStatus.ResourcesReady)

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
		recStatus, err = reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		require.NoError(t, err)
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, found)
		require.NoError(t, err)
		assert.Equal(t, original.Labels, found.Labels)
		assert.Equal(t, original.Spec.Replicas, found.Spec.Replicas)
		assert.Equal(t, original.Spec.Template, found.Spec.Template)
	})

}

func TestCheckMattermostExternalVolumeFileStore(t *testing.T) {
	logger, _, reconciler := setupTestDeps(t)

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
			FileStore: mmv1beta.FileStore{
				ExternalVolume: &mmv1beta.ExternalVolumeFileStore{
					VolumeClaimName: "pvc1",
				},
			},
		},
	}
	currentMMStatus := &mmv1beta.MattermostStatus{}

	// An external database is the fixed choice for these tests; operator-managed
	// An external database is required; configure database.external.
	if mm.Spec.Database.External == nil {
		mm.Spec.Database.External = &mmv1beta.ExternalDatabase{Secret: "dbSecret"}
	}

	dbInfo, err := mattermostApp.NewExternalDBConfig(mm, corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "dbSecret"},
		Data: map[string][]byte{
			"DB_CONNECTION_STRING": []byte("mysql://user:pass@tcp(mysql:3306)/mattermost"),
		},
	})
	require.NoError(t, err)

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mm.Spec.FileStore.ExternalVolume.VolumeClaimName,
			Namespace: mm.GetNamespace(),
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
		},
	}
	err = reconciler.Client.Create(context.TODO(), pvc)
	require.NoError(t, err)

	fileStoreInfo, err := reconciler.checkExternalVolumeFileStore(mm, logger)
	require.NoError(t, err)

	t.Run("deployment", func(t *testing.T) {
		recStatus, recErr := reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		assert.NoError(t, recErr)
		assert.Equal(t, true, recStatus.ResourcesReady)

		foundDeploy := &appsv1.Deployment{}
		deployErr := reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, foundDeploy)
		require.NoError(t, deployErr)
		require.NotNil(t, foundDeploy)
		require.Len(t, foundDeploy.Spec.Template.Spec.Volumes, 1)
		require.Len(t, foundDeploy.Spec.Template.Spec.Containers, 1)

		foundDeploymentPV := foundDeploy.Spec.Template.Spec.Volumes[0]
		assert.Equal(t, mattermostApp.FileStoreDefaultVolumeName, foundDeploymentPV.Name)
		assert.Equal(t, mm.Spec.FileStore.ExternalVolume.VolumeClaimName, foundDeploymentPV.PersistentVolumeClaim.ClaimName)

		foundMMContainer := foundDeploy.Spec.Template.Spec.Containers[0]
		assert.Contains(t, foundMMContainer.Env, corev1.EnvVar{Name: "MM_FILESETTINGS_DRIVERNAME", Value: "local"})
	})
}

func TestCheckMattermostLocalFileStore(t *testing.T) {
	logger, _, reconciler := setupTestDeps(t)

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
			FileStore: mmv1beta.FileStore{
				Local: &mmv1beta.LocalFileStore{
					Enabled:     true,
					StorageSize: "1Gi",
				},
			},
		},
	}
	currentMMStatus := &mmv1beta.MattermostStatus{}

	// An external database is the fixed choice for these tests; operator-managed
	// An external database is required; configure database.external.
	if mm.Spec.Database.External == nil {
		mm.Spec.Database.External = &mmv1beta.ExternalDatabase{Secret: "dbSecret"}
	}

	dbInfo, err := mattermostApp.NewExternalDBConfig(mm, corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "dbSecret"},
		Data: map[string][]byte{
			"DB_CONNECTION_STRING": []byte("mysql://user:pass@tcp(mysql:3306)/mattermost"),
		},
	})
	require.NoError(t, err)

	fileStoreInfo, err := reconciler.checkLocalFileStore(mm, logger)
	require.NoError(t, err)

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

		recStatus, recErr := reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, currentMMStatus, logger)
		assert.NoError(t, recErr)
		assert.Equal(t, true, recStatus.ResourcesReady)

		foundDeploy := &appsv1.Deployment{}
		deployErr := reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, foundDeploy)
		require.NoError(t, deployErr)
		require.NotNil(t, foundDeploy)
	})

	t.Run("pvc", func(t *testing.T) {
		foundPvc := &corev1.PersistentVolumeClaim{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, foundPvc)
		require.NoError(t, err)
		require.NotNil(t, foundPvc)

		expectedStorage := resource.MustParse("1Gi")
		actualStorage := foundPvc.Spec.Resources.Requests.Storage()

		assert.Equal(t, expectedStorage, *actualStorage)
	})

	t.Run("update pvc", func(t *testing.T) {
		mm.Spec.FileStore.Local.StorageSize = "2Gi"

		_, err := reconciler.checkLocalFileStore(mm, logger)
		require.NoError(t, err)

		foundPvc := &corev1.PersistentVolumeClaim{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, foundPvc)
		require.NoError(t, err)
		require.NotNil(t, foundPvc)

		expectedStorage := resource.MustParse("2Gi")
		actualStorage := foundPvc.Spec.Resources.Requests.Storage()

		assert.Equal(t, expectedStorage, *actualStorage)
	})

	t.Run("default access modes for new PVC", func(t *testing.T) {
		// Create a new Mattermost instance with a different name to test fresh PVC creation
		newMMName := "foo-new"
		newMM := &mmv1beta.Mattermost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      newMMName,
				Namespace: mmNamespace,
				UID:       types.UID("test-new"),
			},
			Spec: mmv1beta.MattermostSpec{
				Replicas:    &replicas,
				Image:       "mattermost/mattermost-enterprise-edition",
				Version:     operatortest.LatestStableMattermostVersion,
				IngressName: "foo-new.mattermost.dev",
				FileStore: mmv1beta.FileStore{
					Local: &mmv1beta.LocalFileStore{
						Enabled:     true,
						StorageSize: "1Gi",
						// No AccessModes specified - should default to ReadWriteMany
					},
				},
			},
		}

		_, err := reconciler.checkLocalFileStore(newMM, logger)
		require.NoError(t, err)

		foundPvc := &corev1.PersistentVolumeClaim{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: newMMName, Namespace: mmNamespace}, foundPvc)
		require.NoError(t, err)
		require.NotNil(t, foundPvc)

		// Verify that new PVC gets ReadWriteMany by default
		assert.Equal(t, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}, foundPvc.Spec.AccessModes)
	})

	t.Run("explicit access modes", func(t *testing.T) {
		// Create a new Mattermost instance with explicit access modes
		explicitMMName := "foo-explicit"
		explicitMM := &mmv1beta.Mattermost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      explicitMMName,
				Namespace: mmNamespace,
				UID:       types.UID("test-explicit"),
			},
			Spec: mmv1beta.MattermostSpec{
				Replicas:    &replicas,
				Image:       "mattermost/mattermost-enterprise-edition",
				Version:     operatortest.LatestStableMattermostVersion,
				IngressName: "foo-explicit.mattermost.dev",
				FileStore: mmv1beta.FileStore{
					Local: &mmv1beta.LocalFileStore{
						Enabled:     true,
						StorageSize: "1Gi",
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce, corev1.ReadOnlyMany},
					},
				},
			},
		}

		_, err := reconciler.checkLocalFileStore(explicitMM, logger)
		require.NoError(t, err)

		foundPvc := &corev1.PersistentVolumeClaim{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: explicitMMName, Namespace: mmNamespace}, foundPvc)
		require.NoError(t, err)
		require.NotNil(t, foundPvc)

		// Verify that explicit access modes are used
		expectedAccessModes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce, corev1.ReadOnlyMany}
		assert.ElementsMatch(t, expectedAccessModes, foundPvc.Spec.AccessModes)
	})

	t.Run("backwards compatibility - preserve existing ReadWriteOnce", func(t *testing.T) {
		// The original PVC from the first test should still have ReadWriteMany (since it was created as new)
		// But let's test backwards compatibility by getting the existing PVC and checking its access modes
		foundPvc := &corev1.PersistentVolumeClaim{}
		err := reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, foundPvc)
		require.NoError(t, err)
		require.NotNil(t, foundPvc)

		// Save the current access modes
		originalAccessModes := foundPvc.Spec.AccessModes

		// Run checkLocalFileStore again without specifying AccessModes
		mm.Spec.FileStore.Local.AccessModes = nil
		_, err = reconciler.checkLocalFileStore(mm, logger)
		require.NoError(t, err)

		// Get the PVC again and verify access modes were preserved
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, foundPvc)
		require.NoError(t, err)

		// Should preserve the original access modes for backwards compatibility
		assert.Equal(t, originalAccessModes, foundPvc.Spec.AccessModes)
	})

	t.Run("error when trying to change existing PVC access modes", func(t *testing.T) {
		// Get current PVC to check its access modes
		foundPvc := &corev1.PersistentVolumeClaim{}
		err := reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, foundPvc)
		require.NoError(t, err)

		// Try to specify different access modes - should return an error
		originalAccessModes := foundPvc.Spec.AccessModes
		var differentAccessModes []corev1.PersistentVolumeAccessMode
		if len(originalAccessModes) > 0 && originalAccessModes[0] == corev1.ReadWriteMany {
			differentAccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
		} else {
			differentAccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
		}

		mm.Spec.FileStore.Local.AccessModes = differentAccessModes
		_, err = reconciler.checkLocalFileStore(mm, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot change PVC access modes")
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
		require.NoError(t, err)

		service := &corev1.Service{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, service)
		require.NoError(t, err)

		service.Spec.ClusterIPs = []string{"10.10.10.10", "10.10.10.11"}
		service.Spec.ClusterIP = "10.10.10.10"
		err = reconciler.Client.Update(context.TODO(), service)
		require.NoError(t, err)

		mm.Spec.ResourceLabels = map[string]string{"myLabel": "test"}
		err = reconciler.checkMattermostService(mm, currentMMStatus, logger)
		require.NoError(t, err)

		modified := &corev1.Service{}
		err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: mmName, Namespace: mmNamespace}, modified)
		require.NoError(t, err)
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
	forbiddenServicePatch = `[
	{
		"op":"replace",
		"path":"/spec/selector",
		"value":{"app":"attacker"}
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

				recStatus, err := reconciler.checkMattermostDeployment(mm, dbInfo, fileStoreInfo, &mmStatus, logger)
				require.NoError(t, err)
				assert.Equal(t, true, recStatus.ResourcesReady)

				deployKey := types.NamespacedName{Name: mmName, Namespace: mmNamespace}
				deploy := appsv1.Deployment{}
				err = fakeClient.Get(context.Background(), deployKey, &deploy)
				require.NoError(t, err)

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
			{
				description: "continue without patch on forbidden path",
				patch: &mmv1beta.ResourcePatch{
					Service: &mmv1beta.Patch{
						Patch: forbiddenServicePatch,
					},
				},
				assertFn: func(t *testing.T, svc *corev1.Service, mmStatus mmv1beta.MattermostStatus) {
					assert.Equal(t, 2, len(svc.Spec.Ports))

					require.NotNil(t, mmStatus.ResourcePatch)
					require.NotNil(t, mmStatus.ResourcePatch.ServicePatch)
					assert.Contains(t, mmStatus.ResourcePatch.ServicePatch.Error, "forbidden path")
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
				require.NoError(t, err)

				deployKey := types.NamespacedName{Name: mmName, Namespace: mmNamespace}
				svc := corev1.Service{}
				err = fakeClient.Get(context.Background(), deployKey, &svc)
				require.NoError(t, err)

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
	c := fake.NewClientBuilder().Build()
	r := &MattermostReconciler{
		Client:         c,
		Scheme:         s,
		Log:            logger,
		MaxReconciling: 5,
		Resources:      resources.NewResourceHelper(c, s),
	}
	return logger, c, r
}

func fixedDBAndFileStoreInfo(t *testing.T, mm *mmv1beta.Mattermost) (mattermostApp.DatabaseConfig, mattermostApp.FileStoreConfig) {
	// An external database is the fixed choice for these tests; operator-managed
	// An external database is required; configure database.external.
	if mm.Spec.Database.External == nil {
		mm.Spec.Database.External = &mmv1beta.ExternalDatabase{Secret: "dbSecret"}
	}

	dbInfo, err := mattermostApp.NewExternalDBConfig(mm, corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "dbSecret"},
		Data: map[string][]byte{
			"DB_CONNECTION_STRING": []byte("mysql://user:pass@tcp(mysql:3306)/mattermost"),
		},
	})
	require.NoError(t, err)

	// An external file store is the fixed choice for these tests; operator-managed
	// MinIO no longer exists and a file store is now mandatory.
	if mm.Spec.FileStore.External == nil {
		mm.Spec.FileStore.External = &mmv1beta.ExternalFileStore{
			URL:    "s3.example.com",
			Bucket: "test-bucket",
			Secret: "fileStoreSecret",
		}
	}

	fsConfig, err := mattermostApp.NewExternalFileStoreInfo(mm, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "fileStoreSecret"},
		Data: map[string][]byte{
			"accesskey": []byte("access-key"),
			"secretkey": []byte("secret-key"),
		},
	})
	require.NoError(t, err)

	return dbInfo, fsConfig
}

func TestCheckMattermostHTTPRoute(t *testing.T) {
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
			HTTPRoute: &mmv1beta.HTTPRouteSpec{
				Enabled:    true,
				Host:       "mm.example.com",
				GatewayRef: mmv1beta.GatewayReference{Name: "shared-gateway"},
			},
		},
	}

	routeKey := types.NamespacedName{Name: mmName, Namespace: mmNamespace}

	fetchRoute := func(t *testing.T) (*gatewayv1.HTTPRoute, error) {
		t.Helper()
		found := &gatewayv1.HTTPRoute{}
		return found, reconciler.Client.Get(context.TODO(), routeKey, found)
	}

	requireRoute := func(t *testing.T) *gatewayv1.HTTPRoute {
		t.Helper()
		found, err := fetchRoute(t)
		require.NoError(t, err)
		return found
	}

	hostnamesOf := func(t *testing.T, route *gatewayv1.HTTPRoute) []string {
		t.Helper()
		hostnames := make([]string, 0, len(route.Spec.Hostnames))
		for _, h := range route.Spec.Hostnames {
			hostnames = append(hostnames, string(h))
		}
		return hostnames
	}

	t.Run("created when enabled", func(t *testing.T) {
		require.NoError(t, reconciler.checkMattermostHTTPRoute(mm, logger))

		found := requireRoute(t)
		assert.Equal(t, []string{"mm.example.com"}, hostnamesOf(t, found))
		require.Len(t, found.GetOwnerReferences(), 1)
		assert.Equal(t, mmName, found.GetOwnerReferences()[0].Name)
	})

	t.Run("reconcile is idempotent", func(t *testing.T) {
		// Guards against an update loop: JSON merge patch replaces lists wholesale,
		// so a generated field that does not round trip would make every reconcile
		// issue a write. A stable resourceVersion proves the patch was empty.
		before := requireRoute(t)
		require.NoError(t, reconciler.checkMattermostHTTPRoute(mm, logger))
		after := requireRoute(t)
		assert.Equal(t, before.GetResourceVersion(), after.GetResourceVersion())
	})

	t.Run("manual drift is reverted", func(t *testing.T) {
		original := requireRoute(t)

		modified := original.DeepCopy()
		modified.Spec.Hostnames = []gatewayv1.Hostname{"tampered.example.com"}
		modified.SetLabels(nil)
		require.NoError(t, reconciler.Client.Update(context.TODO(), modified))

		require.NoError(t, reconciler.checkMattermostHTTPRoute(mm, logger))

		found := requireRoute(t)
		assert.Equal(t, []string{"mm.example.com"}, hostnamesOf(t, found))
		assert.Equal(t, original.GetLabels(), found.GetLabels())
	})

	t.Run("spec changes are applied", func(t *testing.T) {
		mm.Spec.HTTPRoute.Hosts = []mmv1beta.IngressHost{{HostName: "extra.example.com"}}

		require.NoError(t, reconciler.checkMattermostHTTPRoute(mm, logger))

		assert.Equal(t, []string{"mm.example.com", "extra.example.com"}, hostnamesOf(t, requireRoute(t)))
	})

	t.Run("deleted when disabled", func(t *testing.T) {
		mm.Spec.HTTPRoute.Enabled = false

		require.NoError(t, reconciler.checkMattermostHTTPRoute(mm, logger))

		_, err := fetchRoute(t)
		require.Error(t, err)
		assert.True(t, k8sErrors.IsNotFound(err), "expected HTTPRoute to be deleted, got %v", err)
	})

	t.Run("disabled and already absent is a no-op", func(t *testing.T) {
		require.NoError(t, reconciler.checkMattermostHTTPRoute(mm, logger))
	})

	t.Run("no HTTPRoute section is a no-op", func(t *testing.T) {
		mm.Spec.HTTPRoute = nil
		require.NoError(t, reconciler.checkMattermostHTTPRoute(mm, logger))
	})
}

// TestUseServiceLoadBalancerSuppressesHTTPRoute covers switching an installation
// to Service load balancer mode after an HTTPRoute already exists. That mode does
// not expose the Service port the route targets, so the route has to be removed
// rather than left behind.
//
// Only the HTTPRoute is asserted here. Whether Service load balancer mode should
// also clean up a leftover Ingress or IngressClass is a pre-existing question in
// the Ingress path and is deliberately out of scope for HTTPRoute support.
func TestUseServiceLoadBalancerSuppressesHTTPRoute(t *testing.T) {
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
			HTTPRoute: &mmv1beta.HTTPRouteSpec{
				Enabled:    true,
				Host:       "mm.example.com",
				GatewayRef: mmv1beta.GatewayReference{Name: "shared-gateway"},
			},
		},
	}

	key := types.NamespacedName{Name: mmName, Namespace: mmNamespace}

	httpRouteExists := func(t *testing.T) bool {
		t.Helper()
		found := &gatewayv1.HTTPRoute{}
		err := reconciler.Client.Get(context.TODO(), key, found)
		if err != nil && k8sErrors.IsNotFound(err) {
			return false
		}
		require.NoError(t, err)
		return true
	}

	t.Run("exists while routing through the cluster", func(t *testing.T) {
		require.NoError(t, reconciler.checkMattermostHTTPRoute(mm, logger))
		assert.True(t, httpRouteExists(t))
	})

	t.Run("removed once useServiceLoadBalancer is enabled", func(t *testing.T) {
		mm.Spec.UseServiceLoadBalancer = true

		require.NoError(t, reconciler.checkMattermostHTTPRoute(mm, logger))
		assert.False(t, httpRouteExists(t), "HTTPRoute should be removed in Service load balancer mode")
	})

	t.Run("not recreated while in Service load balancer mode", func(t *testing.T) {
		require.NoError(t, reconciler.checkMattermostHTTPRoute(mm, logger))
		assert.False(t, httpRouteExists(t))
	})

	t.Run("returns when useServiceLoadBalancer is turned back off", func(t *testing.T) {
		mm.Spec.UseServiceLoadBalancer = false

		require.NoError(t, reconciler.checkMattermostHTTPRoute(mm, logger))
		assert.True(t, httpRouteExists(t))
	})
}
