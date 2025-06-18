package mattermost

import (
	"fmt"
	"testing"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/mattermost/mattermost-operator/pkg/database"
	"github.com/mattermost/mattermost-operator/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// Going forward, when new logic is added for constructing a kubernetes resource
// to run Mattermost, the test for that resource should be updated to check that
// the right configuration is present.

func TestGenerateService_V1Beta(t *testing.T) {
	tests := []struct {
		name string
		spec mmv1beta.MattermostSpec
	}{
		{
			name: "type headless",
			spec: mmv1beta.MattermostSpec{},
		},
		{
			name: "type load-balancer",
			spec: mmv1beta.MattermostSpec{
				UseServiceLoadBalancer: true,
			},
		},
		{
			name: "type aws controller",
			spec: mmv1beta.MattermostSpec{
				AWSLoadBalancerController: &mmv1beta.AWSLoadBalancerController{
					Enabled: true,
				},
			},
		},
		{
			name: "type headless, has annotations",
			spec: mmv1beta.MattermostSpec{
				ServiceAnnotations: map[string]string{
					"custom":       "1",
					"other-custom": "2",
				},
			},
		},
		{
			name: "type load-balancer, has annotations",
			spec: mmv1beta.MattermostSpec{
				UseServiceLoadBalancer: true,
				ServiceAnnotations: map[string]string{
					"custom":       "1",
					"other-custom": "2",
				},
			},
		},
		{
			name: "type aws controller, has annotations",
			spec: mmv1beta.MattermostSpec{
				AWSLoadBalancerController: &mmv1beta.AWSLoadBalancerController{
					Enabled: true,
				},
				ServiceAnnotations: map[string]string{
					"custom":       "1",
					"other-custom": "2",
				},
			},
		},
		{
			name: "has labels",
			spec: mmv1beta.MattermostSpec{
				ResourceLabels: map[string]string{
					"resource": "label",
				},
				PodTemplate: &mmv1beta.PodTemplate{
					ExtraLabels: map[string]string{
						"pod": "label",
					},
				},
			},
		},
	}

	expectPort := func(t *testing.T, service *corev1.Service, portNumber int32, appProtocol *string) {
		t.Helper()
		for _, port := range service.Spec.Ports {
			if port.Port == portNumber {
				if *port.AppProtocol == *appProtocol {
					return
				}
				assert.Fail(t, fmt.Sprintf("failed to find appProtocol %s on port %d", *appProtocol, portNumber))
			}
		}
		assert.Fail(t, fmt.Sprintf("failed to find port %d on service", portNumber))
	}

	expectLabels := func(t *testing.T, service *corev1.Service, mmspec mmv1beta.MattermostSpec) {
		t.Helper()
		if mmspec.ResourceLabels != nil {
			for k, v := range mmspec.ResourceLabels {
				if service.Labels[k] != v {
					assert.Fail(t, "Resource labels not found on service", fmt.Sprintf("%s=%s", k, v))
				}
			}
		}
		if mmspec.PodTemplate != nil && mmspec.PodTemplate.ExtraLabels != nil {
			for k, v := range mmspec.PodTemplate.ExtraLabels {
				if service.Labels[k] == v {
					assert.Fail(t, "Pod labels should not be applied to service", fmt.Sprintf("%s=%s", k, v))
				}
			}
		}
	}

	expectAnnotations := func(t *testing.T, service *corev1.Service, mmspec mmv1beta.MattermostSpec) {
		t.Helper()
		if mmspec.ServiceAnnotations != nil {
			for k, v := range mmspec.ServiceAnnotations {
				if service.Annotations[k] != v {
					assert.Fail(t, "Resource annotation not found on service", fmt.Sprintf("%s=%s", k, v))
				}
			}
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mattermost := &mmv1beta.Mattermost{
				Spec: tt.spec,
			}

			service := GenerateServiceV1Beta(mattermost)
			require.NotNil(t, service)

			if mattermost.Spec.UseServiceLoadBalancer {
				assert.Equal(t, service.Spec.Type, corev1.ServiceTypeLoadBalancer)
				expectPort(t, service, 80, utils.NewString("http"))
				expectPort(t, service, 443, utils.NewString("https"))
			} else {
				expectPort(t, service, 8065, utils.NewString("http"))
				expectPort(t, service, 8067, utils.NewString("http"))
			}

			expectLabels(t, service, mattermost.Spec)
			expectAnnotations(t, service, mattermost.Spec)

			assert.True(t, service.Spec.PublishNotReadyAddresses)
		})
	}
}

func TestGenerateIngress_V1Beta(t *testing.T) {
	mmName := "my-mm"

	expectedMMIngressRule := networkingv1.IngressRuleValue{
		HTTP: &networkingv1.HTTPIngressRuleValue{
			Paths: []networkingv1.HTTPIngressPath{
				{
					Path: "/",
					Backend: networkingv1.IngressBackend{
						Service: &networkingv1.IngressServiceBackend{
							Name: mmName,
							Port: networkingv1.ServiceBackendPort{
								Number: 8065,
							},
						},
					},
					PathType: &defaultIngressPathType,
				},
			},
		},
	}

	expectedIngresMeta := func(extraAnnotations map[string]string) metav1.ObjectMeta {
		annotations := map[string]string{"nginx.ingress.kubernetes.io/proxy-body-size": "1000M"}
		for k, v := range extraAnnotations {
			annotations[k] = v
		}

		return metav1.ObjectMeta{
			Name: mmName,
			Labels: map[string]string{
				"app": "mattermost",
				"installation.mattermost.com/installation": mmName,
				"installation.mattermost.com/resource":     mmName,
			},
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "installation.mattermost.com/v1beta1",
					Kind:               "Mattermost",
					Name:               mmName,
					BlockOwnerDeletion: utils.NewBool(true),
					Controller:         utils.NewBool(true),
				},
			},
		}
	}

	tests := []struct {
		name            string
		spec            mmv1beta.MattermostSpec
		expectedIngress *networkingv1.Ingress
	}{
		{
			name: "no tls, no ingress class",
			spec: mmv1beta.MattermostSpec{Ingress: &mmv1beta.Ingress{Enabled: true, Host: "test"}},
			expectedIngress: &networkingv1.Ingress{
				ObjectMeta: expectedIngresMeta(map[string]string{ingressClassAnnotation: "nginx"}),
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host:             "test",
							IngressRuleValue: expectedMMIngressRule,
						},
					},
				},
			},
		},
		{
			name: "custom tls secret, custom ingress class",
			spec: mmv1beta.MattermostSpec{Ingress: &mmv1beta.Ingress{Host: "test", IngressClass: utils.NewString("custom-nginx"), TLSSecret: "my-secret"}},
			expectedIngress: &networkingv1.Ingress{
				ObjectMeta: expectedIngresMeta(nil),
				Spec: networkingv1.IngressSpec{
					IngressClassName: utils.NewString("custom-nginx"),
					Rules: []networkingv1.IngressRule{
						{
							Host:             "test",
							IngressRuleValue: expectedMMIngressRule,
						},
					},
					TLS: []networkingv1.IngressTLS{
						{
							Hosts:      []string{"test"},
							SecretName: "my-secret",
						},
					},
				},
			},
		},
		{
			name: "default tls secret, no ingress spec",
			spec: mmv1beta.MattermostSpec{
				IngressName:   "test",
				UseIngressTLS: true,
			},
			expectedIngress: &networkingv1.Ingress{
				ObjectMeta: expectedIngresMeta(map[string]string{ingressClassAnnotation: "nginx"}),
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host:             "test",
							IngressRuleValue: expectedMMIngressRule,
						},
					},
					TLS: []networkingv1.IngressTLS{
						{
							Hosts:      []string{"test"},
							SecretName: "test-tls-cert",
						},
					},
				},
			},
		},
		{
			name: "custom ingress class annotation",
			spec: mmv1beta.MattermostSpec{
				IngressName:        "test",
				IngressAnnotations: map[string]string{ingressClassAnnotation: "custom-nginx"},
			},
			expectedIngress: &networkingv1.Ingress{
				ObjectMeta: expectedIngresMeta(map[string]string{ingressClassAnnotation: "custom-nginx"}),
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host:             "test",
							IngressRuleValue: expectedMMIngressRule,
						},
					},
				},
			},
		},
		{
			name: "support multiple hosts and ignore duplicates",
			spec: mmv1beta.MattermostSpec{
				Ingress: &mmv1beta.Ingress{
					Enabled: true,
					Host:    "test",
					Hosts: []mmv1beta.IngressHost{
						{HostName: "test"},
						{HostName: "test-2"},
						{HostName: "test-2"},
						{HostName: "test-3"},
					},
					IngressClass: utils.NewString("nginx"),
				},
			},
			expectedIngress: &networkingv1.Ingress{
				ObjectMeta: expectedIngresMeta(nil),
				Spec: networkingv1.IngressSpec{
					IngressClassName: utils.NewString("nginx"),
					Rules: []networkingv1.IngressRule{
						{
							Host:             "test",
							IngressRuleValue: expectedMMIngressRule,
						},
						{
							Host:             "test-2",
							IngressRuleValue: expectedMMIngressRule,
						},
						{
							Host:             "test-3",
							IngressRuleValue: expectedMMIngressRule,
						},
					},
				},
				Status: networkingv1.IngressStatus{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mattermost := &mmv1beta.Mattermost{
				ObjectMeta: metav1.ObjectMeta{Name: mmName},
				Spec:       tt.spec,
			}

			ingress := GenerateIngressV1Beta(mattermost)
			require.NotNil(t, ingress)
			assert.Equal(t, tt.expectedIngress, ingress)
			assert.Equal(t, networkingv1.PathTypeImplementationSpecific, *ingress.Spec.Rules[0].HTTP.Paths[0].PathType)
		})
	}
}

func TestGenerateJobServerDeployment_V1Beta(t *testing.T) {
	replicas := int32(3)
	mattermost := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: mmv1beta.MattermostSpec{
			Replicas:      &replicas,
			LicenseSecret: "license-secret",
			PodTemplate:   &mmv1beta.PodTemplate{},
		},
	}
	databaseConfig := &ExternalDBConfig{
		secretName:    "database-secret",
		hasDBCheckURL: true,
		dbType:        database.PostgreSQLDatabase,
	}
	fileStoreInfo := &ExternalFileStore{
		fsInfo: FileStoreInfo{
			secretName: "file-store-secret",
			bucketName: "file-store-bucket",
			url:        "s3.amazon.com",
			useS3SSL:   true,
		},
	}

	jobServerdeployment := GenerateJobServerDeploymentV1Beta(mattermost, databaseConfig, fileStoreInfo, mattermost.Name, "", "service-account", "")
	require.NotNil(t, jobServerdeployment)

	assert.Equal(t, "test-jobserver", jobServerdeployment.Name)
	assert.Equal(t, mattermost.MattermostJobServerPodLabels(mattermost.Name), jobServerdeployment.Spec.Template.Labels)
	assert.Equal(t, mattermost.MattermostJobServerPodLabels(mattermost.Name), jobServerdeployment.Spec.Selector.MatchLabels)

	assert.Equal(t, []string{"mattermost", "jobserver"}, jobServerdeployment.Spec.Template.Spec.Containers[0].Command)
	assert.Equal(t, *jobServerdeployment.Spec.Replicas, int32(1))
	require.Len(t, jobServerdeployment.Spec.Template.Spec.Containers, 1)
	assert.Nil(t, jobServerdeployment.Spec.Template.Spec.Containers[0].ReadinessProbe)
	assert.Nil(t, jobServerdeployment.Spec.Template.Spec.Containers[0].LivenessProbe)

	mattermost.Spec.PodTemplate.Command = []string{"mattermost", "custom-command"}
	jobServerdeployment = GenerateJobServerDeploymentV1Beta(mattermost, databaseConfig, fileStoreInfo, mattermost.Name, "", "service-account", "")
	require.NotNil(t, jobServerdeployment)
	assert.Equal(t, []string{"mattermost", "jobserver"}, jobServerdeployment.Spec.Template.Spec.Containers[0].Command)
}

func TestGenerateDeployment_V1Beta(t *testing.T) {
	tests := []struct {
		name            string
		spec            mmv1beta.MattermostSpec
		database        DatabaseConfig
		fileStore       FileStoreConfig
		want            *appsv1.Deployment
		requiredEnv     []string
		requiredEnvVals map[string]string
	}{
		{
			name: "has license",
			spec: mmv1beta.MattermostSpec{
				LicenseSecret: "license-secret",
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Volumes: []corev1.Volume{
								{
									Name: "mattermost-license",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "license-secret",
										},
									},
								},
							},
						},
					},
				},
			},
			requiredEnvVals: map[string]string{"MM_SERVICESETTINGS_LICENSEFILELOCATION": "/mattermost-license/license"},
		},
		{
			name:     "external database",
			spec:     mmv1beta.MattermostSpec{},
			database: &ExternalDBConfig{secretName: "database-secret"},
			want:     &appsv1.Deployment{},
		},
		{
			name:        "external database with reader endpoints",
			spec:        mmv1beta.MattermostSpec{},
			database:    &ExternalDBConfig{secretName: "database-secret", hasReaderEndpoints: true},
			want:        &appsv1.Deployment{},
			requiredEnv: []string{"MM_SQLSETTINGS_DATASOURCEREPLICAS"},
		},
		{
			name: "external known database with check url",
			spec: mmv1beta.MattermostSpec{},
			database: &ExternalDBConfig{
				secretName:    "database-secret",
				hasDBCheckURL: true,
				dbType:        database.PostgreSQLDatabase,
			},
			want: &appsv1.Deployment{},
		},
		{
			name: "external unknown database with check url",
			spec: mmv1beta.MattermostSpec{},
			database: &ExternalDBConfig{
				secretName:    "database-secret",
				hasDBCheckURL: true,
				dbType:        "cockroach",
			},
			want: &appsv1.Deployment{},
		},
		{
			name: "external file store",
			spec: mmv1beta.MattermostSpec{},
			fileStore: &ExternalFileStore{
				fsInfo: FileStoreInfo{
					secretName: "file-store-secret",
					bucketName: "file-store-bucket",
					url:        "s3.amazon.com",
					useS3SSL:   true,
				},
			},
			want:            &appsv1.Deployment{},
			requiredEnvVals: map[string]string{"MM_FILESETTINGS_AMAZONS3SSL": "true"},
		},
		{
			name: "operator managed file store",
			spec: mmv1beta.MattermostSpec{},
			fileStore: &OperatorManagedMinioConfig{
				fsInfo: FileStoreInfo{
					secretName: "file-store-secret",
					bucketName: "file-store-bucket",
					url:        "minio.local.com",
					useS3SSL:   false,
				},
			},
			want:            &appsv1.Deployment{},
			requiredEnvVals: map[string]string{"MM_FILESETTINGS_AMAZONS3SSL": "false"},
		},
		{
			name:      "local file store",
			spec:      mmv1beta.MattermostSpec{},
			fileStore: &LocalFileStore{},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Volumes: []corev1.Volume{
								{
									Name: "mattermost-data",
									VolumeSource: corev1.VolumeSource{
										PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: "",
											ReadOnly:  false,
										},
									},
								},
							},
						},
					},
				},
			},
			requiredEnvVals: map[string]string{"MM_FILESETTINGS_DRIVERNAME": "local"},
		},
		{
			name: "override envs set by default with ones in MM spec",
			spec: mmv1beta.MattermostSpec{
				MattermostEnv: []corev1.EnvVar{
					{Name: "MM_FILESETTINGS_AMAZONS3SSL", Value: "false"},
				},
			},
			fileStore: &ExternalFileStore{
				fsInfo: FileStoreInfo{
					secretName: "file-store-secret",
					bucketName: "file-store-bucket",
					url:        "s3.amazon.com",
					useS3SSL:   true,
				},
			},
			want:            &appsv1.Deployment{},
			requiredEnvVals: map[string]string{"MM_FILESETTINGS_AMAZONS3SSL": "false"},
		},
		{
			name: "image pull policy",
			spec: mmv1beta.MattermostSpec{
				ImagePullPolicy: corev1.PullAlways,
			},
			want: &appsv1.Deployment{},
		},
		{
			name: "node selector 1",
			spec: mmv1beta.MattermostSpec{
				Scheduling: mmv1beta.Scheduling{
					NodeSelector: map[string]string{"type": "compute"},
				},
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{"type": "compute"},
						},
					},
				},
			},
		},
		{
			name: "node selector 2",
			spec: mmv1beta.MattermostSpec{
				Scheduling: mmv1beta.Scheduling{
					NodeSelector: map[string]string{"type": "compute", "size": "big", "region": "iceland"},
				},
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{"type": "compute", "size": "big", "region": "iceland"},
						},
					},
				},
			},
		},
		{
			name: "node selector nil",
			spec: mmv1beta.MattermostSpec{
				Scheduling: mmv1beta.Scheduling{
					NodeSelector: nil,
				},
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeSelector: nil,
						},
					},
				},
			},
		},
		{
			name: "affinity 1",
			spec: mmv1beta.MattermostSpec{
				Scheduling: mmv1beta.Scheduling{
					Affinity: &corev1.Affinity{
						PodAffinity: &corev1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{"key": "value"},
									},
								},
							},
						},
					},
				},
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Affinity: &corev1.Affinity{
								PodAffinity: &corev1.PodAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
										{
											LabelSelector: &metav1.LabelSelector{
												MatchLabels: map[string]string{"key": "value"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "affinity nil",
			spec: mmv1beta.MattermostSpec{
				Scheduling: mmv1beta.Scheduling{
					Affinity: nil,
				},
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Affinity: nil,
						},
					},
				},
			},
		},
		{
			name: "negative app replica",
			spec: mmv1beta.MattermostSpec{
				Replicas: utils.NewInt32(-1),
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: utils.NewInt32(0),
				},
			},
		},
		{
			name: "nil replicas",
			spec: mmv1beta.MattermostSpec{
				Replicas: nil,
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: utils.NewInt32(0),
				},
			},
		},
		{
			name: "volumes",
			spec: mmv1beta.MattermostSpec{
				Volumes:      []corev1.Volume{fixVolume()},
				VolumeMounts: []corev1.VolumeMount{fixVolumeMount()},
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Volumes: []corev1.Volume{fixVolume()},
						},
					},
				},
			},
		},
		{
			name: "volumes and licence",
			spec: mmv1beta.MattermostSpec{
				LicenseSecret: "license-secret",
				Volumes:       []corev1.Volume{fixVolume()},
				VolumeMounts:  []corev1.VolumeMount{fixVolumeMount()},
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Volumes: []corev1.Volume{
								fixVolume(),
								{
									Name: "mattermost-license",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "license-secret",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "elastic search",
			spec: mmv1beta.MattermostSpec{
				ElasticSearch: mmv1beta.ElasticSearch{
					Host:     "http://elastic",
					UserName: "user",
					Password: "password",
				},
			},
			want: &appsv1.Deployment{},
			requiredEnvVals: map[string]string{
				"MM_ELASTICSEARCHSETTINGS_ENABLEINDEXING":  "true",
				"MM_ELASTICSEARCHSETTINGS_ENABLESEARCHING": "true",
				"MM_ELASTICSEARCHSETTINGS_CONNECTIONURL":   "http://elastic",
				"MM_ELASTICSEARCHSETTINGS_USERNAME":        "user",
				"MM_ELASTICSEARCHSETTINGS_PASSWORD":        "password",
			},
		},
		{
			name: "precedence order of labels",
			spec: mmv1beta.MattermostSpec{
				PodTemplate: &mmv1beta.PodTemplate{
					ExtraLabels: map[string]string{
						"app": "extraLabels",
						"pod": "extraLabels",
					},
				},
				ResourceLabels: map[string]string{
					"app":      "resourceLabels",
					"pod":      "resourceLabels",
					"resource": "resourceLabels",
				},
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app":      "mattermost",
								"pod":      "extraLabels",
								"resource": "resourceLabels",
							},
						},
						Spec: corev1.PodSpec{},
					},
				},
			},
		},
		{
			name: "precedence order of annotations",
			spec: mmv1beta.MattermostSpec{
				LicenseSecret: "license-secret", // Add license for Prometheus annotations
				PodTemplate: &mmv1beta.PodTemplate{
					ExtraAnnotations: map[string]string{
						"prometheus.io/path": "/notmetrics",
						"owner":              "test",
					},
				},
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"owner":              "test",
								"prometheus.io/path": "/metrics",
							},
						},
						Spec: corev1.PodSpec{
							Volumes: []corev1.Volume{
								{
									Name: "mattermost-license",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "license-secret",
										},
									},
								},
							},
						},
					},
				},
			},
			requiredEnvVals: map[string]string{"MM_SERVICESETTINGS_LICENSEFILELOCATION": "/mattermost-license/license"},
		},
		{
			name: "dedicated job server",
			spec: mmv1beta.MattermostSpec{
				JobServer: &mmv1beta.JobServer{
					DedicatedJobServer: true,
				},
			},
			want: &appsv1.Deployment{},
			requiredEnvVals: map[string]string{
				"MM_JOBSETTINGS_RUNJOBS": "false",
			},
		},
		{
			name: "custom command",
			spec: mmv1beta.MattermostSpec{
				PodTemplate: &mmv1beta.PodTemplate{
					Command: []string{"mattermost", "custom-command"},
				},
			},
			want: &appsv1.Deployment{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mattermost := &mmv1beta.Mattermost{
				Spec: tt.spec,
			}
			databaseConfig := tt.database
			if databaseConfig == nil {
				databaseConfig = &MySQLDBConfig{}
			}
			fileStoreInfo := tt.fileStore
			if fileStoreInfo == nil {
				fileStoreInfo = NewOperatorManagedFileStoreInfo(mattermost, "minio-secret", "http://minio")
			}

			deployment := GenerateDeploymentV1Beta(mattermost, databaseConfig, fileStoreInfo, "", "", "service-account", "")
			require.NotNil(t, deployment)

			assert.Equal(t, "service-account", deployment.Spec.Template.Spec.ServiceAccountName)
			assert.Equal(t, tt.want.Spec.Template.Spec.NodeSelector, deployment.Spec.Template.Spec.NodeSelector)
			assert.Equal(t, tt.want.Spec.Template.Spec.Affinity, deployment.Spec.Template.Spec.Affinity)
			assert.Equal(t, tt.want.Spec.Template.Spec.Volumes, deployment.Spec.Template.Spec.Volumes)
			assert.Equal(t, len(tt.want.Spec.Template.Spec.Volumes), len(deployment.Spec.Template.Spec.Containers[0].VolumeMounts))

			mattermostAppContainer := mmv1beta.GetMattermostAppContainerFromDeployment(deployment)
			require.NotNil(t, mattermostAppContainer)
			if mattermost.Spec.PodTemplate != nil && mattermost.Spec.PodTemplate.Command != nil {
				assert.Equal(t, mattermost.Spec.PodTemplate.Command, mattermostAppContainer.Command)
			}

			if mattermost.Spec.ImagePullPolicy != "" {
				assert.Equal(t, mattermost.Spec.ImagePullPolicy, mattermostAppContainer.ImagePullPolicy)
			}

			// Basic env var check to ensure the key exists.
			assertEnvVarExists(t, "MM_CONFIG", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_METRICSSETTINGS_LISTENADDRESS", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_METRICSSETTINGS_ENABLE", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_PLUGINSETTINGS_ENABLEUPLOADS", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_CLUSTERSETTINGS_ENABLE", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_CLUSTERSETTINGS_CLUSTERNAME", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_FILESETTINGS_MAXFILESIZE", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_INSTALL_TYPE", mattermostAppContainer.Env)

			// Passed ingress name is empty -- SiteURL should not be set.
			assertEnvVarNotExist(t, "MM_SERVICESETTINGS_SITEURL", mattermostAppContainer.Env)

			for _, env := range tt.requiredEnv {
				assertEnvVarExists(t, env, mattermostAppContainer.Env)
			}

			for env, val := range tt.requiredEnvVals {
				assertEnvVarEqual(t, env, val, mattermostAppContainer.Env)
			}

			// External db check.
			expectedInitContainers := 0 // Due to disabling DB setup job we start with 0 init containers

			if externalDB, ok := tt.database.(*ExternalDBConfig); ok {
				if externalDB.hasDBCheckURL {
					if externalDB.dbType == database.MySQLDatabase ||
						externalDB.dbType == database.PostgreSQLDatabase {
						expectedInitContainers++
						assertEnvVarExists(t, "MM_SQLSETTINGS_DATASOURCE", mattermostAppContainer.Env)
					}
				}
			} else {
				expectedInitContainers++
				assertEnvVarExists(t, "MYSQL_USERNAME", mattermostAppContainer.Env)
				assertEnvVarExists(t, "MYSQL_PASSWORD", mattermostAppContainer.Env)
			}

			if _, ok := fileStoreInfo.(*OperatorManagedMinioConfig); ok {
				expectedInitContainers += 2
			}

			assert.Equal(t, expectedInitContainers, len(deployment.Spec.Template.Spec.InitContainers))

			// Container check.
			assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers))

			// Label/Annotation checks
			if tt.want.Spec.Template.ObjectMeta.Labels != nil {
				for k, v := range tt.want.Spec.Template.ObjectMeta.Labels {
					assert.Equal(t, deployment.Spec.Template.Labels[k], v)
				}
			}
			if tt.want.Spec.Template.ObjectMeta.Annotations != nil {
				for k, v := range tt.want.Spec.Template.ObjectMeta.Annotations {
					assert.Equal(t, deployment.Spec.Template.Annotations[k], v)
				}
			}
		})
	}

	t.Run("custom pod extensions and DB check", func(t *testing.T) {
		customInitContainers := []corev1.Container{
			{Image: "my-check-image", Name: "custom-check"},
			{Image: "my-other-check-image", Name: "other-custom-check"},
		}
		defaultExternalPostgresInitContainers := []corev1.Container{
			{
				Name:            "init-check-database",
				Image:           "postgres:13",
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command:         []string{"sh", "-c", "until pg_isready --dbname=\"$DB_CONNECTION_CHECK_URL\"; do echo waiting for database; sleep 5; done;"},
				Env:             []corev1.EnvVar{{Name: "DB_CONNECTION_CHECK_URL", Value: "", ValueFrom: EnvSourceFromSecret("secret", "DB_CONNECTION_CHECK_URL")}},
			},
		}

		for _, testCase := range []struct {
			description            string
			mmSpec                 mmv1beta.MattermostSpec
			dbConfig               DatabaseConfig
			expectedInitContainers []corev1.Container
		}{
			{
				description: "custom init container with DB check disabled",
				mmSpec: mmv1beta.MattermostSpec{
					Database: mmv1beta.Database{
						External:              &mmv1beta.ExternalDatabase{},
						DisableReadinessCheck: true,
					},
					PodExtensions: mmv1beta.PodExtensions{
						InitContainers: customInitContainers,
					},
				},
				dbConfig:               &ExternalDBConfig{dbType: database.PostgreSQLDatabase, hasDBCheckURL: true},
				expectedInitContainers: customInitContainers,
			},
			{
				description: "custom init container with DB check enabled",
				mmSpec: mmv1beta.MattermostSpec{
					Database: mmv1beta.Database{
						External:              &mmv1beta.ExternalDatabase{},
						DisableReadinessCheck: false,
					},
					PodExtensions: mmv1beta.PodExtensions{
						InitContainers: customInitContainers,
					},
				},
				dbConfig:               &ExternalDBConfig{dbType: database.PostgreSQLDatabase, secretName: "secret", hasDBCheckURL: true},
				expectedInitContainers: append(defaultExternalPostgresInitContainers, customInitContainers...),
			},
			{
				description: "empty init containers slice",
				mmSpec: mmv1beta.MattermostSpec{
					Database: mmv1beta.Database{
						External:              &mmv1beta.ExternalDatabase{},
						DisableReadinessCheck: true,
					},
					PodExtensions: mmv1beta.PodExtensions{
						InitContainers: []corev1.Container{},
					},
				},
				dbConfig:               &ExternalDBConfig{dbType: database.PostgreSQLDatabase, hasDBCheckURL: true},
				expectedInitContainers: nil,
			},
			{
				description: "empty init containers slice with DB check",
				mmSpec: mmv1beta.MattermostSpec{
					Database: mmv1beta.Database{
						External:              &mmv1beta.ExternalDatabase{},
						DisableReadinessCheck: false,
					},
					PodExtensions: mmv1beta.PodExtensions{
						InitContainers: []corev1.Container{},
					},
				},
				dbConfig:               &ExternalDBConfig{dbType: database.PostgreSQLDatabase, secretName: "secret", hasDBCheckURL: true},
				expectedInitContainers: defaultExternalPostgresInitContainers,
			},
			{
				description: "nil init containers slice",
				mmSpec: mmv1beta.MattermostSpec{
					Database: mmv1beta.Database{
						External: &mmv1beta.ExternalDatabase{},
					},
					PodExtensions: mmv1beta.PodExtensions{
						InitContainers: nil,
					},
				},
				dbConfig:               &ExternalDBConfig{dbType: database.PostgreSQLDatabase, hasDBCheckURL: false},
				expectedInitContainers: nil,
			},
		} {
			t.Run(testCase.description, func(t *testing.T) {
				mattermost := &mmv1beta.Mattermost{
					Spec: testCase.mmSpec,
				}
				deployment := GenerateDeploymentV1Beta(mattermost, testCase.dbConfig, &ExternalFileStore{}, "", "", "", "image")
				assert.Equal(t, testCase.expectedInitContainers, deployment.Spec.Template.Spec.InitContainers)
			})
		}
	})

	t.Run("custom sidecar container pod extension", func(t *testing.T) {
		dbConfig := &ExternalDBConfig{dbType: database.PostgreSQLDatabase, hasDBCheckURL: false}
		customSideBarContainers := []corev1.Container{
			{Image: "my-log-exporter-image", Name: "log-exporter"},
			{Image: "my-audit-image", Name: "audit"},
		}

		for _, testCase := range []struct {
			description               string
			mmSpec                    mmv1beta.MattermostSpec
			expectedSidecarContainers []corev1.Container
		}{
			{
				description: "no custom sidebar containers",
				mmSpec: mmv1beta.MattermostSpec{
					PodExtensions: mmv1beta.PodExtensions{
						SidecarContainers: nil,
					},
				},
				expectedSidecarContainers: nil,
			},
			{
				description: "custom sidebar containers",
				mmSpec: mmv1beta.MattermostSpec{
					PodExtensions: mmv1beta.PodExtensions{
						SidecarContainers: customSideBarContainers,
					},
				},
				expectedSidecarContainers: customSideBarContainers,
			},
		} {
			t.Run(testCase.description, func(t *testing.T) {
				mattermost := &mmv1beta.Mattermost{
					Spec: testCase.mmSpec,
				}
				deployment := GenerateDeploymentV1Beta(mattermost, dbConfig, &ExternalFileStore{}, "", "", "", "image")
				require.Equal(t, len(testCase.expectedSidecarContainers), len(deployment.Spec.Template.Spec.Containers)-1)
				if testCase.mmSpec.PodExtensions.SidecarContainers != nil {
					assert.Equal(t, testCase.expectedSidecarContainers, deployment.Spec.Template.Spec.Containers[1:])
				}
			})
		}
	})

	t.Run("should set SiteURL env if ingress host provided", func(t *testing.T) {
		mattermost := &mmv1beta.Mattermost{
			Spec: mmv1beta.MattermostSpec{},
		}
		dbCfg := &ExternalDBConfig{dbType: database.PostgreSQLDatabase, hasDBCheckURL: true}
		fileStoreCfg := &ExternalFileStore{}

		deployment := GenerateDeploymentV1Beta(mattermost, dbCfg, fileStoreCfg, "", "my-mattermost.com", "", "")
		mattermostAppContainer := mmv1beta.GetMattermostAppContainer(deployment.Spec.Template.Spec.Containers)

		assertEnvVarEqual(t, "MM_SERVICESETTINGS_SITEURL", "https://my-mattermost.com", mattermostAppContainer.Env)
	})
}

func TestGenerateRBACResources_V1Beta(t *testing.T) {
	roleName := "role"
	saName := "service-account"
	mattermost := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mm",
			Namespace: "test-namespace",
		},
	}

	serviceAccount := GenerateServiceAccountV1Beta(mattermost, saName)
	require.Equal(t, saName, serviceAccount.Name)
	require.Equal(t, mattermost.Namespace, serviceAccount.Namespace)
	require.Equal(t, 1, len(serviceAccount.OwnerReferences))

	role := GenerateRoleV1Beta(mattermost, roleName)
	require.Equal(t, roleName, role.Name)
	require.Equal(t, mattermost.Namespace, role.Namespace)
	require.Equal(t, 1, len(role.OwnerReferences))
	require.Equal(t, 1, len(role.Rules))

	roleBinding := GenerateRoleBindingV1Beta(mattermost, roleName, saName)
	require.Equal(t, roleName, roleBinding.Name)
	require.Equal(t, mattermost.Namespace, roleBinding.Namespace)
	require.Equal(t, 1, len(roleBinding.OwnerReferences))
	require.Equal(t, 1, len(roleBinding.Subjects))
	require.Equal(t, saName, roleBinding.Subjects[0].Name)
	require.Equal(t, roleName, roleBinding.RoleRef.Name)
}

func fixVolume() corev1.Volume {
	return corev1.Volume{
		Name: "test-volume",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: "mounted-secret",
			},
		},
	}
}

func fixVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "test-volume",
		MountPath: "/etc/test",
	}
}

func assertEnvVarEqual(t *testing.T, name, val string, env []corev1.EnvVar) {
	for _, e := range env {
		if e.Name == name {
			assert.Equal(t, e.Value, val)
			return
		}
	}

	assert.Fail(t, fmt.Sprintf("failed to find env var %s", name))
}
