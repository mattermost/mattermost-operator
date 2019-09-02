package clusterinstallation

import (
	"context"
	"fmt"
	"reflect"
	"time"

	objectMatcher "github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
)

func (r *ReconcileClusterInstallation) checkMattermost(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	reqLogger = reqLogger.WithValues("Reconcile", "mattermost")

	err := r.checkMattermostService(mattermost, mattermost.Name, mattermost.GetProductionDeploymentName(), reqLogger)
	if err != nil {
		return err
	}

	if !mattermost.Spec.UseServiceLoadBalancer {
		err = r.checkMattermostIngress(mattermost, mattermost.Name, mattermost.Spec.IngressName, reqLogger)
		if err != nil {
			return err
		}
	}

	if !mattermost.Spec.BlueGreen.Enable {
		err = r.checkMattermostDeployment(mattermost, mattermost.Name, mattermost.Spec.IngressName, mattermost.GetImageName(), reqLogger)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *ReconcileClusterInstallation) checkMattermostService(mattermost *mattermostv1alpha1.ClusterInstallation, resourceName, selectorName string, reqLogger logr.Logger) error {
	service := mattermost.GenerateService(resourceName, selectorName)

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

func (r *ReconcileClusterInstallation) checkMattermostIngress(mattermost *mattermostv1alpha1.ClusterInstallation, resourceName, ingressName string, reqLogger logr.Logger) error {
	ingress := mattermost.GenerateIngress(resourceName, ingressName)

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

func (r *ReconcileClusterInstallation) checkMattermostDeployment(mattermost *mattermostv1alpha1.ClusterInstallation, resourceName, ingressName, imageName string, reqLogger logr.Logger) error {
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

	deployment := mattermost.GenerateDeployment(resourceName, ingressName, imageName, dbUser, dbPassword, externalDB, isLicensed, minioService)
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

	err = r.updateMattermostDeployment(mattermost, deployment, foundDeployment, imageName, reqLogger)
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
func (r *ReconcileClusterInstallation) updateMattermostDeployment(mi *mattermostv1alpha1.ClusterInstallation, new, original *appsv1.Deployment, imageName string, reqLogger logr.Logger) error {
	var update bool

	// Look for mattermost container in pod spec and determine if the image
	// needs to be updated.
	for _, container := range original.Spec.Template.Spec.Containers {
		if container.Name == new.Spec.Template.Spec.Containers[0].Name {
			if container.Image != imageName {
				reqLogger.Info("Current image is not the same as the requested, will upgrade the Mattermost installation")
				update = true
			}
			break
		}
		// If we got here, something went wrong
		return errors.New("Unable to find mattermost container in deployment")
	}

	// Run a single-pod job with the new mattermost image to perform any
	// database migrations before altering the deployment. If this fails,
	// we will return and not upgrade the deployment.
	if update {
		reqLogger.Info(fmt.Sprintf("Running Mattermost image %s upgrade job check", imageName))

		updateName := "mattermost-update-check"
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      updateName,
				Namespace: mi.GetNamespace(),
			},
			Spec: batchv1.JobSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": updateName},
					},
					Spec: *new.Spec.Template.Spec.DeepCopy(),
				},
			},
		}

		// Override values for job-specific behavior.
		job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
		for i := range job.Spec.Template.Spec.Containers {
			job.Spec.Template.Spec.Containers[i].Command = []string{"mattermost", "version"}
		}

		err := r.client.Create(context.TODO(), job)
		if err != nil && !k8sErrors.IsAlreadyExists(err) {
			return err
		}
		defer func() {
			err = r.client.Delete(context.TODO(), job)
			if err != nil {
				reqLogger.Error(err, "Unable to cleanup image update check job")
			}
		}()

		// Wait up to 60 seconds for the update check job to return successfully.
		timer := time.NewTimer(60 * time.Second)
		defer timer.Stop()

	upgradeJobCheck:
		for {
			select {
			case <-timer.C:
				return errors.New("timed out waiting for mattermost image update check job to succeed")
			default:
				foundJob := &batchv1.Job{}
				err = r.client.Get(
					context.TODO(),
					types.NamespacedName{
						Name:      updateName,
						Namespace: mi.GetNamespace(),
					},
					foundJob,
				)
				if err != nil {
					continue
				}
				if foundJob.Status.Failed > 0 {
					return errors.New("Upgrade image job check failed")
				}
				if foundJob.Status.Succeeded > 0 {
					break upgradeJobCheck
				}

				time.Sleep(1 * time.Second)
			}
		}
	}

	reqLogger.Info("Upgrade image job ran successfully")

	patchResult, err := objectMatcher.DefaultPatchMaker.Calculate(original, new)
	if err != nil {
		return errors.Wrap(err, "error checking the difference in the deployment")
	}

	if !patchResult.IsEmpty() {
		err := objectMatcher.DefaultAnnotator.SetLastAppliedAnnotation(new)
		if err != nil {
			return errors.Wrap(err, "error applying the annotation in the deployment")
		}

		return r.client.Update(context.TODO(), new)
	}

	return nil
}
