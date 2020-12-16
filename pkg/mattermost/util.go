package mattermost

import corev1 "k8s.io/api/core/v1"

func EnvSourceFromSecret(secretName, key string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: secretName,
			},
			Key: key,
		},
	}
}
