// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package mattermost

import (
	"fmt"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// liteLLMLabels returns the label set for all LiteLLM resources.
func liteLLMLabels() map[string]string {
	return map[string]string{"app": "litellm"}
}

// LiteLLMServiceURL returns the in-cluster base URL for the LiteLLM service.
func LiteLLMServiceURL(namespace string) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
		mmv1beta.AgentLiteLLMServiceName, namespace, mmv1beta.AgentLiteLLMPort)
}

// secretEnvSource returns an EnvVarSource that reads from a Secret key.
func secretEnvSource(secretName, key string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
			Key:                  key,
		},
	}
}

// GenerateLiteLLMConfigMap returns the ConfigMap for LiteLLM general settings.
// It contains only general_settings — all models are registered via API.
// This resource is NOT owned by any single Agent (it is shared), so the caller
// must NOT use r.Resources.Create (which sets OwnerReference). Use r.client.Create directly.
func GenerateLiteLLMConfigMap(namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMConfigMapName,
			Namespace: namespace,
			Labels:    liteLLMLabels(),
		},
		Data: map[string]string{
			"config.yaml": "general_settings:\n  store_model_in_db: true\n",
		},
	}
}

// GenerateLiteLLMDeployment returns the Deployment for the LiteLLM gateway.
// This resource is NOT owned by any single Agent. The caller must NOT use
// r.Resources.Create — use r.client.Create directly.
func GenerateLiteLLMDeployment(namespace, image string) *appsv1.Deployment {
	replicas := int32(1)
	configVolumeName := "litellm-config"

	baseEnv := []corev1.EnvVar{
		{
			Name:      "DATABASE_URL",
			ValueFrom: secretEnvSource(mmv1beta.AgentLiteLLMDBCredentialsSecret, "connectionString"),
		},
		{
			Name:      "LITELLM_MASTER_KEY",
			ValueFrom: secretEnvSource(mmv1beta.AgentLiteLLMMasterKeySecretName, "masterKey"),
		},
		{
			Name:  "STORE_MODEL_IN_DB",
			Value: "True",
		},
	}
	livenessProbe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/health/liveliness",
				Port: intstr.FromInt32(mmv1beta.AgentLiteLLMPort),
			},
		},
		InitialDelaySeconds: 15,
		PeriodSeconds:       10,
		FailureThreshold:    3,
	}

	readinessProbe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/health/readiness",
				Port: intstr.FromInt32(mmv1beta.AgentLiteLLMPort),
			},
		},
		InitialDelaySeconds: 15,
		PeriodSeconds:       5,
		FailureThreshold:    6,
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMDeploymentName,
			Namespace: namespace,
			Labels:    liteLLMLabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: liteLLMLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: liteLLMLabels(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "litellm",
							Image: image,
							Args:  []string{"--config", "/app/config/config.yaml"},
							Env:   baseEnv,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: mmv1beta.AgentLiteLLMPort,
									Name:          "http",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
							},
							LivenessProbe:  livenessProbe,
							ReadinessProbe: readinessProbe,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      configVolumeName,
									MountPath: "/app/config",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: configVolumeName,
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: mmv1beta.AgentLiteLLMConfigMapName,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// GenerateLiteLLMService returns the ClusterIP Service for the LiteLLM gateway.
// This resource is NOT owned by any single Agent. The caller must NOT use
// r.Resources.Create — use r.client.Create directly.
func GenerateLiteLLMService(namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMServiceName,
			Namespace: namespace,
			Labels:    liteLLMLabels(),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: liteLLMLabels(),
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       mmv1beta.AgentLiteLLMPort,
					TargetPort: intstr.FromInt32(mmv1beta.AgentLiteLLMPort),
				},
			},
		},
	}
}

// GenerateAgentLiteLLMKeySecret returns the Secret storing an agent's LiteLLM virtual key.
// Follows the same pattern as GenerateAgentBotTokenSecret.
func GenerateAgentLiteLLMKeySecret(agent *mmv1beta.Agent, keyValue string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            agent.LiteLLMKeySecretName(),
			Namespace:       agent.Namespace,
			Labels:          mmv1beta.AgentLabels(agent.Name),
			OwnerReferences: AgentOwnerReference(agent),
		},
		Data: map[string][]byte{"apiKey": []byte(keyValue)},
	}
}
