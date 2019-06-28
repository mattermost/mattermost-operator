package mysql

import (
	"fmt"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	"github.com/mattermost/mattermost-operator/pkg/utils"

	mysqlOperator "github.com/presslabs/mysql-operator/pkg/apis/mysql/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	rbacv1beta1 "k8s.io/api/rbac/v1beta1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	MySQLServiceAccountName = "mysql-agent"
)

// Cluster returns the MySQL cluster to deploy
func Cluster(mattermost *mattermostv1alpha1.ClusterInstallation) *mysqlOperator.MysqlCluster {
	mysqlName := fmt.Sprintf("%s-mysql", mattermost.Name)

	return &mysqlOperator.MysqlCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mysqlName,
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
			Replicas:     utils.NewInt32(4),
			SecretName:   fmt.Sprintf("%s-mysql-root-password", mattermost.Name),
			VolumeSpec: mysqlOperator.VolumeSpec{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						"ReadWriteOnce",
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse(mattermost.Spec.DatabaseType.DbStorageSize),
						},
					},
				},
			},
		},
	}
}

// ServiceAccount returns the service account used by the MySQL Operator
func ServiceAccount(mattermost *mattermostv1alpha1.ClusterInstallation) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MySQLServiceAccountName,
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
	}
}

// RoleBinding returns the role binding used by the MySQL Operator
func RoleBinding(mattermost *mattermostv1alpha1.ClusterInstallation) *rbacv1beta1.RoleBinding {
	return &rbacv1beta1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MySQLServiceAccountName,
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
		Subjects: []rbacv1beta1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      MySQLServiceAccountName,
				Namespace: mattermost.Namespace,
			},
		},
		RoleRef: rbacv1beta1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     MySQLServiceAccountName,
		},
	}
}
