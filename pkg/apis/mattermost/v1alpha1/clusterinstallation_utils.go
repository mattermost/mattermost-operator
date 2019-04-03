package v1alpha1

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// OperatorName is the name of the Mattermost operator
	OperatorName = "mattermost-operator"
	// DefaultAmountOfPods is the default amount of Mattermost pods
	DefaultAmountOfPods = 2
	// DefaultMattermostImage is the default Mattermost docker image
	DefaultMattermostImage = "mattermost/mattermost-enterprise-edition:5.8.0"
	// DefaultMattermostDatabaseType is the default Mattermost database
	DefaultMattermostDatabaseType = "mysql"

	// ClusterLabel is the label applied across all compoments
	ClusterLabel = "v1alpha1.mattermost.com/installation"
)

// SetDefaults set the missing values in the manifest to the default ones
func (mattermost *ClusterInstallation) SetDefaults() error {
	if mattermost.Spec.IngressName == "" {
		return fmt.Errorf("need to set the IngressName")
	}

	if len(mattermost.Spec.Image) == 0 {
		mattermost.Spec.Image = DefaultMattermostImage
	}

	if mattermost.Spec.Replicas == 0 {
		mattermost.Spec.Replicas = DefaultAmountOfPods
	}

	if len(mattermost.Spec.DatabaseType.Type) == 0 {
		mattermost.Spec.DatabaseType.Type = DefaultMattermostDatabaseType
	}
	return nil
}

// GenerateService returns the service for Mattermost
func (mattermost *ClusterInstallation) GenerateService() *corev1.Service {
	mattermostPort := corev1.ServicePort{Port: 8065}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    map[string]string{ClusterLabel: mattermost.Name},
			Name:      mattermost.Name,
			Namespace: mattermost.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   SchemeGroupVersion.Group,
					Version: SchemeGroupVersion.Version,
					Kind:    "ClusterInstallation",
				}),
			},
			Annotations: map[string]string{
				"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{mattermostPort},
			Selector: map[string]string{
				ClusterLabel: mattermost.Name,
			},
			ClusterIP: corev1.ClusterIPNone,
		},
	}
}

// GenerateIngress returns the ingress for Mattermost
func (mattermost *ClusterInstallation) GenerateIngress() *v1beta1.Ingress {
	ingressName := mattermost.Name + "-ingress"
	spec := v1beta1.IngressSpec{}

	backend := v1beta1.IngressBackend{
		ServiceName: mattermost.Name,
		ServicePort: intstr.FromInt(8065),
	}
	rules := v1beta1.IngressRule{
		Host: mattermost.Spec.IngressName,
		IngressRuleValue: v1beta1.IngressRuleValue{
			HTTP: &v1beta1.HTTPIngressRuleValue{
				Paths: []v1beta1.HTTPIngressPath{
					{
						Path:    "/",
						Backend: backend,
					},
				},
			},
		},
	}
	spec.Rules = append(spec.Rules, rules)

	return &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressName,
			Namespace: mattermost.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   SchemeGroupVersion.Group,
					Version: SchemeGroupVersion.Version,
					Kind:    "ClusterInstallation",
				}),
			},
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "nginx",
				//"kubernetes.io/tls-acme":      "true",
			},
		},
		Spec: spec,
	}
}

// GenerateDeployment returns the deployment spec for Mattermost
func (mattermost *ClusterInstallation) GenerateDeployment(dbUser, dbPassword string) *appsv1.Deployment {
	datasourceMM := fmt.Sprintf("mysql://root:%s@tcp(%s-mysql.%s:3306)/mattermost?charset=utf8mb4,utf8&readTimeout=30s&writeTimeout=30s", dbPassword, mattermost.Name, mattermost.Namespace)

	initContainerCMD := fmt.Sprintf("until curl --max-time 5 http://%s-mysql.%s:3306; do echo waiting for mysql; sleep 5; done;", mattermost.Name, mattermost.Namespace)
	cmdInit := []string{"sh", "-c"}
	cmdInit = append(cmdInit, initContainerCMD)
	cmdStartMM := []string{"mattermost"}

	cmdInitDB := []string{"sh", "-c"}
	cmdInitDB = append(cmdInitDB, fmt.Sprintf("mysql -h %s-mysql.%s -u root -p%s -e 'CREATE DATABASE IF NOT EXISTS mattermost'", mattermost.Name, mattermost.Namespace, dbPassword))

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mattermost.Name,
			Namespace: mattermost.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   SchemeGroupVersion.Group,
					Version: SchemeGroupVersion.Version,
					Kind:    "ClusterInstallation",
				}),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &mattermost.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{ClusterLabel: mattermost.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{ClusterLabel: mattermost.Name},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Image:   "appropriate/curl:latest",
							Name:    "init-mysql",
							Command: cmdInit,
						},
						{
							Image:   "mysql:8.0.11",
							Name:    "init-mysql-database",
							Command: cmdInitDB,
						},
					},
					Containers: []corev1.Container{
						{
							Image:   "mattermost/mattermost-enterprise-edition:latest",
							Name:    mattermost.Name,
							Command: cmdStartMM,
							Env: []corev1.EnvVar{
								{
									Name:  "MM_CONFIG",
									Value: datasourceMM,
								},
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8065,
									Name:          mattermost.Name,
								},
							},
						},
					},
				},
			},
		},
	}
}
