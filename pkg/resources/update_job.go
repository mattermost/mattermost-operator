package resources

import (
	"context"

	"github.com/go-logr/logr"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sClient "sigs.k8s.io/controller-runtime/pkg/client"
)

const UpdateJobName = "mattermost-update-check"

func (r *ResourceHelper) LaunchMattermostUpdateJob(
	owner metav1.Object,
	jobNamespace string,
	baseDeployment *appsv1.Deployment,
	reqLogger logr.Logger,
) error {
	job := PrepareMattermostJobTemplate(UpdateJobName, jobNamespace, baseDeployment)

	err := r.Create(owner, job, reqLogger)
	if err != nil && !k8sErrors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

// RestartMattermostUpdateJob removes existing update job if it exists and creates new one.
func (r *ResourceHelper) RestartMattermostUpdateJob(
	owner metav1.Object,
	currentJob *batchv1.Job,
	deployment *appsv1.Deployment,
	reqLogger logr.Logger,
) error {
	err := r.client.Delete(context.TODO(), currentJob, k8sClient.PropagationPolicy(metav1.DeletePropagationBackground))
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete outdated update job")
	}

	job := PrepareMattermostJobTemplate(UpdateJobName, currentJob.Namespace, deployment)

	err = r.Create(owner, job, reqLogger)
	if err != nil {
		return err
	}

	return nil
}

// FetchMattermostUpdateJob gets update job
func (r *ResourceHelper) FetchMattermostUpdateJob(namespace string) (*batchv1.Job, error) {
	job := &batchv1.Job{}
	err := r.client.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      UpdateJobName,
			Namespace: namespace,
		},
		job,
	)
	return job, err
}

func PrepareMattermostJobTemplate(name, namespace string, baseDeployment *appsv1.Deployment) *batchv1.Job {
	backoffLimit := int32(10)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: *baseDeployment.Spec.Template.Spec.DeepCopy(),
			},
			BackoffLimit: &backoffLimit,
		},
	}

	// Remove init container that waits for db setup job.
	job.Spec.Template.Spec.InitContainers = mattermostApp.RemoveContainer(
		mattermostApp.WaitForDBSetupContainerName,
		job.Spec.Template.Spec.InitContainers,
	)

	// We dont need to validate the readiness/liveness for this short lived job.
	for i := range job.Spec.Template.Spec.Containers {
		job.Spec.Template.Spec.Containers[i].LivenessProbe = nil
		job.Spec.Template.Spec.Containers[i].ReadinessProbe = nil
	}

	// Override values for job-specific behavior.
	job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
	for i := range job.Spec.Template.Spec.Containers {
		job.Spec.Template.Spec.Containers[i].Command = []string{"mattermost", "version"}
	}

	return job
}
