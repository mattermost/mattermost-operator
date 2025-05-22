package mattermost

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// mergeStringMaps inserts (and overwrites) data into receiver map object from
// origin.
func mergeStringMaps(receiver, origin map[string]string) map[string]string {
	if receiver == nil {
		receiver = make(map[string]string)
	}

	if origin == nil {
		return receiver
	}

	for key := range origin {
		receiver[key] = origin[key]
	}

	return receiver
}

// mergeEnvVars takes two sets of env vars and merges them together. This will
// replace env vars that already existed or will append them if they are new.
func mergeEnvVars(original, new []corev1.EnvVar) []corev1.EnvVar {
	for _, newEnvVar := range new {
		var replaced bool

		for originalPos, originalEnvVar := range original {
			if originalEnvVar.Name == newEnvVar.Name {
				original[originalPos] = newEnvVar
				replaced = true
			}
		}

		if !replaced {
			original = append(original, newEnvVar)
		}
	}

	return original
}

func determineMaxBodySize(ingressAnnotations map[string]string, defaultSize string) string {
	size, ok := ingressAnnotations["nginx.ingress.kubernetes.io/proxy-body-size"]
	if !ok {
		return defaultSize
	}

	sizeUnit := size[len(size)-1]
	maxFileSize, _ := strconv.Atoi(size[:len(size)-1])

	switch sizeUnit {
	case 'M', 'm':
		return strconv.Itoa(maxFileSize * sizeMB)
	case 'G', 'g':
		return strconv.Itoa(maxFileSize * sizeGB)
	}

	return defaultSize
}

func setProbes(customLiveness, customReadiness corev1.Probe) (*corev1.Probe, *corev1.Probe) {
	liveness := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/api/v4/system/ping",
				Port: intstr.FromInt(8065),
			},
		},
		InitialDelaySeconds: 10,
		PeriodSeconds:       10,
		FailureThreshold:    3,
	}

	if customLiveness.ProbeHandler != (corev1.ProbeHandler{}) {
		liveness.ProbeHandler = customLiveness.ProbeHandler
	}

	if customLiveness.InitialDelaySeconds != 0 {
		liveness.InitialDelaySeconds = customLiveness.InitialDelaySeconds
	}

	if customLiveness.PeriodSeconds != 0 {
		liveness.PeriodSeconds = customLiveness.PeriodSeconds
	}

	if customLiveness.TimeoutSeconds != 0 {
		liveness.TimeoutSeconds = customLiveness.TimeoutSeconds
	}

	if customLiveness.FailureThreshold != 0 {
		liveness.FailureThreshold = customLiveness.FailureThreshold
	}

	if customLiveness.SuccessThreshold != 0 {
		liveness.SuccessThreshold = customLiveness.SuccessThreshold
	}

	readiness := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/api/v4/system/ping",
				Port: intstr.FromInt(8065),
			},
		},
		InitialDelaySeconds: 10,
		PeriodSeconds:       5,
		FailureThreshold:    6,
	}

	if customReadiness.ProbeHandler != (corev1.ProbeHandler{}) {
		readiness.ProbeHandler = customReadiness.ProbeHandler
	}

	if customReadiness.InitialDelaySeconds != 0 {
		readiness.InitialDelaySeconds = customReadiness.InitialDelaySeconds
	}

	if customReadiness.PeriodSeconds != 0 {
		readiness.PeriodSeconds = customReadiness.PeriodSeconds
	}

	if customReadiness.TimeoutSeconds != 0 {
		readiness.TimeoutSeconds = customReadiness.TimeoutSeconds
	}

	if customReadiness.FailureThreshold != 0 {
		readiness.FailureThreshold = customReadiness.FailureThreshold
	}

	if customReadiness.SuccessThreshold != 0 {
		readiness.SuccessThreshold = customReadiness.SuccessThreshold
	}

	return liveness, readiness
}

func siteURLFromHost(ingressHost string) string {
	if ingressHost != "" {
		return fmt.Sprintf("https://%s", ingressHost)
	}
	return ""
}
