package mattermostcluster

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	"github.com/mattermost/mattermost-operator/pkg/controller/mattermostcluster/constants"

	mysqlOperator "github.com/oracle/mysql-operator/pkg/apis/mysql/v1alpha1"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	rbacv1beta1 "k8s.io/api/rbac/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_mattermostcluster")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new MattermostCluster Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileMattermostCluster{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("mattermostcluster-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource MattermostCluster
	err = c.Watch(&source.Kind{Type: &mattermostv1alpha1.MattermostCluster{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner MattermostCluster
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mattermostv1alpha1.MattermostCluster{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileMattermostCluster{}

// ReconcileMattermostCluster reconciles a MattermostCluster object
type ReconcileMattermostCluster struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a MattermostCluster object and makes changes based on the state read
// and what is in the MattermostCluster.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileMattermostCluster) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling MattermostCluster")

	// Fetch the MattermostCluster instance
	mattermost := &mattermostv1alpha1.MattermostCluster{}
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

	err = r.setDefaults(mattermost, reqLogger)
	if err != nil {
		return reconcile.Result{}, err
	}

	if mattermost.Spec.DatabaseType.Type == "mysql" {
		reqLogger.Info("Reconciling MattermostCluster Database MySQL service account")
		err = r.checkDBServiceAccount(mattermost, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}

		reqLogger.Info("Reconciling MattermostCluster Database MySQL role binding")
		err = r.checkDBRoleBinding(mattermost, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}

		reqLogger.Info("Reconciling MattermostCluster Database MySQL")
		err = r.checkDBMySQLDeployment(mattermost, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else {
		reqLogger.Info("Reconciling MattermostCluster Database Postgres")
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

func (r *ReconcileMattermostCluster) setDefaults(mattermost *mattermostv1alpha1.MattermostCluster, reqLogger logr.Logger) error {
	reqLogger.Info("Checking defaults")
	reqLogger.Info(mattermost.Spec.IngressName)
	if mattermost.Spec.IngressName == "" {
		return fmt.Errorf("need to set the IngressName")
	}

	if len(mattermost.Spec.Image) == 0 {
		reqLogger.Info("Setting default Mattermost image: " + constants.DefaultMattermostImage)
		mattermost.Spec.Image = constants.DefaultMattermostImage
	}

	if mattermost.Spec.Replicas == 0 {
		reqLogger.Info("Setting default Mattermost replicas: " + strconv.Itoa(constants.DefaultAmountOfPods))
		mattermost.Spec.Replicas = constants.DefaultAmountOfPods
	}

	if len(mattermost.Spec.DatabaseType.Type) == 0 {
		reqLogger.Info("Setting default Mattermost database type: " + constants.DefaultMattermostDatabaseType)
		mattermost.Spec.DatabaseType.Type = constants.DefaultMattermostDatabaseType
	}

	return nil
}

func (r *ReconcileMattermostCluster) checkDBServiceAccount(mattermost *mattermostv1alpha1.MattermostCluster, reqLogger logr.Logger) error {

	// Service Account for Mysql Oracle Operator
	serviceAccountName := "mysql-agent"
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    map[string]string{constants.ClusterLabel: mattermost.Name},
			Name:      serviceAccountName,
			Namespace: mattermost.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   mattermostv1alpha1.SchemeGroupVersion.Group,
					Version: mattermostv1alpha1.SchemeGroupVersion.Version,
					Kind:    "MattermostCluster",
				}),
			},
		},
	}

	foundServiceAccount := &corev1.ServiceAccount{}
	errGet := r.client.Get(context.TODO(), types.NamespacedName{Name: serviceAccountName, Namespace: mattermost.Namespace}, foundServiceAccount)
	if errGet != nil && errors.IsNotFound(errGet) {
		reqLogger.Info("MattermostCluster Database Service account not found, will deploy one.", "Err", errGet.Error())
		reqLogger.Info("Creating Service Account")
		if err := r.client.Create(context.TODO(), serviceAccount); err != nil {
			reqLogger.Info("Error creating Service Account", "Error", err.Error())
			return err
		}
		// TODO add logic to wait for the completion
		reqLogger.Info("Creating Database Service Account completed")
		if err := controllerutil.SetControllerReference(mattermost, serviceAccount, r.scheme); err != nil {
			return err
		}
	} else if errGet != nil {
		reqLogger.Error(errGet, "MattermostCluster Database Service Account")
		return errGet
	}

	// TODO check how to update without creating new tokens
	// Set MattermostService instance as the owner and controller
	// if !reflect.DeepEqual(serviceAccount, foundServiceAccount) {
	// 	foundServiceAccount = serviceAccount
	// 	reqLogger.Info("Updating Service Account", serviceAccount.Namespace, serviceAccount.Name)
	// 	err = r.client.Update(context.TODO(), foundServiceAccount)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	_ = controllerutil.SetControllerReference(mattermost, foundServiceAccount, r.scheme)
	// }

	return nil
}

func (r *ReconcileMattermostCluster) checkDBRoleBinding(mattermost *mattermostv1alpha1.MattermostCluster, reqLogger logr.Logger) error {

	// Role Biding for Mysql Oracle Operator
	serviceAccountName := "mysql-agent"
	roleBinding := &rbacv1beta1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    map[string]string{constants.ClusterLabel: mattermost.Name},
			Name:      serviceAccountName,
			Namespace: mattermost.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   mattermostv1alpha1.SchemeGroupVersion.Group,
					Version: mattermostv1alpha1.SchemeGroupVersion.Version,
					Kind:    "MattermostCluster",
				}),
			},
		},
		Subjects: []rbacv1beta1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccountName,
				Namespace: mattermost.Namespace,
			},
		},
		RoleRef: rbacv1beta1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     serviceAccountName,
		},
	}

	foundRoleBinding := &rbacv1beta1.RoleBinding{}
	errGet := r.client.Get(context.TODO(), types.NamespacedName{Name: serviceAccountName, Namespace: mattermost.Namespace}, foundRoleBinding)
	if errGet != nil && errors.IsNotFound(errGet) {
		reqLogger.Info("MattermostCluster Database Role Binding not found, will deploy one.", "Err", errGet.Error())
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
		reqLogger.Error(errGet, "MattermostCluster Database Role Binding")
		return errGet
	}

	// TODO check how to update
	// Set MattermostService instance as the owner and controller
	// if !reflect.DeepEqual(roleBinding, foundRoleBinding) {
	// 	foundRoleBinding = roleBinding
	// 	reqLogger.Info("Updating Role Binding", roleBinding.Namespace, roleBinding.Name)
	// 	err = r.client.Update(context.TODO(), foundRoleBinding)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	_ = controllerutil.SetControllerReference(mattermost, foundRoleBinding, r.scheme)
	// }

	return nil
}

func (r *ReconcileMattermostCluster) checkDBMySQLDeployment(mattermost *mattermostv1alpha1.MattermostCluster, reqLogger logr.Logger) error {
	reqLogger.Info("MattermostCluster Database using Oracle Mysql Operator")
	dbName := fmt.Sprintf("%s-db", mattermost.Name)

	deployDB := &mysqlOperator.Cluster{}
	deployDB.SetName(dbName)
	deployDB.SetNamespace(mattermost.Namespace)
	deployDB.Spec.Members = 2
	deployDB.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
		*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
			Group:   mattermostv1alpha1.SchemeGroupVersion.Group,
			Version: mattermostv1alpha1.SchemeGroupVersion.Version,
			Kind:    "MattermostCluster",
		}),
	}
	foundDB := &mysqlOperator.Cluster{}
	errGet := r.client.Get(context.TODO(), types.NamespacedName{Name: dbName, Namespace: mattermost.Namespace}, foundDB)
	if errGet != nil && errors.IsNotFound(errGet) {
		reqLogger.Info("MattermostCluster Database not found, will deploy one.", "Err", errGet.Error())
		reqLogger.Info("Creating Database", "Members", deployDB.Spec.Members)
		if err := r.client.Create(context.TODO(), deployDB); err != nil {
			reqLogger.Info("Error creating Database", "Error", err.Error())
			return err
		}
		// TODO add logic to wait for the completion
		reqLogger.Info("Creating Database completed")
		if err := controllerutil.SetControllerReference(mattermost, deployDB, r.scheme); err != nil {
			return err
		}
	} else if errGet != nil {
		reqLogger.Error(errGet, "MattermostCluster Database")
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

func (r *ReconcileMattermostCluster) checkDBPostgresDeployment(mattermost *mattermostv1alpha1.MattermostCluster, reqLogger logr.Logger) error {
	dbExist := false

	//TODO: Create the logic to check if the DB Postgres already exist or changed otherwise create
	if dbExist {
		return r.client.Update(context.TODO(), mattermost)
	}
	return nil
}

func (r *ReconcileMattermostCluster) checkMinioDeployment(mattermost *mattermostv1alpha1.MattermostCluster, reqLogger logr.Logger) error {
	minioExist := false

	//TODO: Create the logic to check if the Minio already exist or changed otherwise create
	if minioExist {
		return r.client.Update(context.TODO(), mattermost)
	}
	return nil
}

func (r *ReconcileMattermostCluster) getSecrets(mattermost *mattermostv1alpha1.MattermostCluster, reqLogger logr.Logger) (string, error) {
	dbSecretName := fmt.Sprintf("%s-db-root-password", mattermost.Name)
	dbSecret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: dbSecretName, Namespace: mattermost.Namespace}, dbSecret)
	if err != nil {
		return "", err
	}
	return string(dbSecret.Data["password"]), nil
}

func (r *ReconcileMattermostCluster) checkMattermostDeployment(mattermost *mattermostv1alpha1.MattermostCluster, reqLogger logr.Logger) error {
	//TODO: Create the logic to deploy MM including service and ingress
	err := r.serviceForMM(mattermost, reqLogger)
	if err != nil {
		return err
	}

	err = r.ingressForMM(mattermost, reqLogger)
	if err != nil {
		return err
	}

	err = r.deploymentForMM(mattermost, reqLogger)
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileMattermostCluster) serviceForMM(mattermost *mattermostv1alpha1.MattermostCluster, reqLogger logr.Logger) error {
	mattermostPort := corev1.ServicePort{Port: 8065}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    map[string]string{constants.ClusterLabel: mattermost.Name},
			Name:      mattermost.Name,
			Namespace: mattermost.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   mattermostv1alpha1.SchemeGroupVersion.Group,
					Version: mattermostv1alpha1.SchemeGroupVersion.Version,
					Kind:    "MattermostCluster",
				}),
			},
			Annotations: map[string]string{
				"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{mattermostPort},
			Selector: map[string]string{
				constants.ClusterLabel: mattermost.Name,
			},
			ClusterIP: corev1.ClusterIPNone,
		},
	}

	foundService := &corev1.Service{}
	errGet := r.client.Get(context.TODO(), types.NamespacedName{Name: mattermost.Name, Namespace: mattermost.Namespace}, foundService)
	if errGet != nil && errors.IsNotFound(errGet) {
		reqLogger.Info("MattermostCluster Service not found, will deploy one.", "Err", errGet.Error())
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
		reqLogger.Error(errGet, "MattermostCluster Service")
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

func (r *ReconcileMattermostCluster) ingressForMM(mattermost *mattermostv1alpha1.MattermostCluster, reqLogger logr.Logger) error {
	ingressName := mattermost.Name + "-ingress"

	spec := v1beta1.IngressSpec{}
	backend := v1beta1.IngressBackend{
		ServiceName: mattermost.Name,
		ServicePort: intstr.FromInt(8065),
	}
	rules := v1beta1.IngressRule{
		Host: mattermost.Spec.IngressName,
		IngressRuleValue: v1beta1.IngressRuleValue{
			HTTP: &v1beta1.HTTPIngressRuleValue{
				Paths: []v1beta1.HTTPIngressPath{
					{
						Path:    "/",
						Backend: backend,
					},
				},
			},
		},
	}

	spec.Rules = append(spec.Rules, rules)
	ingressMM := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressName,
			Namespace: mattermost.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   mattermostv1alpha1.SchemeGroupVersion.Group,
					Version: mattermostv1alpha1.SchemeGroupVersion.Version,
					Kind:    "MattermostCluster",
				}),
			},
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "nginx",
				//"kubernetes.io/tls-acme":      "true",
			},
		},
		Spec: spec,
	}

	foundIngress := &v1beta1.Ingress{}
	errGet := r.client.Get(context.TODO(), types.NamespacedName{Name: ingressName, Namespace: mattermost.Namespace}, foundIngress)
	if errGet != nil && errors.IsNotFound(errGet) {
		reqLogger.Info("MattermostCluster Ingress not found, will deploy one.", "Err", errGet.Error())
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
		reqLogger.Error(errGet, "MattermostCluster Ingress")
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

func (r *ReconcileMattermostCluster) deploymentForMM(mattermost *mattermostv1alpha1.MattermostCluster, reqLogger logr.Logger) error {
	dbPassword, err := r.getSecrets(mattermost, reqLogger)
	if err != nil {
		return fmt.Errorf("Error getting the database password. Err=%s", err.Error())
	}
	datasourceMM := fmt.Sprintf("mysql://root:%s@tcp(%s-db.%s:3306)/mattermost?charset=utf8mb4,utf8&readTimeout=30s&writeTimeout=30s", dbPassword, mattermost.Name, mattermost.Namespace)
	initContainerCMD := fmt.Sprintf("until curl --max-time 5 http://%s-db.%s:3306; do echo waiting for mysql; sleep 5; done;", mattermost.Name, mattermost.Namespace)
	cmdInit := []string{"sh", "-c"}
	cmdInit = append(cmdInit, initContainerCMD)
	cmdStartMM := []string{"mattermost"} //, "--config", datasourceMM}

	cmdInitDB := []string{"sh", "-c"}
	cmdInitDB = append(cmdInitDB, fmt.Sprintf("mysql -h %s-db.%s -u root -p%s -e 'CREATE DATABASE IF NOT EXISTS mattermost'", mattermost.Name, mattermost.Namespace, dbPassword))

	deployMM := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mattermost.Name,
			Namespace: mattermost.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   mattermostv1alpha1.SchemeGroupVersion.Group,
					Version: mattermostv1alpha1.SchemeGroupVersion.Version,
					Kind:    "MattermostCluster",
				}),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &mattermost.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{constants.ClusterLabel: mattermost.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{constants.ClusterLabel: mattermost.Name},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Image:   "appropriate/curl:latest",
							Name:    "init-mysql",
							Command: cmdInit,
						},
						{
							Image:   "mysql:8.0.11",
							Name:    "init-mysql-database",
							Command: cmdInitDB,
						},
					},
					Containers: []corev1.Container{
						{
							Image:   "mattermost/mattermost-enterprise-edition:latest",
							Name:    mattermost.Name,
							Command: cmdStartMM,
							Env: []corev1.EnvVar{
								{
									Name:  "MM_CONFIG",
									Value: datasourceMM,
								},
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8065,
									Name:          mattermost.Name,
								},
							},
						},
					},
				},
			},
		},
	}

	foundMM := &appsv1.Deployment{}
	errGet := r.client.Get(context.TODO(), types.NamespacedName{Name: mattermost.Name, Namespace: mattermost.Namespace}, foundMM)
	if errGet != nil && errors.IsNotFound(errGet) {
		reqLogger.Info("MattermostCluster App not found, will deploy one.", "Err", errGet.Error())
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
		reqLogger.Error(errGet, "MattermostCluster Application")
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
