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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sClient "sigs.k8s.io/controller-runtime/pkg/client"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
)

const updateJobName = "mattermost-update-check"

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
	desired := mattermost.GenerateIngress(resourceName, ingressName, ingressAnnotations, mattermost.Spec.UseIngressTLS)

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
	var err error
	dbInfo := &databaseInfo{}

	if mattermost.Spec.Database.Secret == "" {
		dbInfo, err = r.getOrCreateMySQLSecrets(mattermost, reqLogger)
		if err != nil {
			return errors.Wrap(err, "unable to get database information")
		}
	} else {
		foundSecret := &corev1.Secret{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: mattermost.Spec.Database.Secret, Namespace: mattermost.Namespace}, foundSecret)
		if err != nil {
			return errors.Wrap(err, "error getting database secret")
		}

		if _, ok := foundSecret.Data["DB_CONNECTION_STRING"]; ok {
			externalDB = true
		}

		if !externalDB {
			dbInfo = getDatabaseInfoFromSecret(foundSecret)
			err = dbInfo.IsValid()
			if err != nil {
				return errors.Wrap(err, "database secret is not valid")
			}
		}
	}

	var minioURL string
	if mattermost.Spec.Minio.IsExternal() {
		minioURL = mattermost.Spec.Minio.ExternalURL
	} else {
		minioURL, err = r.getMinioService(mattermost, reqLogger)
		if err != nil {
			return errors.Wrap(err, "Error getting the minio service.")
		}
	}

	if mattermost.Spec.MattermostLicenseSecret != "" {
		err = r.checkSecret(mattermost.Spec.MattermostLicenseSecret, "license", mattermost.Namespace)
		if err != nil {
			return errors.Wrap(err, "Error getting the mattermost license secret.")
		}
		isLicensed = true
	}

	desired := mattermost.GenerateDeployment(resourceName, ingressName, imageName, dbInfo.userName, dbInfo.userPassword, dbInfo.dbName, externalDB, isLicensed, minioURL)
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
func (r *ReconcileClusterInstallation) deleteAllMattermostComponents(mattermost *mattermostv1alpha1.ClusterInstallation, resourceName string, reqLogger logr.Logger) error {
	err := r.deleteMattermostDeployment(mattermost, resourceName, reqLogger)
	if err != nil {
		return err
	}

	err = r.deleteMattermostService(mattermost, resourceName, reqLogger)
	if err != nil {
		return err
	}

	err = r.deleteMattermostIngress(mattermost, resourceName, reqLogger)
	if err != nil {
		return err
	}

	return nil
}

func (r *ReconcileClusterInstallation) deleteMattermostDeployment(mattermost *mattermostv1alpha1.ClusterInstallation, resourceName string, reqLogger logr.Logger) error {
	return r.deleteMattermostResource(mattermost, resourceName, &appsv1.Deployment{}, reqLogger)
}

func (r *ReconcileClusterInstallation) deleteMattermostService(mattermost *mattermostv1alpha1.ClusterInstallation, resourceName string, reqLogger logr.Logger) error {
	return r.deleteMattermostResource(mattermost, resourceName, &corev1.Service{}, reqLogger)
}

func (r *ReconcileClusterInstallation) deleteMattermostIngress(mattermost *mattermostv1alpha1.ClusterInstallation, resourceName string, reqLogger logr.Logger) error {
	return r.deleteMattermostResource(mattermost, resourceName, &v1beta1.Ingress{}, reqLogger)
}

func (r *ReconcileClusterInstallation) deleteMattermostResource(mattermost *mattermostv1alpha1.ClusterInstallation, resourceName string, resource runtime.Object, reqLogger logr.Logger) error {
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: resourceName, Namespace: mattermost.GetNamespace()}, resource)
	if err != nil && k8sErrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if mattermost resource exists")
		return err
	}

	err = r.client.Delete(context.TODO(), resource)
	if err != nil {
		reqLogger.Error(err, "Failed to delete mattermost resource")
		return err
	}

	return nil
}

func (r *ReconcileClusterInstallation) launchUpdateJob(
	mi *mattermostv1alpha1.ClusterInstallation,
	deployment *appsv1.Deployment,
) error {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      updateJobName,
			Namespace: mi.GetNamespace(),
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": updateJobName},
				},
				Spec: *deployment.Spec.Template.Spec.DeepCopy(),
			},
		},
	}

	// We dont need to validate the readiness/liveness for this short live job
	// if the job fails it will get another later.
	for i := range job.Spec.Template.Spec.Containers {
		job.Spec.Template.Spec.Containers[i].LivenessProbe = nil
		job.Spec.Template.Spec.Containers[i].ReadinessProbe = nil
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

// isMainContainerImageSame checks whether main containers of specified deployments are the same or not.
func (r *ReconcileClusterInstallation) isMainContainerImageSame(
	mattermost *mattermostv1alpha1.ClusterInstallation,
	a *appsv1.Deployment,
	b *appsv1.Deployment,
) (bool, error) {
	// Sanity check
	if (a == nil) || (b == nil) {
		return false, errors.New("Unable to find main container, no deployment provided")
	}

	// Fetch containers to compare

	containerA := mattermost.GetMainContainer(a)
	if containerA == nil {
		return false, errors.Errorf("Unable to find main container, incorrect deployment %s/%s", a.Namespace, a.Name)
	}

	containerB := mattermost.GetMainContainer(b)
	if containerB == nil {
		return false, errors.Errorf("Unable to find main container, incorrect deployment %s/%s", b.Namespace, b.Name)
	}

	// Both containers fetched, can compare images

	return containerA.Image == containerB.Image, nil
}

// updateMattermostDeployment performs deployment update if necessary.
// If a deployment update is necessary, an update job is launched to check new image.
func (r *ReconcileClusterInstallation) updateMattermostDeployment(
	mattermost *mattermostv1alpha1.ClusterInstallation,
	current *appsv1.Deployment,
	desired *appsv1.Deployment,
	imageName string,
	reqLogger logr.Logger,
) error {
	sameImage, err := r.isMainContainerImageSame(mattermost, current, desired)
	if err != nil {
		return err
	}

	if sameImage {
		// Need to update other fields only, update job is not required
		return r.update(current, desired, reqLogger)
	}

	// Image is not the same
	// Run a single-pod job with the new mattermost image
	// It will check whether new image is operational
	// and may perform any database migrations before altering the deployment.
	// If this fails, we will return and not upgrade the deployment.

	reqLogger.Info("Current image is not the same as the requested, will upgrade the Mattermost installation")

	job, err := r.checkUpdateJob(mattermost, desired, reqLogger)
	if job != nil {
		// Job is done, need to cleanup
		defer r.cleanupUpdateJob(job, reqLogger)
	}
	if err != nil {
		return err
	}

	// Job completed successfully

	return r.update(current, desired, reqLogger)
}

// checkUpdateJob checks whether update job status. In case job is not running it is launched
func (r *ReconcileClusterInstallation) checkUpdateJob(
	mattermost *mattermostv1alpha1.ClusterInstallation,
	desired *appsv1.Deployment,
	reqLogger logr.Logger,
) (*batchv1.Job, error) {
	reqLogger.Info(fmt.Sprintf("Running Mattermost update image job check for image %s", mattermost.GetMainContainer(desired).Image))
	job, err := r.fetchRunningUpdateJob(mattermost)
	if err != nil {
		// Unable to fetch job
		if k8sErrors.IsNotFound(err) {
			// Job is not running, let's launch
			reqLogger.Info("Launching update image job")
			if err = r.launchUpdateJob(mattermost, desired); err != nil {
				return nil, errors.Wrap(err, "Launching update image job failed")
			}
			return nil, errors.New("Began update image job")
		} else {
			return nil, errors.Wrap(err, "Error trying to determine if an update image job is already running")
		}
	}

	// Job is either running or completed

	if job.Status.CompletionTime == nil {
		return nil, errors.New("Update image job still running..")
	}

	// Job is completed, can check completion status

	if job.Status.Failed > 0 {
		return job, errors.New("Update image job failed")
	}

	reqLogger.Info("Update image job ran successfully")

	return job, nil
}

// cleanupUpdateJob deletes update job and all pods of the job
func (r *ReconcileClusterInstallation) cleanupUpdateJob(job *batchv1.Job, reqLogger logr.Logger) {
	reqLogger.Info(fmt.Sprintf("Deleting update image job %s/%s", job.GetNamespace(), job.GetName()))

	err := r.client.Delete(context.TODO(), job)
	if err != nil {
		reqLogger.Error(err, "Unable to cleanup update image job")
	}

	podList := &corev1.PodList{}
	listOptions := []k8sClient.ListOption{
		k8sClient.InNamespace(job.GetNamespace()),
		k8sClient.MatchingLabels(labels.Set(map[string]string{"app": updateJobName})),
	}

	err = r.client.List(context.Background(), podList, listOptions...)
	reqLogger.Info(fmt.Sprintf("Deleting %d pods", len(podList.Items)))
	for _, pod := range podList.Items {
		reqLogger.Info(fmt.Sprintf("Deleting pod %s/%s", pod.Namespace, pod.Name))
		err = r.client.Delete(context.TODO(), &pod)
		if err != nil {
			reqLogger.Error(err, fmt.Sprintf("Problem deleting pod %s/%s", pod.Namespace, pod.Name))
		}
	}
}

// fetchRunningUpdateJob gets update job
func (r *ReconcileClusterInstallation) fetchRunningUpdateJob(mi *mattermostv1alpha1.ClusterInstallation) (*batchv1.Job, error) {
	job := &batchv1.Job{}
	err := r.client.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      updateJobName,
			Namespace: mi.GetNamespace(),
		},
		job,
	)
	return job, err
}
