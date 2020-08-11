package mattermost

import (
	"regexp"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"

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

func setProbes(customLiveness, customStartup, customReadiness corev1.Probe) (*corev1.Probe, *corev1.Probe, *corev1.Probe) {
	liveness := &corev1.Probe{
		Handler: corev1.Handler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/api/v4/system/ping",
				Port: intstr.FromInt(8065),
			},
		},
		InitialDelaySeconds: 10,
		PeriodSeconds:       10,
		FailureThreshold:    3,
	}

	if customLiveness.Handler != (corev1.Handler{}) {
		liveness.Handler = customLiveness.Handler
	}

	if customLiveness.InitialDelaySeconds != 0 {
		liveness.InitialDelaySeconds = customLiveness.InitialDelaySeconds
	}

	if customLiveness.PeriodSeconds != 0 {
		liveness.PeriodSeconds = customLiveness.PeriodSeconds
	}

	if customLiveness.FailureThreshold != 0 {
		liveness.FailureThreshold = customLiveness.FailureThreshold
	}

	if customLiveness.SuccessThreshold != 0 {
		liveness.SuccessThreshold = customLiveness.SuccessThreshold
	}

	startUp := &corev1.Probe{
		Handler: corev1.Handler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/api/v4/system/ping",
				Port: intstr.FromInt(8065),
			},
		},
		InitialDelaySeconds: 1,
		PeriodSeconds:       10,
		FailureThreshold:    60,
	}

	if customStartup.Handler != (corev1.Handler{}) {
		startUp.Handler = customStartup.Handler
	}

	if customStartup.InitialDelaySeconds != 0 {
		startUp.InitialDelaySeconds = customStartup.InitialDelaySeconds
	}

	if customStartup.PeriodSeconds != 0 {
		startUp.PeriodSeconds = customStartup.PeriodSeconds
	}

	if customStartup.FailureThreshold != 0 {
		startUp.FailureThreshold = customStartup.FailureThreshold
	}

	if customStartup.SuccessThreshold != 0 {
		startUp.SuccessThreshold = customStartup.SuccessThreshold
	}

	readiness := &corev1.Probe{
		Handler: corev1.Handler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/api/v4/system/ping",
				Port: intstr.FromInt(8065),
			},
		},
		InitialDelaySeconds: 10,
		PeriodSeconds:       5,
		FailureThreshold:    6,
	}

	if customReadiness.Handler != (corev1.Handler{}) {
		readiness.Handler = customReadiness.Handler
	}

	if customReadiness.InitialDelaySeconds != 0 {
		readiness.InitialDelaySeconds = customReadiness.InitialDelaySeconds
	}

	if customReadiness.PeriodSeconds != 0 {
		readiness.PeriodSeconds = customReadiness.PeriodSeconds
	}

	if customReadiness.FailureThreshold != 0 {
		readiness.FailureThreshold = customReadiness.FailureThreshold
	}

	if customReadiness.SuccessThreshold != 0 {
		readiness.SuccessThreshold = customReadiness.SuccessThreshold
	}

	return liveness, startUp, readiness
}

func checkMattermostImageVersion(imageName, version string) error {
	var re = regexp.MustCompile(`mattermost/mattermost-.*[team|enterprise]-edition$`)
	if !re.MatchString(imageName) {
		return errors.Errorf("not using the mattermost official images so cannot validate the version")
	}

	v, err := semver.Parse(version)
	if err != nil {
		return errors.Wrap(err, "failed to parse the version, maybe it is using ")
	}

	expectedRange, err := semver.ParseRange(">=5.26.0")
	if err != nil {
		return errors.Wrapf(err, "failed to parse the version range for %s", version)
	}
	if !expectedRange(v) {
		return errors.Errorf("invalid Version option %s, must be greater than 5.26.0", version)
	}

	return nil
}
