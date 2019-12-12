// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package mattermostrestoredb

import (
	"context"
	"fmt"
	"time"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	mattermostmysql "github.com/mattermost/mattermost-operator/pkg/components/mysql"

	errrors "github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_mattermostrestoredb")

// Add creates a new MattermostRestoreDB Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileMattermostRestoreDB{client: mgr.GetClient(), scheme: mgr.GetScheme(), state: mattermostv1alpha1.Restoring}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("mattermostrestoredb-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource MattermostRestoreDB
	err = c.Watch(&source.Kind{Type: &mattermostv1alpha1.MattermostRestoreDB{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileMattermostRestoreDB implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileMattermostRestoreDB{}

// ReconcileMattermostRestoreDB reconciles a MattermostRestoreDB object
type ReconcileMattermostRestoreDB struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
	state  mattermostv1alpha1.RestoreState
}

func (r *ReconcileMattermostRestoreDB) setRestoring() {
	r.state = mattermostv1alpha1.Restoring
}

func (r *ReconcileMattermostRestoreDB) setFinished() {
	r.state = mattermostv1alpha1.Finished
}

func (r *ReconcileMattermostRestoreDB) setFailed() {
	r.state = mattermostv1alpha1.Failed
}

// Reconcile reads that state of the cluster for a MattermostRestoreDB object and makes changes based on the state read
// and what is in the MattermostRestoreDB.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileMattermostRestoreDB) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling MattermostRestoreDB")
	// Fetch the MattermostRestoreDB instance
	restoreMM := &mattermostv1alpha1.MattermostRestoreDB{}
	err := r.client.Get(context.TODO(), request.NamespacedName, restoreMM)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		r.setFailed()
		return reconcile.Result{}, err
	}

	// Check if this Mattermost ClusterInstallation exists
	clusterInstallation := &mattermostv1alpha1.ClusterInstallation{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: restoreMM.Spec.MattermostClusterName, Namespace: restoreMM.Namespace}, clusterInstallation)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Error(err, "Mattermost Installation not found. Create a ClusterInstallation first", "Namespace", restoreMM.Namespace, "ClusterInstallation.Name", restoreMM.Spec.MattermostClusterName, "RestoreDB.name", restoreMM.Name)
		r.setFailed()
		status := restoreMM.Status
		status.State = r.state
		//TODO: add reason and inform need to delete/apply when a clusterinstallation is ready. JIRA MM-18633
		err = r.updateStatus(restoreMM, status, reqLogger)
		return reconcile.Result{Requeue: false}, err
	} else if err != nil {
		reqLogger.Error(err, "Error trying to get the Mattermost ClusterInstallation", "Namespace", restoreMM.Namespace, "ClusterInstallation.Name", restoreMM.Spec.MattermostClusterName, "RestoreDB.name", restoreMM.Name)
		r.setFailed()
		status := restoreMM.Status
		status.State = r.state
		err = r.updateStatus(restoreMM, status, reqLogger)
		return reconcile.Result{Requeue: false}, err
	}

	// Set the Status and save the DB Replicas in the Status
	if restoreMM.Status.State != r.state {
		reqLogger.Info("Setting restore controller status", "State", r.state, "OriginalReplicas", clusterInstallation.Spec.Database.Replicas)
		status := restoreMM.Status
		status.State = r.state
		status.OriginalDBReplicas = clusterInstallation.Spec.Database.Replicas
		err = r.updateStatus(restoreMM, status, reqLogger)
		if err != nil {
			r.setRestoring()
			return reconcile.Result{}, err
		}
	}

	// Scaling down to apply the restore later
	if clusterInstallation.Spec.Database.Replicas != 0 {
		clusterInstallation.Spec.Database.Replicas = 0
		clusterInstallation.Spec.Database.InitBucketURL = restoreMM.Spec.InitBucketURL
		clusterInstallation.Spec.Database.BackupRestoreSecretName = restoreMM.Spec.RestoreSecret

		err = r.client.Update(context.TODO(), clusterInstallation)
		if err != nil {
			reqLogger.Error(err, "failed to update the clusterinstallation spec")
			return reconcile.Result{}, err
		}
	}

	// Update the mysql secret to use the existing users
	if restoreMM.Spec.MattermostDBName != "" || restoreMM.Spec.MattermostDBUser != "" || restoreMM.Spec.MattermostDBPassword != "" {
		err = r.updateMySQLSecrets(restoreMM, reqLogger)
		if err != nil {
			reqLogger.Error(err, "failed to update the mysql secret")
			return reconcile.Result{}, err
		}
	}

	// Checking if the MySQL Cluster is scaled down to 0
	mySQLCluster := mattermostmysql.Cluster(clusterInstallation)
	statefulsetMySQLName := fmt.Sprintf("%s-mysql", mySQLCluster.Name)
	statefulset := &appsv1.StatefulSet{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: statefulsetMySQLName, Namespace: restoreMM.Namespace}, statefulset)
	if err != nil {
		reqLogger.Error(err, "error getting the MySQL Statefulset")
		return reconcile.Result{}, errrors.Wrap(err, "unable to get the statefulset")
	}
	if statefulset.Status.ReadyReplicas != 0 {
		return reconcile.Result{RequeueAfter: time.Second * 3}, fmt.Errorf("Waiting for MySQL Statefulset scale to 0")
	}

	pods := &corev1.PodList{}

	listOpts := []client.ListOption{
		client.InNamespace(mySQLCluster.GetNamespace()),
		client.MatchingLabels(mattermostv1alpha1.MySQLLabels()),
	}
	err = r.client.List(context.TODO(), pods, listOpts...)
	if err != nil && !errors.IsNotFound(err) {
		return reconcile.Result{}, errrors.Wrap(err, "unable to get pod list")
	}
	if len(pods.Items) != 0 {
		reqLogger.Info("Current MySQL Pods", "Number of Pods", len(pods.Items), "Namespace", mySQLCluster.GetNamespace())
		return reconcile.Result{RequeueAfter: time.Second * 3}, fmt.Errorf("Waiting for MySQL Statefulset pods scale to 0")
	}

	reqLogger.Info("MySQL Statefulset are scaled down. Will continue the restore process", "ReadyReplicas", statefulset.Status.ReadyReplicas, "CurrentReplicas", statefulset.Status.CurrentReplicas, "OriginalReplicas", restoreMM.Status.OriginalDBReplicas)

	// Removing all the PVC for MySQL to be able to apply the restore
	for i := 0; i < int(restoreMM.Status.OriginalDBReplicas); i++ {
		persistentVolumeClaim := &corev1.PersistentVolumeClaim{}
		dbPersistentVolClaim := fmt.Sprintf("data-%s-mysql-%d", mySQLCluster.Name, i)
		reqLogger.Info("Deleting PVC...", "PersistentVolumeClaimName", dbPersistentVolClaim)
		errGet := r.client.Get(context.TODO(), types.NamespacedName{Name: dbPersistentVolClaim, Namespace: restoreMM.Namespace}, persistentVolumeClaim)
		if errGet != nil && errors.IsNotFound(errGet) {
			reqLogger.Info("PVC not found maybe already deleted, skipping", "PersistentVolumeClaimName", dbPersistentVolClaim)
			continue
		}
		if errGet != nil {
			return reconcile.Result{}, errGet
		}

		errDelete := r.client.Delete(context.TODO(), persistentVolumeClaim, client.GracePeriodSeconds(0))
		if errDelete != nil {
			reqLogger.Error(errDelete, "error deleting the DB PVC", "ClusterInstallation.Namespace", clusterInstallation.Namespace, "ClusterInstallation.Name", clusterInstallation.Name)
			return reconcile.Result{}, errDelete
		}
		reqLogger.Info("PVC deleted", "PersistentVolumeClaimName", dbPersistentVolClaim)
	}

	// Scale up again to apply the restore
	clusterInstallation.Spec.Database.Replicas = restoreMM.Status.OriginalDBReplicas
	if restoreMM.Status.OriginalDBReplicas == 0 {
		// at least set to one replica
		clusterInstallation.Spec.Database.Replicas = 1
	}
	err = r.client.Update(context.TODO(), clusterInstallation)
	if err != nil {
		reqLogger.Error(err, "failed to update the clusterinstallation spec")
		return reconcile.Result{}, err
	}

	reqLogger.Info("Restore complete", "restoreMM.Namespace", restoreMM.Namespace, "restoreMM.Name", restoreMM.Name)
	r.setFinished()
	status := restoreMM.Status
	status.State = r.state
	err = r.updateStatus(restoreMM, status, reqLogger)
	if err != nil {
		reqLogger.Error(err, "failed to update the clusterinstallation status")
	}

	return reconcile.Result{}, nil
}
