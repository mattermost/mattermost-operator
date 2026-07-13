// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package mattermost

import (
	"fmt"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// LiteLLMLabels returns the label set for all LiteLLM resources.
func LiteLLMLabels() map[string]string {
	return map[string]string{"app": "mm-agent-litellm"}
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

// GenerateLiteLLMMasterKeySecret returns the Secret storing the LiteLLM master key.
func GenerateLiteLLMMasterKeySecret(namespace, masterKey string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMMasterKeySecretName,
			Namespace: namespace,
			Labels:    LiteLLMLabels(),
		},
		Data: map[string][]byte{"masterKey": []byte(masterKey)},
	}
}

// GenerateLiteLLMDeployment returns the Deployment for the LiteLLM gateway,
// configured from mattermost.Spec.Agents.LLMGateway (defaults applied by
// Mattermost.SetDefaults).
func GenerateLiteLLMDeployment(mattermost *mmv1beta.Mattermost) *appsv1.Deployment {
	replicas := int32(1)
	gateway := mattermost.Spec.Agents.LLMGateway

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

	liteLLMPullPolicy := corev1.PullIfNotPresent
	if imageTagNeedsAlwaysPull(gateway.Image) {
		liteLLMPullPolicy = corev1.PullAlways
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMDeploymentName,
			Namespace: mattermost.Namespace,
			Labels:    LiteLLMLabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: LiteLLMLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: LiteLLMLabels(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "litellm",
							Image:           gateway.Image,
							ImagePullPolicy: liteLLMPullPolicy,
							Env:             baseEnv,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: mmv1beta.AgentLiteLLMPort,
									Name:          "http",
								},
							},
							Resources:      gateway.Resources,
							LivenessProbe:  livenessProbe,
							ReadinessProbe: readinessProbe,
						},
					},
				},
			},
		},
	}
}

// GenerateLiteLLMService returns the ClusterIP Service for the LiteLLM gateway.
func GenerateLiteLLMService(mattermost *mmv1beta.Mattermost) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMServiceName,
			Namespace: mattermost.Namespace,
			Labels:    LiteLLMLabels(),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: LiteLLMLabels(),
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
