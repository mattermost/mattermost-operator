package clusterinstallation

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
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
	desired := mattermost.GenerateService(resourceName, selectorName)

	err := r.createServiceIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return err
	}

	current := &corev1.Service{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return err
	}

	return r.update(current, desired, reqLogger)
}

func (r *ReconcileClusterInstallation) checkMattermostIngress(mattermost *mattermostv1alpha1.ClusterInstallation, resourceName, ingressName string, reqLogger logr.Logger) error {
	desired := mattermost.GenerateIngress(resourceName, ingressName)

	err := r.createIngressIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return err
	}

	current := &v1beta1.Ingress{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return err
	}

	return r.update(current, desired, reqLogger)
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

	desired := mattermost.GenerateDeployment(resourceName, ingressName, imageName, dbUser, dbPassword, externalDB, isLicensed, minioService)
	err = r.createDeploymentIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return err
	}

	current := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		reqLogger.Error(err, "Failed to get mattermost deployment")
		return err
	}

	err = r.updateMattermostDeployment(mattermost, current, desired, imageName, reqLogger)
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
func (r *ReconcileClusterInstallation) updateMattermostDeployment(mattermost *mattermostv1alpha1.ClusterInstallation, current, desired *appsv1.Deployment, imageName string, reqLogger logr.Logger) error {
	var update bool

	// Look for mattermost container in pod spec and determine if the image
	// needs to be updated.
	for _, container := range current.Spec.Template.Spec.Containers {
		if container.Name == desired.Spec.Template.Spec.Containers[0].Name {
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
				Namespace: mattermost.GetNamespace(),
			},
			Spec: batchv1.JobSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": updateName},
					},
					Spec: *desired.Spec.Template.Spec.DeepCopy(),
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
						Namespace: mattermost.GetNamespace(),
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
		reqLogger.Info("Upgrade image job ran successfully")
	}
	return r.update(current, desired, reqLogger)
}
