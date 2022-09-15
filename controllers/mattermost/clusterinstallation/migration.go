package clusterinstallation

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/mattermost/mattermost-operator/pkg/mattermost/healthcheck"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MigrationResult struct {
	Status    string
	RequeueIn time.Duration
	Finished  bool
}

// HandleMigration performs necessary steps to migrate ClusterInstallation to Mattermost.
func (r *ClusterInstallationReconciler) HandleMigration(ci *mattermostv1alpha1.ClusterInstallation, logger logr.Logger) (MigrationResult, error) {
	logger = logger.WithValues("migration", "v1beta")

	err := r.IsConvertible(ci)
	if err != nil {
		logger.Error(err, "ClusterInstallation cannot be converted to Mattermost CR")
		return MigrationResult{
			Status: fmt.Sprintf("Migration to Mattermost cannot be performed safely: %s", err.Error()),
		}, nil
	}

	name := types.NamespacedName{Name: ci.Name, Namespace: ci.Namespace}

	var existingMM mmv1beta.Mattermost
	err = r.Get(context.Background(), name, &existingMM)
	if err != nil && !k8sErrors.IsNotFound(err) {
		return MigrationResult{}, errors.Wrap(err, "failed to check if Mattermost CR exists")
	}
	if k8sErrors.IsNotFound(err) {
		finished := false
		finished, err = r.initializeMigration(ci, logger)
		if err != nil {
			return MigrationResult{}, errors.Wrap(err, "failed to initialize migration")
		}

		if !finished {
			return MigrationResult{
				RequeueIn: 10 * time.Second,
				Status:    "Migration to Mattermost is in progress - recreating deployment",
			}, nil
		}
	}

	if existingMM.Status.State != mmv1beta.Stable {
		logger.Info("Migration not finished. Waiting for Mattermost to be in 'stable' state")
		return MigrationResult{
			RequeueIn: 10 * time.Second,
			Status:    "Migration to Mattermost is in progress - waiting for Mattermost to be ready",
		}, nil
	}

	logger.Info("Migration finished. Removing old Replica Sets and ClusterInstallation")
	err = r.cleanupReplicaSets(ci)
	if err != nil {
		return MigrationResult{}, errors.Wrap(err, "failed to cleanup old Replica Sets")
	}
	err = r.Client.Delete(context.Background(), ci)
	if err != nil && !k8sErrors.IsNotFound(err) {
		return MigrationResult{}, errors.Wrap(err, "failed to cleanup Cluster Installation")
	}

	return MigrationResult{Finished: true}, nil
}

// initializeMigration initializes migration of ClusterInstallation.
// Returns indication if the initialization if finished or an error.
func (r *ClusterInstallationReconciler) initializeMigration(ci *mattermostv1alpha1.ClusterInstallation, logger logr.Logger) (bool, error) {
	mm, err := r.ConvertToMM(ci)
	if err != nil {
		return false, errors.Wrap(err, "failed to convert ClusterInstallation to Mattermost")
	}

	isMigrated, err := r.isDeploymentMigrated(mm, ci)
	if err != nil {
		return false, errors.Wrap(err, "failed to check if Deployment is migrated")
	}

	if !isMigrated {
		logger.Info("Deployment is not migrated. Starting recreation")
		err = r.recreateDeployment(mm, ci, logger)
		if err != nil {
			return false, errors.Wrap(err, "failed to migrate Deployment")
		}
	}

	isReady, err := r.isDeploymentReady(mm, ci, logger)
	if err != nil {
		return false, errors.Wrap(err, "failed to check if deployment is ready")
	}

	if !isReady {
		logger.Info("Deployment is not ready after recreation")
		return false, nil
	}

	logger.Info("Deployment migration finished. Creating Mattermost CR")
	err = r.Create(context.TODO(), mm)
	if err != nil {
		return false, errors.Wrap(err, "failed to create Mattermost CR")
	}

	return true, nil
}

func (r *ClusterInstallationReconciler) recreateDeployment(mm *mmv1beta.Mattermost, ci *mattermostv1alpha1.ClusterInstallation, logger logr.Logger) error {
	oldDeploymentName := types.NamespacedName{Name: ci.Name, Namespace: ci.Namespace}
	var oldDeployment appsv1.Deployment
	err := r.Client.Get(context.TODO(), oldDeploymentName, &oldDeployment)
	if err != nil {
		return errors.Wrap(err, "failed to get old deployment")
	}

	orphanPropagation := v1.DeletePropagationOrphan
	err = r.Client.Delete(context.TODO(), &oldDeployment, client.PropagationPolicy(orphanPropagation))
	if err != nil {
		return errors.Wrap(err, "failed to delete old deployment")
	}

	err = r.waitForDeploymentDeletion(oldDeploymentName, logger)
	if err != nil {
		return errors.Wrap(err, "error while waiting for deployment deletion")
	}

	newLabels := mm.MattermostLabels(mm.Name)
	selectorLabels := mmv1beta.MattermostSelectorLabels(mm.Name)
	newDeployment := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      ci.Name,
			Namespace: ci.Namespace,
			Labels:    newLabels,
		},
		Spec: *oldDeployment.Spec.DeepCopy(),
	}
	newDeployment.Spec.Selector = &v1.LabelSelector{MatchLabels: selectorLabels}
	newDeployment.Spec.Template.Labels = newLabels

	err = r.Client.Create(context.TODO(), newDeployment)
	if err != nil {
		return errors.Wrap(err, "failed to create new Deployment")
	}

	return nil
}

func (r *ClusterInstallationReconciler) waitForDeploymentDeletion(name types.NamespacedName, logger logr.Logger) error {
	timeout := time.After(1 * time.Minute)

	for {
		var deployment appsv1.Deployment

		err := r.NonCachedAPIReader.Get(context.TODO(), name, &deployment)
		if err != nil && k8sErrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			// Log error but do not fail until timeout is reached
			logger.Error(err, "failed to get old deployment while waiting for deletion")
		}

		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for deployment deletion")
		default:
			time.Sleep(2 * time.Second)
		}
	}
}

func (r *ClusterInstallationReconciler) isDeploymentMigrated(mm *mmv1beta.Mattermost, ci *mattermostv1alpha1.ClusterInstallation) (bool, error) {
	labels := mm.MattermostLabels(mm.Name)
	listOptions := []client.ListOption{
		client.InNamespace(ci.Namespace),
		client.MatchingLabels(labels),
	}

	var deployments appsv1.DeploymentList
	err := r.Client.List(context.TODO(), &deployments, listOptions...)
	if err != nil {
		return false, errors.Wrap(err, "failed to list deployments")
	}

	return len(deployments.Items) == 1, nil
}

func (r *ClusterInstallationReconciler) isDeploymentReady(mm *mmv1beta.Mattermost, ci *mattermostv1alpha1.ClusterInstallation, logger logr.Logger) (bool, error) {
	labels := mm.MattermostLabels(mm.Name)
	listOptions := []client.ListOption{
		client.InNamespace(ci.Namespace),
		client.MatchingLabels(labels),
	}

	var deployments appsv1.DeploymentList
	err := r.Client.List(context.TODO(), &deployments, listOptions...)
	if err != nil {
		return false, errors.Wrap(err, "failed to list deployments")
	}

	mmDeployment := getDeployment(deployments, mm.Name)
	if mmDeployment == nil {
		return false, fmt.Errorf("failed to found migrated deployment")
	}

	healthChecker := healthcheck.NewHealthChecker(r.NonCachedAPIReader, listOptions, logger)

	status, err := healthChecker.CheckReplicaSetRollout(mm.Name, mm.Namespace)
	if err != nil {
		return false, errors.Wrap(err, "failed to check if pods are ready")
	}

	var replicas int32 = 1
	if mm.Spec.Replicas != nil {
		replicas = *mm.Spec.Replicas
	}

	return replicas == status.UpdatedReplicas, nil
}

func (r *ClusterInstallationReconciler) cleanupReplicaSets(ci *mattermostv1alpha1.ClusterInstallation) error {
	listOptions := []client.ListOption{
		client.InNamespace(ci.Namespace),
		client.MatchingLabels(ci.ClusterInstallationLabels(ci.Name)),
	}

	replicaSets := appsv1.ReplicaSetList{}
	err := r.List(context.TODO(), &replicaSets, listOptions...)
	if err != nil {
		return errors.Wrap(err, "failed to list Replica Sets")
	}

	for _, res := range replicaSets.Items {
		err = r.Delete(context.TODO(), &res)
		if err != nil {
			return errors.Wrap(err, "failed to remove old Replica Sets")
		}
	}

	return nil
}

func getDeployment(list appsv1.DeploymentList, name string) *appsv1.Deployment {
	for _, d := range list.Items {
		if d.Name == name {
			return &d
		}
	}

	return nil
}
