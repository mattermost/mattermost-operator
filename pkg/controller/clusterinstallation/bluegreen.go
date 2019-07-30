package clusterinstallation

import (
	"context"

	// "fmt"
	"reflect"
	// "time"

	// objectMatcher "github.com/banzaicloud/k8s-objectmatcher/patch"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"

	// batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"

	// k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
)

func (r *ReconcileClusterInstallation) checkBlueGreen(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	reqLogger = reqLogger.WithValues("Reconcile", "mattermost")

	err := r.checkBlueGreenService(mattermost, reqLogger)
	if err != nil {
		return err
	}

	err = r.checkBlueGreenIngress(mattermost, reqLogger)
	if err != nil {
		return err
	}

	return r.checkBlueGreenDeployment(mattermost, reqLogger)
}

func (r *ReconcileClusterInstallation) checkBlueGreenService(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	service := mattermost.GenerateBlueGreenService()

	err := r.createServiceIfNotExists(mattermost, service, reqLogger)
	if err != nil {
		return err
	}

	foundService := &corev1.Service{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, foundService)
	if err != nil {
		return err
	}

	var update bool

	updatedLabels := ensureLabels(service.Labels, foundService.Labels)
	if !reflect.DeepEqual(updatedLabels, foundService.Labels) {
		reqLogger.Info("Updating mattermost service labels")
		foundService.Labels = updatedLabels
		update = true
	}

	if !reflect.DeepEqual(service.Annotations, foundService.Annotations) {
		reqLogger.Info("Updating mattermost service annotations")
		foundService.Annotations = service.Annotations
		update = true
	}

	// If we are using the loadBalancer the ClusterIp is immutable
	// and other fields are created in the first time
	if mattermost.Spec.UseServiceLoadBalancer {
		service.Spec.ClusterIP = foundService.Spec.ClusterIP
		service.Spec.ExternalTrafficPolicy = foundService.Spec.ExternalTrafficPolicy
		service.Spec.SessionAffinity = foundService.Spec.SessionAffinity
		for _, foundPort := range foundService.Spec.Ports {
			for i, servicePort := range service.Spec.Ports {
				if foundPort.Name == servicePort.Name {
					service.Spec.Ports[i].NodePort = foundPort.NodePort
					service.Spec.Ports[i].Protocol = foundPort.Protocol
				}
			}
		}
	}
	if !reflect.DeepEqual(service.Spec, foundService.Spec) {
		reqLogger.Info("Updating mattermost service spec")
		foundService.Spec = service.Spec
		update = true
	}

	if update {
		return r.client.Update(context.TODO(), foundService)
	}

	return nil
}

func (r *ReconcileClusterInstallation) checkBlueGreenIngress(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	ingress := mattermost.GenerateBlueGreenIngress()

	err := r.createIngressIfNotExists(mattermost, ingress, reqLogger)
	if err != nil {
		return err
	}

	foundIngress := &v1beta1.Ingress{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: ingress.Name, Namespace: ingress.Namespace}, foundIngress)
	if err != nil {
		return err
	}

	var update bool

	updatedLabels := ensureLabels(ingress.Labels, foundIngress.Labels)
	if !reflect.DeepEqual(updatedLabels, foundIngress.Labels) {
		reqLogger.Info("Updating mattermost ingress labels")
		foundIngress.Labels = updatedLabels
		update = true
	}

	if !reflect.DeepEqual(ingress.Annotations, foundIngress.Annotations) {
		reqLogger.Info("Updating mattermost ingress annotations")
		foundIngress.Annotations = ingress.Annotations
		update = true
	}

	if !reflect.DeepEqual(ingress.Spec, foundIngress.Spec) {
		reqLogger.Info("Updating mattermost ingress spec")
		foundIngress.Spec = ingress.Spec
		update = true
	}

	if update {
		return r.client.Update(context.TODO(), foundIngress)
	}

	return nil
}

func (r *ReconcileClusterInstallation) checkBlueGreenDeployment(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	var externalDB, isLicensed bool
	var dbUser, dbPassword string
	var err error
	if mattermost.Spec.Database.ExternalSecret != "" {
		err = r.checkSecret(mattermost.Spec.Database.ExternalSecret, "externalDB", mattermost.Namespace)
		if err != nil {
			return errors.Wrap(err, "Error getting the external database secret.")
		}
		externalDB = true
	} else {
		dbPassword, err = r.getOrCreateMySQLSecrets(mattermost, reqLogger)
		if err != nil {
			return errors.Wrap(err, "Error getting mysql database password.")
		}
		dbUser = "mmuser"
	}

	minioService, err := r.getMinioService(mattermost, reqLogger)
	if err != nil {
		return errors.Wrap(err, "Error getting the minio service.")
	}

	if mattermost.Spec.MattermostLicenseSecret != "" {
		err = r.checkSecret(mattermost.Spec.MattermostLicenseSecret, "license", mattermost.Namespace)
		if err != nil {
			return errors.Wrap(err, "Error getting the mattermost license secret.")
		}
		isLicensed = true
	}

	deployment := mattermost.GenerateBlueGreenDeployment(dbUser, dbPassword, externalDB, isLicensed, minioService)
	err = r.createDeploymentIfNotExists(mattermost, deployment, reqLogger)
	if err != nil {
		return err
	}

	foundDeployment := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, foundDeployment)
	if err != nil {
		reqLogger.Error(err, "Failed to get mattermost deployment")
		return err
	}

	return nil
}
