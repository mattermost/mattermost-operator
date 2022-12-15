package clusterinstallation

import (
	"context"
	"testing"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	mattermostv1beta1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/mattermost/mattermost-operator/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestConvertToMM(t *testing.T) {

	s := prepareSchema(t, scheme.Scheme)
	// Create a fake client to mock API calls.
	c := fake.NewFakeClient()
	// Create a ReconcileClusterInstallation object with the scheme and fake
	// client.
	reconciler := &ClusterInstallationReconciler{
		Client:             c,
		NonCachedAPIReader: c,
		Scheme:             s,
		MaxReconciling:     5,
	}

	externalDBSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "external-db-secret", Namespace: "test-namespace"},
		Data: map[string][]byte{
			"DB_CONNECTION_STRING":    []byte("mysql://endpoint"),
			"DB_CONNECTION_CHECK_URL": []byte("http://endpoint"),
		},
	}

	operatorDBSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "operator-db-secret", Namespace: "test-namespace"},
		Data: map[string][]byte{
			"ROOT_PASSWORD": []byte("root"),
			"USER":          []byte("user"),
			"PASSWORD":      []byte("pass"),
			"DATABASE":      []byte("database1"),
		},
	}

	err := c.Create(context.TODO(), externalDBSecret)
	require.NoError(t, err)
	err = c.Create(context.TODO(), operatorDBSecret)
	require.NoError(t, err)

	for _, testCase := range []struct {
		description         string
		clusterInstallation mattermostv1alpha1.ClusterInstallation
		mattermost          mattermostv1beta1.Mattermost
	}{
		{
			description: "should convert default",
			clusterInstallation: mattermostv1alpha1.ClusterInstallation{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test1",
					Namespace:   "test-1-ns",
					Labels:      map[string]string{"key1": "val1"},
					Annotations: map[string]string{"ann1": "ann_val1"},
				},
				Spec: mattermostv1alpha1.ClusterInstallationSpec{},
			},
			mattermost: mattermostv1beta1.Mattermost{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test1",
					Namespace:   "test-1-ns",
					Labels:      map[string]string{"key1": "val1"},
					Annotations: map[string]string{"ann1": "ann_val1"},
				},
				Spec: mattermostv1beta1.MattermostSpec{
					Database: mattermostv1beta1.Database{
						OperatorManaged: &mattermostv1beta1.OperatorManagedDatabase{},
					},
					FileStore: mattermostv1beta1.FileStore{
						OperatorManaged: &mattermostv1beta1.OperatorManagedMinio{},
					},
				},
			},
		},
		{
			description: "should convert operator managed settings",
			clusterInstallation: mattermostv1alpha1.ClusterInstallation{
				ObjectMeta: fixObjectMeta(),
				Spec: mattermostv1alpha1.ClusterInstallationSpec{
					Database: mattermostv1alpha1.Database{
						Type:                     "mysql",
						StorageSize:              "200Gb",
						Replicas:                 3,
						Resources:                fixResources("100m", "200Mi"),
						InitBucketURL:            "http://init-bucket",
						BackupSchedule:           "schedule",
						BackupURL:                "http://backup",
						BackupRemoteDeletePolicy: "always",
						BackupSecretName:         "backup-secret",
						BackupRestoreSecretName:  "restore-secret",
					},
					Minio: mattermostv1alpha1.Minio{
						StorageSize: "10Gb",
						Servers:     5,
						Resources:   fixResources("500m", "800Mi"),
					},
				},
			},
			mattermost: mattermostv1beta1.Mattermost{
				ObjectMeta: fixObjectMeta(),
				Spec: mattermostv1beta1.MattermostSpec{
					Database: mattermostv1beta1.Database{
						OperatorManaged: &mattermostv1beta1.OperatorManagedDatabase{
							Type:                     "mysql",
							StorageSize:              "200Gb",
							Replicas:                 utils.NewInt32(3),
							Resources:                fixResources("100m", "200Mi"),
							InitBucketURL:            "http://init-bucket",
							BackupSchedule:           "schedule",
							BackupURL:                "http://backup",
							BackupRemoteDeletePolicy: "always",
							BackupSecretName:         "backup-secret",
							BackupRestoreSecretName:  "restore-secret",
						},
					},
					FileStore: mattermostv1beta1.FileStore{
						OperatorManaged: &mattermostv1beta1.OperatorManagedMinio{
							StorageSize: "10Gb",
							Servers:     utils.NewInt32(5),
							Resources:   fixResources("500m", "800Mi"),
						},
					},
				},
			},
		},
		{
			description: "should convert external file store",
			clusterInstallation: mattermostv1alpha1.ClusterInstallation{
				ObjectMeta: fixObjectMeta(),
				Spec: mattermostv1alpha1.ClusterInstallationSpec{
					Minio: mattermostv1alpha1.Minio{
						StorageSize:    "ignored",
						Servers:        100,
						ExternalURL:    "s3.amazon.com",
						ExternalBucket: "my-bucket",
						Secret:         "file-store-secret",
					},
				},
			},
			mattermost: mattermostv1beta1.Mattermost{
				ObjectMeta: fixObjectMeta(),
				Spec: mattermostv1beta1.MattermostSpec{
					Database: mattermostv1beta1.Database{
						OperatorManaged: &mattermostv1beta1.OperatorManagedDatabase{},
					},
					FileStore: mattermostv1beta1.FileStore{
						External: &mattermostv1beta1.ExternalFileStore{
							URL:    "s3.amazon.com",
							Bucket: "my-bucket",
							Secret: "file-store-secret",
						},
					},
				},
			},
		},
		{
			description: "should convert external database",
			clusterInstallation: mattermostv1alpha1.ClusterInstallation{
				ObjectMeta: fixObjectMeta(),
				Spec: mattermostv1alpha1.ClusterInstallationSpec{
					Database: mattermostv1alpha1.Database{Secret: "external-db-secret"},
				},
			},
			mattermost: mattermostv1beta1.Mattermost{
				ObjectMeta: fixObjectMeta(),
				Spec: mattermostv1beta1.MattermostSpec{
					Database: mattermostv1beta1.Database{
						External: &mattermostv1beta1.ExternalDatabase{
							Secret: "external-db-secret",
						},
					},
					FileStore: mattermostv1beta1.FileStore{
						OperatorManaged: &mattermostv1beta1.OperatorManagedMinio{},
					},
				},
			},
		},
		{
			description: "should convert operator managed database from secret",
			clusterInstallation: mattermostv1alpha1.ClusterInstallation{
				ObjectMeta: fixObjectMeta(),
				Spec: mattermostv1alpha1.ClusterInstallationSpec{
					Database: mattermostv1alpha1.Database{Secret: "operator-db-secret"},
				},
			},
			mattermost: mattermostv1beta1.Mattermost{
				ObjectMeta: fixObjectMeta(),
				Spec: mattermostv1beta1.MattermostSpec{
					Database: mattermostv1beta1.Database{
						OperatorManaged: &mattermostv1beta1.OperatorManagedDatabase{},
					},
					FileStore: mattermostv1beta1.FileStore{
						OperatorManaged: &mattermostv1beta1.OperatorManagedMinio{},
					},
				},
			},
		},
		{
			description: "should convert other fields",
			clusterInstallation: mattermostv1alpha1.ClusterInstallation{
				ObjectMeta: fixObjectMeta(),
				Spec: mattermostv1alpha1.ClusterInstallationSpec{
					Image:                   "image",
					Version:                 "ver",
					Size:                    "10000size",
					Replicas:                0,
					Resources:               fixResources("100m", "100Mi"),
					IngressName:             "ingress",
					MattermostLicenseSecret: "license-secret",
					NodeSelector:            map[string]string{"node": "choose-me"},
					Affinity:                &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{}},
					ElasticSearch: mattermostv1alpha1.ElasticSearch{
						Host:     "http://es",
						UserName: "es-user",
						Password: "es-pass",
					},
					UseServiceLoadBalancer: true,
					ServiceAnnotations:     map[string]string{"ann1": "val1"},
					UseIngressTLS:          true,
					ResourceLabels:         map[string]string{"resource": "this-one"},
					IngressAnnotations:     map[string]string{"ingress": "this-one"},
					MattermostEnv: []corev1.EnvVar{
						{
							Name:  "test_env",
							Value: "env val",
						},
					},
					LivenessProbe:  corev1.Probe{InitialDelaySeconds: 10},
					ReadinessProbe: corev1.Probe{SuccessThreshold: 20},
				},
			},
			mattermost: mattermostv1beta1.Mattermost{
				ObjectMeta: fixObjectMeta(),
				Spec: mattermostv1beta1.MattermostSpec{
					Size:     "10000size",
					Image:    "image",
					Version:  "ver",
					Replicas: nil,
					MattermostEnv: []corev1.EnvVar{
						{
							Name:  "test_env",
							Value: "env val",
						},
					},
					LicenseSecret:          "license-secret",
					IngressName:            "ingress",
					UseServiceLoadBalancer: true,
					ServiceAnnotations:     map[string]string{"ann1": "val1"},
					UseIngressTLS:          true,
					ResourceLabels:         map[string]string{"resource": "this-one"},
					IngressAnnotations:     map[string]string{"ingress": "this-one"},
					Database: mattermostv1beta1.Database{
						OperatorManaged: &mattermostv1beta1.OperatorManagedDatabase{},
					},
					FileStore: mattermostv1beta1.FileStore{
						OperatorManaged: &mattermostv1beta1.OperatorManagedMinio{},
					},
					ElasticSearch: mattermostv1beta1.ElasticSearch{
						Host:     "http://es",
						UserName: "es-user",
						Password: "es-pass",
					},
					Scheduling: mattermostv1beta1.Scheduling{
						NodeSelector: map[string]string{"node": "choose-me"},
						Affinity:     &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{}},
						Resources:    fixResources("100m", "100Mi"),
					},
					Probes: mattermostv1beta1.Probes{
						LivenessProbe:  corev1.Probe{InitialDelaySeconds: 10},
						ReadinessProbe: corev1.Probe{SuccessThreshold: 20},
					},
				},
			},
		},
		{
			description: "should set replicas to 0 if negative",
			clusterInstallation: mattermostv1alpha1.ClusterInstallation{
				ObjectMeta: fixObjectMeta(),
				Spec: mattermostv1alpha1.ClusterInstallationSpec{
					Replicas: -1,
				},
			},
			mattermost: mattermostv1beta1.Mattermost{
				ObjectMeta: fixObjectMeta(),
				Spec: mattermostv1beta1.MattermostSpec{
					Database: mattermostv1beta1.Database{
						OperatorManaged: &mattermostv1beta1.OperatorManagedDatabase{},
					},
					FileStore: mattermostv1beta1.FileStore{
						OperatorManaged: &mattermostv1beta1.OperatorManagedMinio{},
					},
					Replicas: utils.NewInt32(0),
				},
			},
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			mm, err := reconciler.ConvertToMM(&testCase.clusterInstallation)
			require.NoError(t, err)

			assert.Equal(t, &testCase.mattermost, mm)
		})
	}

	t.Run("should return error if secret not found", func(t *testing.T) {
		ci := mattermostv1alpha1.ClusterInstallation{
			ObjectMeta: fixObjectMeta(),
			Spec: mattermostv1alpha1.ClusterInstallationSpec{
				Database: mattermostv1alpha1.Database{Secret: "invalid-secret"},
			},
		}

		_, err := reconciler.ConvertToMM(&ci)
		require.Error(t, err)
	})

	t.Run("should return error if secret is invalid", func(t *testing.T) {
		invalidDBSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "invalid-db-secret", Namespace: "test-namespace"},
			Data: map[string][]byte{
				"ROOT_PASSWORD": []byte("root"),
			},
		}

		err := c.Create(context.TODO(), invalidDBSecret)
		require.NoError(t, err)

		ci := mattermostv1alpha1.ClusterInstallation{
			ObjectMeta: fixObjectMeta(),
			Spec: mattermostv1alpha1.ClusterInstallationSpec{
				Database: mattermostv1alpha1.Database{Secret: "invalid-db-secret"},
			},
		}

		_, err = reconciler.ConvertToMM(&ci)
		require.Error(t, err)
	})
}

func fixObjectMeta() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      "test-name",
		Namespace: "test-namespace",
	}
}

func fixResources(cpu, mem string) corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			"cpu":    resource.MustParse(cpu),
			"memory": resource.MustParse(mem),
		},
		Limits: corev1.ResourceList{
			"cpu":    resource.MustParse(cpu),
			"memory": resource.MustParse(mem),
		},
	}
}
