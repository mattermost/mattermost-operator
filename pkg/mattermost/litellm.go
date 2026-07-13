// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package mattermost

import (
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// LiteLLMDBCredentialsHashAnnotation is stamped on the LiteLLM pod template
// with a hash of the database connection string, so credential rotation rolls
// the pods (the connection string is consumed via env-from-secret, which is
// fixed at pod start).
const LiteLLMDBCredentialsHashAnnotation = "mattermost.com/db-credentials-hash"

// LiteLLMSelectorLabels returns the stable labels used to select LiteLLM pods.
func LiteLLMSelectorLabels() map[string]string {
	return map[string]string{"app": mmv1beta.AgentLiteLLMDeploymentName}
}

// LiteLLMLabels returns all labels for LiteLLM resources owned by a Mattermost.
func LiteLLMLabels(mattermost *mmv1beta.Mattermost) map[string]string {
	labels := LiteLLMSelectorLabels()
	labels[mmv1beta.ClusterLabel] = mattermost.Name
	return labels
}

// GenerateLiteLLMMasterKeySecret returns the Secret storing the LiteLLM master key.
func GenerateLiteLLMMasterKeySecret(mattermost *mmv1beta.Mattermost, masterKey string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMMasterKeySecretName,
			Namespace: mattermost.Namespace,
			Labels:    LiteLLMLabels(mattermost),
		},
		Data: map[string][]byte{mmv1beta.SecretKeyMasterKey: []byte(masterKey)},
	}
}

// GenerateLiteLLMDeployment returns the Deployment for the LiteLLM gateway,
// configured from mattermost.Spec.Agents.LLMGateway (defaults applied by
// Mattermost.SetDefaults). dbCredentialsHash is a hash of the database
// connection string, stamped on the pod template so credential rotation
// triggers a rollout.
func GenerateLiteLLMDeployment(mattermost *mmv1beta.Mattermost, dbCredentialsHash string) *appsv1.Deployment {
	replicas := int32(1)
	gateway := mattermost.Spec.Agents.LLMGateway

	baseEnv := []corev1.EnvVar{
		{
			Name:      "DATABASE_URL",
			ValueFrom: EnvSourceFromSecret(mmv1beta.AgentLiteLLMDBCredentialsSecret, mmv1beta.SecretKeyConnectionString),
		},
		{
			Name:      "LITELLM_MASTER_KEY",
			ValueFrom: EnvSourceFromSecret(mmv1beta.AgentLiteLLMMasterKeySecretName, mmv1beta.SecretKeyMasterKey),
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
			Name:            mmv1beta.AgentLiteLLMDeploymentName,
			Namespace:       mattermost.Namespace,
			Labels:          LiteLLMLabels(mattermost),
			OwnerReferences: MattermostOwnerReference(mattermost),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: LiteLLMSelectorLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: LiteLLMLabels(mattermost),
					Annotations: map[string]string{
						LiteLLMDBCredentialsHashAnnotation: dbCredentialsHash,
					},
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
			Name:            mmv1beta.AgentLiteLLMServiceName,
			Namespace:       mattermost.Namespace,
			Labels:          LiteLLMLabels(mattermost),
			OwnerReferences: MattermostOwnerReference(mattermost),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: LiteLLMSelectorLabels(),
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
