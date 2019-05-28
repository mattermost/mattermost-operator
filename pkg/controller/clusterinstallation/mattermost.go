package clusterinstallation

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
)

func (r *ReconcileClusterInstallation) checkMattermost(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	err := r.checkMattermostService(mattermost, reqLogger)
	if err != nil {
		return err
	}

	err = r.checkMattermostIngress(mattermost, reqLogger)
	if err != nil {
		return err
	}

	return r.checkMattermostDeployment(mattermost, reqLogger)
}

func (r *ReconcileClusterInstallation) checkMattermostService(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	service := mattermost.GenerateService()

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

func (r *ReconcileClusterInstallation) checkMattermostIngress(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	ingress := mattermost.GenerateIngress()

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

func (r *ReconcileClusterInstallation) checkMattermostDeployment(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	var externalDB bool
	var dbUser, dbPassword string
	var err error
	if mattermost.Spec.DatabaseType.ExternalDatabaseSecret != "" {
		err = r.checkSecret(mattermost.Spec.DatabaseType.ExternalDatabaseSecret, mattermost.Namespace)
		if err != nil {
			return errors.Wrap(err, "Error getting the external database secret.")
		}
		externalDB = true
	} else {
		dbPassword, err = r.getMySQLSecrets(mattermost)
		if err != nil {
			return errors.Wrap(err, "Error getting mysql database password.")
		}
		dbUser = "root"
	}

	minioService, err := r.getMinioService(mattermost, reqLogger)
	if err != nil {
		return errors.Wrap(err, "Error getting the minio service.")
	}

	deployment := mattermost.GenerateDeployment(dbUser, dbPassword, externalDB, minioService)
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

	err = r.updateMattermostDeployment(mattermost, foundDeployment, reqLogger)
	if err != nil {
		reqLogger.Error(err, "Failed to update mattermost deployment")
		return err
	}

	return nil
}

// updateMattermostDeployment checks if the deployment should be updated.
// If an update is required then the deployment spec is set to:
// - roll forward version
// - keep active MattermostInstallation available by setting maxUnavailable=N-1
func (r *ReconcileClusterInstallation) updateMattermostDeployment(mi *mattermostv1alpha1.ClusterInstallation, d *appsv1.Deployment, reqLogger logr.Logger) error {
	var update bool

	// Ensure deployment replicas is the same as the spec
	if *d.Spec.Replicas != mi.Spec.Replicas {
		reqLogger.Info("Updating mattermost deployment replica count")
		d.Spec.Replicas = &mi.Spec.Replicas
		update = true
	}

	// Look for mattermost container in pod spec and determine if the image
	// needs to be updated.
	for pos, container := range d.Spec.Template.Spec.Containers {
		if container.Name == mi.Name {
			image := mi.GetImageName()
			if container.Image != image {
				reqLogger.Info("Updating mattermost deployment pod image")
				container.Image = image
				d.Spec.Template.Spec.Containers[pos] = container
				update = true
			}

			break
		}

		// If we got here, something went wrong
		return fmt.Errorf("Unable to find mattermost container in deployment")
	}

	updatedLabels := ensureLabels(mattermostv1alpha1.ClusterInstallationLabels(mi.Name), d.Labels)
	if !reflect.DeepEqual(updatedLabels, d.Labels) {
		reqLogger.Info("Updating mattermost deployment labels")
		update = true
		d.Labels = updatedLabels
	}

	if update {
		mu := intstr.FromInt(int(mi.Spec.Replicas - 1))
		if d.Spec.Strategy.RollingUpdate == nil {
			d.Spec.Strategy.RollingUpdate = &appsv1.RollingUpdateDeployment{}
		}
		d.Spec.Strategy.RollingUpdate.MaxUnavailable = &mu
		return r.client.Update(context.TODO(), d)
	}

	return nil
}
