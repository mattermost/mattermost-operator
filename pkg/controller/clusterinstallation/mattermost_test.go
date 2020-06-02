package clusterinstallation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	blubr "github.com/mattermost/blubr"
	"github.com/mattermost/mattermost-operator/pkg/apis"
	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	mattermostmysql "github.com/mattermost/mattermost-operator/pkg/components/mysql"
	operatortest "github.com/mattermost/mattermost-operator/test"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestCheckMattermost(t *testing.T) {
	// Setup logging for the reconciler so we can see what happened on failure.
	logger := blubr.InitLogger()
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
			Version:     operatortest.LatestStableMattermostVersion,
			IngressName: "foo.mattermost.dev",
		},
	}

	apis.AddToScheme(scheme.Scheme)
	s := scheme.Scheme
	s.AddKnownTypes(mattermostv1alpha1.SchemeGroupVersion, ci)
	r := &ReconcileClusterInstallation{client: fake.NewFakeClient(), scheme: s}

	err := prepAllDependencyTestResources(r.client, ci)
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
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Spec.Selector, found.Spec.Selector)
		assert.Equal(t, original.Spec.Ports, found.Spec.Ports)
	})

	t.Run("ingress no tls", func(t *testing.T) {
		ci.Spec.UseIngressTLS = false
		err = r.checkMattermostIngress(ci, ci.Name, ci.Spec.IngressName, ci.Spec.IngressAnnotations, logger)
		assert.NoError(t, err)

		found := &v1beta1.Ingress{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
		require.NoError(t, err)
		require.NotNil(t, found)
		require.Nil(t, found.Spec.TLS)

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
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
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

		err = r.client.Update(context.TODO(), modified)
		require.NoError(t, err)
		err = r.checkMattermostIngress(ci, ci.Name, ci.Spec.IngressName, ci.Spec.IngressAnnotations, logger)
		require.NoError(t, err)
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: ciName, Namespace: ciNamespace}, found)
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
		assert.Equal(t, original.GetLabels(), found.GetLabels())
		assert.Equal(t, original.Spec.Replicas, found.Spec.Replicas)
		assert.Equal(t, original.Spec.Template, found.Spec.Template)
	})

	t.Run("final check", func(t *testing.T) {
		t.Run("database secret", func(t *testing.T) {
			dbSecret := &corev1.Secret{}
			err := r.client.Get(context.TODO(), types.NamespacedName{Name: mattermostmysql.DefaultDatabaseSecretName(ciName), Namespace: ciNamespace}, dbSecret)
			require.NoError(t, err)

			dbInfo := getDatabaseInfoFromSecret(dbSecret)
			require.NoError(t, dbInfo.IsValid())
		})
	})
}

func TestCheckMattermostExternalDB(t *testing.T) {
	// Setup logging for the reconciler so we can see what happened on failure.
	logger := blubr.InitLogger()
	logger = logger.WithName("test.opr")
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
			CACertificates: mattermostv1alpha1.CACertificates{
				SecretName: "ca-certificates-secret-name",
				Path:       "/etc/ssl/certs/my_custom_path",
			},
		},
	}

	apis.AddToScheme(scheme.Scheme)
	s := scheme.Scheme
	s.AddKnownTypes(mattermostv1alpha1.SchemeGroupVersion, ci)
	r := &ReconcileClusterInstallation{client: fake.NewFakeClient(), scheme: s}

	err := prepAllDependencyTestResources(r.client, ci)
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
	err = r.client.Create(context.TODO(), externalDBSecret)
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
		assert.Equal(t, original.GetName(), found.GetName())
		assert.Equal(t, original.GetNamespace(), found.GetNamespace())
		assert.Equal(t, original.Spec.Selector, found.Spec.Selector)
		assert.Equal(t, original.Spec.Ports, found.Spec.Ports)
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

	t.Run("final check", func(t *testing.T) {
		t.Run("default database secret should be missing", func(t *testing.T) {
			dbSecret := &corev1.Secret{}
			err := r.client.Get(context.TODO(), types.NamespacedName{Name: mattermostmysql.DefaultDatabaseSecretName(ciName), Namespace: ciNamespace}, dbSecret)
			require.Error(t, err)
		})
	})
}

func TestMakeCertificateFilesMap(t *testing.T) {
	pemData := `
-----BEGIN CERTIFICATE-----
MIIEBjCCAu6gAwIBAgIJAMc0ZzaSUK51MA0GCSqGSIb3DQEBCwUAMIGPMQswCQYD
VQQGEwJVUzEQMA4GA1UEBwwHU2VhdHRsZTETMBEGA1UECAwKV2FzaGluZ3RvbjEi
MCAGA1UECgwZQW1hem9uIFdlYiBTZXJ2aWNlcywgSW5jLjETMBEGA1UECwwKQW1h
em9uIFJEUzEgMB4GA1UEAwwXQW1hem9uIFJEUyBSb290IDIwMTkgQ0EwHhcNMTkw
ODIyMTcwODUwWhcNMjQwODIyMTcwODUwWjCBjzELMAkGA1UEBhMCVVMxEDAOBgNV
BAcMB1NlYXR0bGUxEzARBgNVBAgMCldhc2hpbmd0b24xIjAgBgNVBAoMGUFtYXpv
biBXZWIgU2VydmljZXMsIEluYy4xEzARBgNVBAsMCkFtYXpvbiBSRFMxIDAeBgNV
BAMMF0FtYXpvbiBSRFMgUm9vdCAyMDE5IENBMIIBIjANBgkqhkiG9w0BAQEFAAOC
AQ8AMIIBCgKCAQEArXnF/E6/Qh+ku3hQTSKPMhQQlCpoWvnIthzX6MK3p5a0eXKZ
oWIjYcNNG6UwJjp4fUXl6glp53Jobn+tWNX88dNH2n8DVbppSwScVE2LpuL+94vY
0EYE/XxN7svKea8YvlrqkUBKyxLxTjh+U/KrGOaHxz9v0l6ZNlDbuaZw3qIWdD/I
6aNbGeRUVtpM6P+bWIoxVl/caQylQS6CEYUk+CpVyJSkopwJlzXT07tMoDL5WgX9
O08KVgDNz9qP/IGtAcRduRcNioH3E9v981QO1zt/Gpb2f8NqAjUUCUZzOnij6mx9
McZ+9cWX88CRzR0vQODWuZscgI08NvM69Fn2SQIDAQABo2MwYTAOBgNVHQ8BAf8E
BAMCAQYwDwYDVR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQUc19g2LzLA5j0Kxc0LjZa
pmD/vB8wHwYDVR0jBBgwFoAUc19g2LzLA5j0Kxc0LjZapmD/vB8wDQYJKoZIhvcN
AQELBQADggEBAHAG7WTmyjzPRIM85rVj+fWHsLIvqpw6DObIjMWokpliCeMINZFV
ynfgBKsf1ExwbvJNzYFXW6dihnguDG9VMPpi2up/ctQTN8tm9nDKOy08uNZoofMc
NUZxKCEkVKZv+IL4oHoeayt8egtv3ujJM6V14AstMQ6SwvwvA93EP/Ug2e4WAXHu
cbI1NAbUgVDqp+DRdfvZkgYKryjTWd/0+1fS8X1bBZVWzl7eirNVnHbSH2ZDpNuY
0SBd8dj5F6ld3t58ydZbrTHze7JJOd8ijySAp4/kiu9UfZWuTPABzDa/DSdz9Dk/
zPW4CXXvhLmE02TA9/HeCw3KEHIwicNuEfw=
-----END CERTIFICATE-----
`

	certFilesMap, err := makeCertificateFilesMap(nil)
	require.NoError(t, err)
	require.NotNil(t, certFilesMap)
	assert.Equal(t, 0, len(certFilesMap))

	secret := corev1.Secret{}
	certFilesMap, err = makeCertificateFilesMap(&secret)
	require.NoError(t, err)
	require.NotNil(t, certFilesMap)
	assert.Equal(t, 0, len(certFilesMap))

	secret.Data = map[string][]byte{
		pemData: []byte(pemData),
	}
	certFilesMap, err = makeCertificateFilesMap(&secret)
	require.Error(t, err)
	require.Equal(t, "invalid certificate file: filename exceeded maximum length of 253 charecters", err.Error())
	require.NotNil(t, certFilesMap)
	assert.Equal(t, 0, len(certFilesMap))

	secret.Data = map[string][]byte{
		"certificate-file.pem":  []byte(pemData),
		"certificate-file2.pem": []byte(pemData),
	}
	certFilesMap, err = makeCertificateFilesMap(&secret)
	require.NoError(t, err)
	require.NotNil(t, certFilesMap)
	assert.Equal(t, 2, len(certFilesMap))
	assert.Equal(t, certFilesMap["certificatefilepem"], "certificate-file.pem")
	assert.Equal(t, certFilesMap["certificatefile2pem"], "certificate-file2.pem")

	secret.Data = map[string][]byte{
		"certificate-file.pem": []byte("certificate file validation error"),
	}
	certFilesMap, err = makeCertificateFilesMap(&secret)
	require.Error(t, err)
	require.Equal(t, "invalid certificate file: failed to decode the certificate PEM "+
		"correspondent to the filename certificate-file.pem", err.Error())
	require.NotNil(t, certFilesMap)
	assert.Equal(t, 0, len(certFilesMap))
}
