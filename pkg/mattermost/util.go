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

func FindContainer(name string, containers []corev1.Container) (int, bool) {
	for i, cont := range containers {
		if cont.Name == name {
			return i, true
		}
	}
	return -1, false
}

func RemoveContainer(name string, containers []corev1.Container) []corev1.Container {
	position, found := FindContainer(name, containers)
	if found {
		containers = append(containers[:position], containers[position+1:]...)
	}

	return containers
}
