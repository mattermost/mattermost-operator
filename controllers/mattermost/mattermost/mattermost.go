package mattermost

import (
	"context"
	"fmt"

	"github.com/mattermost/mattermost-operator/pkg/resources"

	"github.com/go-logr/logr"
	"github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sClient "sigs.k8s.io/controller-runtime/pkg/client"
)

type reconcileStatus struct {
	ResourcesReady bool
}

func (r *MattermostReconciler) checkMattermost(
	mattermost *mmv1beta.Mattermost,
	dbInfo mattermostApp.DatabaseConfig,
	fileStoreInfo *mattermostApp.FileStoreInfo,
	status *mmv1beta.MattermostStatus,
	reqLogger logr.Logger) (reconcileStatus, error) {
	reqLogger = reqLogger.WithValues("Reconcile", "mattermost")
	recStatus := reconcileStatus{
		ResourcesReady: true,
	}

	err := r.checkLicence(mattermost)
	if err != nil {
		return reconcileStatus{}, errors.Wrap(err, "failed to check mattermost license secret.")
	}

	err = r.checkMattermostService(mattermost, status, reqLogger)
	if err != nil {
		return reconcileStatus{}, err
	}

	err = r.checkMattermostRBAC(mattermost, reqLogger)
	if err != nil {
		return reconcileStatus{}, err
	}

	if !mattermost.Spec.UseServiceLoadBalancer {
		err = r.checkMattermostIngressClass(mattermost, reqLogger)
		if err != nil {
			return reconcileStatus{}, err
		}

		err = r.checkMattermostIngress(mattermost, reqLogger)
		if err != nil {
			return reconcileStatus{}, err
		}
	}

	recStatus, err = r.checkMattermostDeployment(mattermost, dbInfo, fileStoreInfo, status, reqLogger)
	if err != nil {
		return reconcileStatus{}, err
	}

	return recStatus, nil
}

func (r *MattermostReconciler) checkLicence(mattermost *mmv1beta.Mattermost) error {
	if mattermost.Spec.LicenseSecret == "" {
		return nil
	}
	return r.assertSecretContains(mattermost.Spec.LicenseSecret, "license", mattermost.Namespace)
}

func (r *MattermostReconciler) checkMattermostService(
	mattermost *mmv1beta.Mattermost,
	status *mmv1beta.MattermostStatus,
	reqLogger logr.Logger) error {
	desired := mattermostApp.GenerateServiceV1Beta(mattermost)

	patchedObj, applied, err := mattermost.Spec.ResourcePatch.ApplyToService(desired)
	if err != nil {
		reqLogger.Error(err, "Failed to patch service")
		status.SetServicePatchStatus(false, errors.Wrap(err, "failed to apply patch to Service"))
	} else if applied {
		reqLogger.Info("Applied patch to service")
		desired = patchedObj
		status.SetServicePatchStatus(true, nil)
	} else {
		status.ClearServicePatchStatus()
	}

	err = r.Resources.CreateServiceIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return err
	}

	current := &corev1.Service{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return err
	}

	if current.Spec.Type != desired.Spec.Type && current.Spec.Type != "" {
		reqLogger.Info("Recreating service due to service type change", "name", current.Name)
		err := r.Resources.DeleteService(types.NamespacedName{Namespace: current.Namespace, Name: current.Name}, reqLogger)
		if err != nil {
			return errors.Wrap(err, "failed to delete service")
		}
		reqLogger.Info("Creating service", "name", desired.Name)

		return r.Resources.Create(mattermost, desired, reqLogger)
	}

	resources.CopyServiceEmptyAutoAssignedFields(desired, current)

	return r.Resources.Update(current, desired, reqLogger)
}

func (r *MattermostReconciler) checkMattermostRBAC(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	err := r.checkMattermostSA(mattermost, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to check mattermost ServiceAccount")
	}
	err = r.checkMattermostRole(mattermost, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to check mattermost Role")
	}
	err = r.checkMattermostRoleBinding(mattermost, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to check mattermost RoleBinding")
	}

	return nil
}

func (r *MattermostReconciler) checkMattermostSA(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	desired := mattermostApp.GenerateServiceAccountV1Beta(mattermost, mattermost.Name)
	err := r.Resources.CreateServiceAccountIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return err
	}

	current := &corev1.ServiceAccount{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return err
	}

	return r.Resources.Update(current, desired, reqLogger)
}

func (r *MattermostReconciler) checkMattermostRole(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	desired := mattermostApp.GenerateRoleV1Beta(mattermost, mattermost.Name)
	err := r.Resources.CreateRoleIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return err
	}

	current := &rbacv1.Role{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return err
	}

	return r.Resources.Update(current, desired, reqLogger)
}

func (r *MattermostReconciler) checkMattermostRoleBinding(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	desired := mattermostApp.GenerateRoleBindingV1Beta(mattermost, mattermost.Name, mattermost.Name)
	err := r.Resources.CreateRoleBindingIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return err
	}

	current := &rbacv1.RoleBinding{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return err
	}

	return r.Resources.Update(current, desired, reqLogger)
}

func (r *MattermostReconciler) checkMattermostIngress(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	var desired *networkingv1.Ingress
	desired = mattermostApp.GenerateIngressV1Beta(mattermost)

	if mattermost.Spec.AWSLoadBalancerController != nil && mattermost.Spec.AWSLoadBalancerController.Enabled {
		desired = mattermostApp.GenerateALBIngressV1Beta(mattermost)
	}

	if !mattermost.IngressEnabled() && !mattermost.AWSLoadBalancerEnabled() {
		err := r.Resources.DeleteIngress(types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, reqLogger)
		if err != nil {
			return errors.Wrap(err, "failed to delete disabled ingress")
		}
		return nil
	}

	err := r.Resources.CreateIngressIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return err
	}

	current := &networkingv1.Ingress{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return err
	}

	return r.Resources.Update(current, desired, reqLogger)
}

func (r *MattermostReconciler) checkMattermostIngressClass(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	var desired *networkingv1.IngressClass
	desired = mattermostApp.GenerateALBIngressClassV1Beta(mattermost)

	if mattermost.Spec.AWSLoadBalancerController == nil {
		err := r.Resources.DeleteIngressClass(types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, reqLogger)
		if err != nil {
			return errors.Wrap(err, "failed to delete disabled ingressClass")
		}
		return nil
	}

	err := r.Resources.CreateIngressClassIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return err
	}

	current := &networkingv1.IngressClass{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return err
	}

	return r.Resources.Update(current, desired, reqLogger)
}

func (r *MattermostReconciler) checkMattermostDeployment(
	mattermost *mmv1beta.Mattermost,
	dbConfig mattermostApp.DatabaseConfig,
	fileStoreInfo *mattermostApp.FileStoreInfo,
	status *mmv1beta.MattermostStatus,
	reqLogger logr.Logger) (reconcileStatus, error) {
	desired := mattermostApp.GenerateDeploymentV1Beta(
		mattermost,
		dbConfig,
		fileStoreInfo,
		mattermost.Name,
		mattermost.GetIngressHost(),
		mattermost.Name,
		mattermost.GetImageName(),
	)

	recStatus := reconcileStatus{
		ResourcesReady: true,
	}

	patchedObj, applied, err := mattermost.Spec.ResourcePatch.ApplyToDeployment(desired)
	if err != nil {
		reqLogger.Error(err, "Failed to patch deployment")
		status.SetDeploymentPatchStatus(false, errors.Wrap(err, "failed to apply patch to Deployment"))
		fmt.Println(mattermost.Status.ResourcePatch)
	} else if applied {
		reqLogger.Info("Applied patch to deployment")
		desired = patchedObj
		status.SetDeploymentPatchStatus(true, nil)
	} else {
		status.ClearDeploymentPatchStatus()
	}

	// TODO: DB setup job is temporarily disabled as `mattermost version` command
	// does not account for the custom configuration
	//err = r.checkMattermostDBSetupJob(mattermost, desired, reqLogger)
	//if err != nil {
	//	return errors.Wrap(err, "failed to check mattermost DB setup job")
	//}

	err = r.Resources.CreateDeploymentIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return reconcileStatus{}, errors.Wrap(err, "failed to create mattermost deployment")
	}

	current := &appsv1.Deployment{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return reconcileStatus{}, errors.Wrap(err, "failed to get mattermost deployment")
	}

	recStatus, err = r.updateMattermostDeployment(mattermost, current, desired, reqLogger)
	if err != nil {
		return reconcileStatus{}, errors.Wrap(err, "failed to update mattermost deployment")
	}

	return recStatus, nil
}

func (r *MattermostReconciler) checkMattermostDBSetupJob(mattermost *mmv1beta.Mattermost, deployment *appsv1.Deployment, reqLogger logr.Logger) error {
	desiredJob := resources.PrepareMattermostJobTemplate(mattermostApp.SetupJobName, mattermost.Namespace, deployment, mattermost.Spec.UpdateJob)
	desiredJob.OwnerReferences = mattermostApp.MattermostOwnerReference(mattermost)

	currentJob := &batchv1.Job{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: desiredJob.Name, Namespace: desiredJob.Namespace}, currentJob)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			reqLogger.Info("Creating DB setup job", "name", desiredJob.Name)
			return r.Resources.Create(mattermost, desiredJob, reqLogger)
		}
		return errors.Wrap(err, "failed to get current db setup job")
	}
	// For now, there is no need to perform job update, so just return.
	return nil
}

// isMainDeploymentContainerImageSame checks whether main containers of specified deployments are the same or not.
func (r *MattermostReconciler) isMainDeploymentContainerImageSame(
	a *appsv1.Deployment,
	b *appsv1.Deployment,
) (bool, error) {
	// Sanity check
	if (a == nil) || (b == nil) {
		return false, errors.New("failed to find main container, no deployment provided")
	}

	isSameImage, err := r.isMainContainerImageSame(
		a.Spec.Template.Spec.Containers,
		b.Spec.Template.Spec.Containers,
	)
	if err != nil {
		return false, errors.Wrapf(err, "failed to compare deployment images, deployments: %s/%s, %s/%s", a.Namespace, a.Name, b.Namespace, b.Name)
	}

	return isSameImage, nil
}

// isMainContainerImageSame checks whether main containers of specified slices are the same or not.
func (r *MattermostReconciler) isMainContainerImageSame(
	a []corev1.Container,
	b []corev1.Container,
) (bool, error) {
	// Fetch containers to compare
	containerA := mmv1beta.GetMattermostAppContainer(a)
	if containerA == nil {
		return false, errors.Errorf("failed to find main container in a list while comparing images")
	}
	containerB := mmv1beta.GetMattermostAppContainer(b)
	if containerB == nil {
		return false, errors.Errorf("failed to find main container in a list while comparing images")
	}

	// Both containers fetched, can compare images
	return containerA.Image == containerB.Image, nil
}

// updateMattermostDeployment performs deployment update if necessary.
// If a deployment update is necessary, an update job is launched to check new image.
func (r *MattermostReconciler) updateMattermostDeployment(
	mattermost *mmv1beta.Mattermost,
	current *appsv1.Deployment,
	desired *appsv1.Deployment,
	reqLogger logr.Logger,
) (reconcileStatus, error) {
	sameImage, err := r.isMainDeploymentContainerImageSame(current, desired)
	recStatus := reconcileStatus{
		ResourcesReady: true,
	}

	if err != nil {
		return reconcileStatus{}, err
	}

	if sameImage {
		// Need to update other fields only, update job is not required
		return recStatus, r.Resources.Update(current, desired, reqLogger)
	}

	// Image is not the same

	reqLogger.Info("Current image is not the same as the requested, will upgrade the Mattermost installation")

	if mattermost.Spec.UpdateJob != nil && mattermost.Spec.UpdateJob.Disabled {
		reqLogger.Info("Update job is disabled, new image will rollout without being verified")
		return recStatus, r.Resources.Update(current, desired, reqLogger)
	}

	// Run a single-pod job with the new mattermost image
	// It will check whether new image is operational
	// and may perform any database migrations before altering the deployment.
	// If this fails, we will return and not upgrade the deployment.

	job, recStatus, err := r.checkUpdateJob(mattermost, mattermost.Namespace, desired, reqLogger)
	if job != nil {
		// Job is done, need to cleanup
		defer r.cleanupUpdateJob(job, reqLogger)
	}
	if err != nil {
		return recStatus, err
	}

	// Job completed successfully
	if recStatus.ResourcesReady {
		return recStatus, r.Resources.Update(current, desired, reqLogger)
	}

	return recStatus, nil
}

// checkUpdateJob checks whether update job status. In case job is not running it is launched
func (r *MattermostReconciler) checkUpdateJob(
	mattermost *v1beta1.Mattermost,
	jobNamespace string,
	baseDeployment *appsv1.Deployment,
	reqLogger logr.Logger,
) (*batchv1.Job, reconcileStatus, error) {
	recStatus := reconcileStatus{
		ResourcesReady: true,
	}
	reqLogger.Info(fmt.Sprintf("Running Mattermost update image job check for image %s", mmv1beta.GetMattermostAppContainerFromDeployment(baseDeployment).Image))
	job, err := r.Resources.FetchMattermostUpdateJob(jobNamespace)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			reqLogger.Info("Launching update image job")
			if err = r.Resources.LaunchMattermostUpdateJob(mattermost, jobNamespace, baseDeployment, reqLogger, mattermost.Spec.UpdateJob); err != nil {
				return nil, reconcileStatus{}, errors.Wrap(err, "Launching update image job failed")
			}
			recStatus.ResourcesReady = false
			return nil, recStatus, nil
		}

		return nil, reconcileStatus{}, errors.Wrap(err, "failed to determine if an update image job is already running")
	}

	// Job is either running or completed

	// If desired deployment image does not match the one used by update job, restart it.
	isSameImage, err := r.isMainContainerImageSame(
		baseDeployment.Spec.Template.Spec.Containers,
		job.Spec.Template.Spec.Containers,
	)
	if err != nil {
		return nil, reconcileStatus{}, errors.Wrap(err, "failed to compare image of update job and desired deployment")
	}
	if !isSameImage {
		reqLogger.Info("Mattermost image changed, restarting update job")
		err = r.Resources.RestartMattermostUpdateJob(mattermost, job, baseDeployment, reqLogger, mattermost.Spec.UpdateJob)
		if err != nil {
			recStatus.ResourcesReady = false
			return nil, recStatus, errors.Wrap(err, "failed to restart update job")
		}

		recStatus.ResourcesReady = false
		reqLogger.Info("Restarted update image job")
		return nil, recStatus, nil
	}

	if job.Status.CompletionTime == nil {
		recStatus.ResourcesReady = false
		reqLogger.Info("update image job still running")
		return nil, recStatus, nil
	}

	// Job is completed, can check completion status

	if job.Status.Failed > 0 {
		recStatus.ResourcesReady = false
		return job, recStatus, errors.New("update image job failed")
	}

	reqLogger.Info("Update image job ran successfully")

	return job, recStatus, nil
}

// cleanupUpdateJob deletes update job and all pods of the job
func (r *MattermostReconciler) cleanupUpdateJob(job *batchv1.Job, reqLogger logr.Logger) {
	reqLogger.Info(fmt.Sprintf("Deleting update image job %s/%s", job.GetNamespace(), job.GetName()))

	err := r.Client.Delete(context.TODO(), job, k8sClient.PropagationPolicy(metav1.DeletePropagationBackground))
	if err != nil {
		// Do not return error on fail as it is not critical
		reqLogger.Error(err, "Unable to cleanup update image job")
	}
}
