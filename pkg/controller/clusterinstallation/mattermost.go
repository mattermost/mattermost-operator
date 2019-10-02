package clusterinstallation

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	k8sClient "sigs.k8s.io/controller-runtime/pkg/client"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
)

const updateName = "mattermost-update-check"

func (r *ReconcileClusterInstallation) checkMattermost(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	reqLogger = reqLogger.WithValues("Reconcile", "mattermost")

	err := r.checkMattermostService(mattermost, mattermost.Name, mattermost.GetProductionDeploymentName(), reqLogger)
	if err != nil {
		return err
	}

	if !mattermost.Spec.UseServiceLoadBalancer {
		ingressAnnotations := map[string]string{
			"kubernetes.io/ingress.class":                 "nginx",
			"nginx.ingress.kubernetes.io/proxy-body-size": "1000M",
		}
		for k, v := range mattermost.Spec.IngressAnnotations {
			ingressAnnotations[k] = v
		}

		err = r.checkMattermostIngress(mattermost, mattermost.Name, mattermost.Spec.IngressName, ingressAnnotations, reqLogger)
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

func (r *ReconcileClusterInstallation) checkMattermostIngress(mattermost *mattermostv1alpha1.ClusterInstallation, resourceName, ingressName string, ingressAnnotations map[string]string, reqLogger logr.Logger) error {
	desired := mattermost.GenerateIngress(resourceName, ingressName, ingressAnnotations)

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
	var dbData databaseInfo
	var err error
	if mattermost.Spec.Database.ExternalSecret != "" {
		err = r.checkSecret(mattermost.Spec.Database.ExternalSecret, "externalDB", mattermost.Namespace)
		if err != nil {
			return errors.Wrap(err, "Error getting the external database secret.")
		}
		externalDB = true
	} else {
		dbData, err = r.getOrCreateMySQLSecrets(mattermost, reqLogger)
		if err != nil {
			return errors.Wrap(err, "Error getting mysql database password.")
		}

		err = dbData.Valid()
		if err != nil {
			return err
		}
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

	desired := mattermost.GenerateDeployment(resourceName, ingressName, imageName, dbData.userName, dbData.userPassword, dbData.dbName, externalDB, isLicensed, minioService)
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

func (r *ReconcileClusterInstallation) launchUpdateJob(mi *mattermostv1alpha1.ClusterInstallation, new *appsv1.Deployment, imageName string, reqLogger logr.Logger) error {
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
		alreadyRunning, err := r.fetchRunningUpdateJob(mattermost, reqLogger)
		if err != nil && k8sErrors.IsNotFound(err) {
			reqLogger.Info("Launching update job")
			if err = r.launchUpdateJob(mattermost, desired, imageName, reqLogger); err != nil {
				return errors.Wrap(err, "Launching update job failed")
			}
			return errors.New("Began update job")
		}

		if err != nil {
			return errors.Wrap(err, "Error trying to determine if an update job already is running")
		}

		if alreadyRunning.Status.CompletionTime == nil {
			return errors.New("Update image job still running..")
		}

		// job is done, schedule cleanup
		defer func() {
			reqLogger.Info(fmt.Sprintf("Deleting job %s/%s",
				alreadyRunning.GetNamespace(), alreadyRunning.GetName()))

			err = r.client.Delete(context.TODO(), alreadyRunning)
			if err != nil {
				reqLogger.Error(err, "Unable to cleanup image update check job")
			}

			podList := &corev1.PodList{}
			listOptions := k8sClient.ListOptions{
				LabelSelector: labels.SelectorFromSet(
					labels.Set(map[string]string{"app": updateName})),
				Namespace: alreadyRunning.GetNamespace(),
			}

			err = r.client.List(context.Background(), &listOptions, podList)
			reqLogger.Info(fmt.Sprintf("Deleting %d pods", len(podList.Items)))
			for _, p := range podList.Items {
				reqLogger.Info(fmt.Sprintf("Deleting pod %s/%s", p.Namespace, p.Name))
				err = r.client.Delete(context.TODO(), &p)
				if err != nil {
					reqLogger.Error(err, fmt.Sprintf("Problem deleting pod %s/%s", p.Namespace, p.Name))
				}
			}
		}()

		// it's done, it either failed or succeded
		if alreadyRunning.Status.Failed > 0 {
			return errors.New("Upgrade job failed")
		}

		reqLogger.Info("Upgrade image job ran successfully")
	}

	return r.update(current, desired, reqLogger)
}

func (r *ReconcileClusterInstallation) fetchRunningUpdateJob(mi *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) (*batchv1.Job, error) {
	foundJob := &batchv1.Job{}
	err := r.client.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      updateName,
			Namespace: mi.GetNamespace(),
		},
		foundJob,
	)
	return foundJob, err
}
