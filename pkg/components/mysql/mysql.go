package mysql

import (
	"fmt"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	"github.com/mattermost/mattermost-operator/pkg/utils"
	mysqlOperator "github.com/presslabs/mysql-operator/pkg/apis/mysql/v1alpha1"

	componentUtils "github.com/mattermost/mattermost-operator/pkg/components/utils"

	corev1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Cluster returns the MySQL cluster to deploy
func Cluster(mattermost *mattermostv1alpha1.ClusterInstallation) *mysqlOperator.MysqlCluster {
	mysql := &mysqlOperator.MysqlCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentUtils.HashWithPrefix("db", mattermost.Name),
			Namespace: mattermost.Namespace,
			Labels:    mattermostv1alpha1.ClusterInstallationResourceLabels(mattermost.Name),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   mattermostv1alpha1.SchemeGroupVersion.Group,
					Version: mattermostv1alpha1.SchemeGroupVersion.Version,
					Kind:    "ClusterInstallation",
				}),
			},
		},
		Spec: mysqlOperator.MysqlClusterSpec{
			MysqlVersion: "5.7",
			Replicas:     utils.NewInt32(mattermost.Spec.Database.Replicas),
			SecretName:   DefaultDatabaseSecretName(mattermost.Name),
			VolumeSpec: mysqlOperator.VolumeSpec{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						"ReadWriteOnce",
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse(mattermost.Spec.Database.StorageSize),
						},
					},
				},
			},
			BackupSchedule:           mattermost.Spec.Database.BackupSchedule,
			BackupURL:                mattermost.Spec.Database.BackupURL,
			BackupSecretName:         mattermost.Spec.Database.BackupRestoreSecretName,
			BackupRemoteDeletePolicy: mysqlOperator.DeletePolicy(mattermost.Spec.Database.BackupRemoteDeletePolicy),
		},
	}

	if mattermost.Spec.Database.InitBucketURL != "" && mattermost.Spec.Database.BackupRestoreSecretName != "" {
		mysql.Spec.InitBucketURL = mattermost.Spec.Database.InitBucketURL
		mysql.Spec.InitBucketSecretName = mattermost.Spec.Database.BackupRestoreSecretName
	}

	if mattermost.Spec.Database.Secret != "" {
		mysql.Spec.SecretName = mattermost.Spec.Database.Secret
	}

	return mysql
}

// DefaultDatabaseSecretName returns the default database secret name based on
// the provided installation name.
func DefaultDatabaseSecretName(installationName string) string {
	return fmt.Sprintf("%s-mysql-root-password", installationName)
}
