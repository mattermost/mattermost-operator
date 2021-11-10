package mattermost

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"
)

func generalMattermostEnvVars(siteURL string) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  "MM_PLUGINSETTINGS_ENABLEUPLOADS",
			Value: "true",
		},
		{
			Name:  "MM_METRICSSETTINGS_ENABLE",
			Value: "true",
		},
		{
			Name:  "MM_METRICSSETTINGS_LISTENADDRESS",
			Value: ":8067",
		},
		{
			Name:  "MM_CLUSTERSETTINGS_ENABLE",
			Value: "true",
		},
		{
			Name:  "MM_CLUSTERSETTINGS_CLUSTERNAME",
			Value: "production",
		},
		{
			Name:  "MM_INSTALL_TYPE",
			Value: "kubernetes-operator",
		},
	}

	if siteURL != "" {
		envs = append(envs, corev1.EnvVar{
			Name:  "MM_SERVICESETTINGS_SITEURL",
			Value: siteURL,
		})
	}

	return envs
}

func fileStoreEnvVars(fileStore *FileStoreInfo) []corev1.EnvVar {
	minioAccessEnv := EnvSourceFromSecret(fileStore.secretName, fileStoreSecretAccessKey)
	minioSecretEnv := EnvSourceFromSecret(fileStore.secretName, fileStoreSecretSecretKey)

	return []corev1.EnvVar{
		{
			Name:  "MM_FILESETTINGS_DRIVERNAME",
			Value: "amazons3",
		},
		{
			Name:      "MM_FILESETTINGS_AMAZONS3ACCESSKEYID",
			ValueFrom: minioAccessEnv,
		},
		{
			Name:      "MM_FILESETTINGS_AMAZONS3SECRETACCESSKEY",
			ValueFrom: minioSecretEnv,
		},
		{
			Name:  "MM_FILESETTINGS_AMAZONS3BUCKET",
			Value: fileStore.bucketName,
		},
		{
			Name:  "MM_FILESETTINGS_AMAZONS3ENDPOINT",
			Value: fileStore.url,
		},
		{
			Name:  "MM_FILESETTINGS_AMAZONS3SSL",
			Value: strconv.FormatBool(fileStore.useS3SSL),
		},
	}
}

func elasticSearchEnvVars(host, user, password string) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "MM_ELASTICSEARCHSETTINGS_ENABLEINDEXING",
			Value: "true",
		},
		{
			Name:  "MM_ELASTICSEARCHSETTINGS_ENABLESEARCHING",
			Value: "true",
		},
		{
			Name:  "MM_ELASTICSEARCHSETTINGS_CONNECTIONURL",
			Value: host,
		},
		{
			Name:  "MM_ELASTICSEARCHSETTINGS_USERNAME",
			Value: user,
		},
		{
			Name:  "MM_ELASTICSEARCHSETTINGS_PASSWORD",
			Value: password,
		},
	}
}
