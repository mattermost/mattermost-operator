package mysql

import (
	"fmt"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"

	mysqlOperator "github.com/oracle/mysql-operator/pkg/apis/mysql/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	rbacv1beta1 "k8s.io/api/rbac/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	MySQLServiceAccountName = "mysql-agent"
)

// Cluster returns the MySQL component to deploy
func Cluster(mattermost *mattermostv1alpha1.ClusterInstallation) *mysqlOperator.Cluster {
	mySQLName := fmt.Sprintf("%s-mysql", mattermost.Name)

	mySQLCluster := &mysqlOperator.Cluster{}
	mySQLCluster.SetName(mySQLName)
	mySQLCluster.SetNamespace(mattermost.Namespace)
	mySQLCluster.Spec.Members = 2
	mySQLCluster.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
		*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
			Group:   mattermostv1alpha1.SchemeGroupVersion.Group,
			Version: mattermostv1alpha1.SchemeGroupVersion.Version,
			Kind:    "ClusterInstallation",
		}),
	}

	return mySQLCluster
}

// ServiceAccount returns the service account used by the MySQL Operator
func ServiceAccount(mattermost *mattermostv1alpha1.ClusterInstallation) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    map[string]string{mattermostv1alpha1.ClusterLabel: mattermost.Name},
			Name:      MySQLServiceAccountName,
			Namespace: mattermost.Namespace,
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

// RoleBinding returns the service account used by the MySQL Operator
func RoleBinding(mattermost *mattermostv1alpha1.ClusterInstallation) *rbacv1beta1.RoleBinding {
	return &rbacv1beta1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    map[string]string{mattermostv1alpha1.ClusterLabel: mattermost.Name},
			Name:      MySQLServiceAccountName,
			Namespace: mattermost.Namespace,
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
