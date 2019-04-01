package clusterinstallation

import (
	"context"
	"fmt"
	"reflect"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	mattermostmysql "github.com/mattermost/mattermost-operator/pkg/components/mysql"

	mysqlOperator "github.com/oracle/mysql-operator/pkg/apis/mysql/v1alpha1"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	rbacv1beta1 "k8s.io/api/rbac/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_clusterinstallation")

// Add creates a new ClusterInstallation Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileClusterInstallation{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("clusterinstallation-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ClusterInstallation
	err = c.Watch(&source.Kind{Type: &mattermostv1alpha1.ClusterInstallation{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner ClusterInstallation
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mattermostv1alpha1.ClusterInstallation{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterInstallation{}

// ReconcileClusterInstallation reconciles a ClusterInstallation object
type ReconcileClusterInstallation struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a ClusterInstallation object and makes changes based on the state read
// and what is in the ClusterInstallation.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileClusterInstallation) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ClusterInstallation")

	// Fetch the ClusterInstallation instance
	mattermost := &mattermostv1alpha1.ClusterInstallation{}
	err := r.client.Get(context.TODO(), request.NamespacedName, mattermost)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	err = mattermost.SetDefaults()
	if err != nil {
		return reconcile.Result{}, err
	}

	if mattermost.Spec.DatabaseType.Type == "mysql" {
		reqLogger.Info("Reconciling ClusterInstallation Database MySQL service account")
		err = r.checkDBServiceAccount(mattermost, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}

		reqLogger.Info("Reconciling ClusterInstallation Database MySQL role binding")
		err = r.checkDBRoleBinding(mattermost, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}

		reqLogger.Info("Reconciling ClusterInstallation Database MySQL")
		err = r.checkDBMySQLDeployment(mattermost, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else {
		reqLogger.Info("Reconciling ClusterInstallation Database Postgres")
		err = r.checkDBPostgresDeployment(mattermost, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// TODO
	err = r.checkMinioDeployment(mattermost, reqLogger)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.checkMattermostDeployment(mattermost, reqLogger)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Already exists - don't requeue
	return reconcile.Result{}, nil
}

// createServiceAccount creates the mysql service account for the mysql operator
func (r *ReconcileClusterInstallation) createServiceAccount(mattermost *mattermostv1alpha1.ClusterInstallation, serviceAccount *corev1.ServiceAccount, reqLogger logr.Logger) error {
	foundServiceAccount := &corev1.ServiceAccount{}

	errGet := r.client.Get(context.TODO(), types.NamespacedName{Name: serviceAccount.Name, Namespace: serviceAccount.Namespace}, foundServiceAccount)
	if errGet != nil && errors.IsNotFound(errGet) {
		reqLogger.Info("ClusterInstallation Database Service account not found, will deploy one.", "Err", errGet.Error())
		reqLogger.Info("Creating Service Account")
		if err := r.client.Create(context.TODO(), serviceAccount); err != nil {
			reqLogger.Info("Error creating Service Account", "Error", err.Error())
			return err
		}
		reqLogger.Info("Creating Database Service Account completed")
		if err := controllerutil.SetControllerReference(mattermost, serviceAccount, r.scheme); err != nil {
			return err
		}
	} else if errGet != nil {
		reqLogger.Error(errGet, "ClusterInstallation Database Service Account")
		return errGet
	}

	return nil
}

func (r *ReconcileClusterInstallation) checkDBServiceAccount(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	mysqlServiceAccount := mattermostmysql.ServiceAccount(mattermost)

	foundServiceAccount := &corev1.ServiceAccount{}

	errGet := r.client.Get(context.TODO(), types.NamespacedName{Name: mysqlServiceAccount.Name, Namespace: mysqlServiceAccount.Namespace}, foundServiceAccount)
	if errGet != nil && errors.IsNotFound(errGet) {
		return r.createServiceAccount(mattermost, mysqlServiceAccount, reqLogger)
	} else if errGet != nil {
		reqLogger.Error(errGet, "ClusterInstallation Database Service Account")
		return errGet
	}

	return nil
}

func (r *ReconcileClusterInstallation) checkDBRoleBinding(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	roleBinding := mattermostmysql.RoleBinding(mattermost)

	foundRoleBinding := &rbacv1beta1.RoleBinding{}
	errGet := r.client.Get(context.TODO(), types.NamespacedName{Name: roleBinding.Name, Namespace: roleBinding.Namespace}, foundRoleBinding)
	if errGet != nil && errors.IsNotFound(errGet) {
		reqLogger.Info("ClusterInstallation Database Role Binding not found, will deploy one.", "Err", errGet.Error())
		reqLogger.Info("Creating Role Binding")
		if err := r.client.Create(context.TODO(), roleBinding); err != nil {
			reqLogger.Info("Error Role Binding", "Error", err.Error())
			return err
		}
		// TODO add logic to wait for the completion
		reqLogger.Info("Creating Database Role Binding completed")
		if err := controllerutil.SetControllerReference(mattermost, roleBinding, r.scheme); err != nil {
			return err
		}
	} else if errGet != nil {
		reqLogger.Error(errGet, "ClusterInstallation Database Role Binding")
		return errGet
	}

	return nil
}

func (r *ReconcileClusterInstallation) checkDBMySQLDeployment(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	reqLogger.Info("ClusterInstallation Database using Oracle Mysql Operator")
	deployDB := mattermostmysql.Cluster(mattermost)

	foundDB := &mysqlOperator.Cluster{}
	errGet := r.client.Get(context.TODO(), types.NamespacedName{Name: deployDB.Name, Namespace: deployDB.Namespace}, foundDB)
	if errGet != nil && errors.IsNotFound(errGet) {
		reqLogger.Info("ClusterInstallation Database not found, will deploy one.", "Err", errGet.Error())
		reqLogger.Info("Creating Database", "Members", deployDB.Spec.Members)
		if err := r.client.Create(context.TODO(), deployDB); err != nil {
			reqLogger.Info("Error creating Database", "Error", err.Error())
			return err
		}
		reqLogger.Info("Creating Database completed")
		if err := controllerutil.SetControllerReference(mattermost, deployDB, r.scheme); err != nil {
			return err
		}
	} else if errGet != nil {
		reqLogger.Error(errGet, "ClusterInstallation Database")
		return errGet
	}

	// Set MattermostService instance as the owner and controller
	if !reflect.DeepEqual(deployDB.Spec, foundDB.Spec) {
		foundDB.Spec = deployDB.Spec
		reqLogger.Info("Updating DB", deployDB.Namespace, deployDB.Name)
		err := r.client.Update(context.TODO(), foundDB)
		if err != nil {
			return err
		}
		_ = controllerutil.SetControllerReference(mattermost, foundDB, r.scheme)
	}

	return nil
}

func (r *ReconcileClusterInstallation) checkDBPostgresDeployment(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	dbExist := false

	//TODO: Create the logic to check if the DB Postgres already exist or changed otherwise create
	if dbExist {
		return r.client.Update(context.TODO(), mattermost)
	}
	return nil
}

func (r *ReconcileClusterInstallation) checkMinioDeployment(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	minioExist := false

	//TODO: Create the logic to check if the Minio already exist or changed otherwise create
	if minioExist {
		return r.client.Update(context.TODO(), mattermost)
	}
	return nil
}

func (r *ReconcileClusterInstallation) getMySQLSecrets(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) (string, error) {
	dbSecretName := fmt.Sprintf("%s-mysql-root-password", mattermost.Name)
	dbSecret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: dbSecretName, Namespace: mattermost.Namespace}, dbSecret)
	if err != nil {
		return "", err
	}
	return string(dbSecret.Data["password"]), nil
}

func (r *ReconcileClusterInstallation) checkMattermostDeployment(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	err := r.checkService(mattermost, reqLogger)
	if err != nil {
		return err
	}

	err = r.checkIngress(mattermost, reqLogger)
	if err != nil {
		return err
	}

	err = r.checkDeployment(mattermost, reqLogger)
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileClusterInstallation) checkService(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	svc := mattermost.GenerateService()

	foundService := &corev1.Service{}
	errGet := r.client.Get(context.TODO(), types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, foundService)
	if errGet != nil && errors.IsNotFound(errGet) {
		reqLogger.Info("ClusterInstallation Service not found, will deploy one.", "Err", errGet.Error())
		reqLogger.Info("Creating Mattermost Service")
		if err := r.client.Create(context.TODO(), svc); err != nil {
			reqLogger.Info("Error creating Service", "Error", err.Error())
			return err
		}
		reqLogger.Info("Creating Mattermost Service completed")
		if err := controllerutil.SetControllerReference(mattermost, svc, r.scheme); err != nil {
			return err
		}
	} else if errGet != nil {
		reqLogger.Error(errGet, "ClusterInstallation Service")
		return errGet
	}

	// TODO check how to do the update
	// if !reflect.DeepEqual(serviceMM, foundService) {
	// 	clusterIP := foundService.Spec.ClusterIP
	// 	foundService = serviceMM
	// 	foundService.Spec.ClusterIP = clusterIP
	// 	reqLogger.Info("Updating Mattermost Service")
	// 	err = r.client.Update(context.TODO(), foundService)
	// 	if err != nil {
	// 		return err
	// 	}
	// _ = controllerutil.SetControllerReference(mattermost, foundService, r.scheme)
	// }
	return nil
}

func (r *ReconcileClusterInstallation) checkIngress(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	ingressMM := mattermost.GenerateIngress()

	foundIngress := &v1beta1.Ingress{}
	errGet := r.client.Get(context.TODO(), types.NamespacedName{Name: ingressMM.Name, Namespace: ingressMM.Namespace}, foundIngress)
	if errGet != nil && errors.IsNotFound(errGet) {
		reqLogger.Info("ClusterInstallation Ingress not found, will deploy one.", "Err", errGet.Error())
		reqLogger.Info("Creating Mattermost Ingress")
		if err := r.client.Create(context.TODO(), ingressMM); err != nil {
			reqLogger.Info("Error creating Ingress", "Error", err.Error())
			return err
		}
		reqLogger.Info("Creating Mattermost Ingress completed")
		if err := controllerutil.SetControllerReference(mattermost, ingressMM, r.scheme); err != nil {
			return err
		}
	} else if errGet != nil {
		reqLogger.Error(errGet, "ClusterInstallation Ingress")
		return errGet
	}

	// TODO check how to do the update
	// if !reflect.DeepEqual(ingressMM, foundIngress) {
	// 	foundIngress = ingressMM
	// 	reqLogger.Info("Updating Mattermost Ingress")
	// 	err = r.client.Update(context.TODO(), foundIngress)
	// 	if err != nil {
	// 		return err
	// 	}
	// _ = controllerutil.SetControllerReference(mattermost, foundIngress, r.scheme)
	// }
	return nil
}

func (r *ReconcileClusterInstallation) checkDeployment(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	dbPassword, err := r.getMySQLSecrets(mattermost, reqLogger)
	if err != nil {
		return fmt.Errorf("Error getting the database password. Err=%s", err.Error())
	}

	deployMM := mattermost.GenerateDeployment("", dbPassword)

	foundMM := &appsv1.Deployment{}
	errGet := r.client.Get(context.TODO(), types.NamespacedName{Name: mattermost.Name, Namespace: mattermost.Namespace}, foundMM)
	if errGet != nil && errors.IsNotFound(errGet) {
		reqLogger.Info("ClusterInstallation App not found, will deploy one.", "Err", errGet.Error())
		reqLogger.Info("Creating Mattermost Ingress")
		if err := r.client.Create(context.TODO(), deployMM); err != nil {
			reqLogger.Info("Error creating Mattermost Application", "Error", err.Error())
			return err
		}
		reqLogger.Info("Creating Mattermost Application completed")
		if err := controllerutil.SetControllerReference(mattermost, deployMM, r.scheme); err != nil {
			return err
		}
	} else if errGet != nil {
		reqLogger.Error(errGet, "ClusterInstallation Application")
		return errGet
	}

	// TODO check how to do the update
	// if !reflect.DeepEqual(ingressMM, foundIngress) {
	// 	foundIngress = ingressMM
	// 	reqLogger.Info("Updating Mattermost Ingress")
	// 	err = r.client.Update(context.TODO(), foundIngress)
	// 	if err != nil {
	// 		return err
	// 	}
	// _ = controllerutil.SetControllerReference(mattermost, foundIngress, r.scheme)
	// }
	return nil
}
