package main

import (
	"flag"
	"fmt"
	"github.com/mattermost/mattermost-operator/controllers/mattermost/clusterinstallation"
	v1beta1Minio "github.com/minio/minio-operator/pkg/apis/miniocontroller/v1beta1"
	"os"
	"runtime"

	blubr "github.com/mattermost/blubr"
	mattermostcomv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	"github.com/mattermost/mattermost-operator/controllers/mattermost/mattermostrestoredb"
	v1alpha1MySQL "github.com/presslabs/mysql-operator/pkg/apis/mysql/v1alpha1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	scheme = k8sruntime.NewScheme()
)

// Change below variables to serve metrics on different host or port.
// Or use "metrics-addr" flag
var (
	// metricsHost specifies host to bind to for serving prometheus metrics
	metricsHost = "0.0.0.0"
	// metricsPort specifies port to bind to for serving prometheus metrics
	metricsPort int32 = 8383
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(mattermostcomv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme

	utilruntime.Must(v1beta1Minio.AddToScheme(scheme))
	utilruntime.Must(v1alpha1MySQL.SchemeBuilder.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", fmt.Sprintf("%s:%d", metricsHost, metricsPort), "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Setup logging.
	// This logger wraps logrus in a 'logr.Logger' interface. This is required
	// for the deferred logging required by the various operator packages.
	logger := blubr.InitLogger()
	logger = logger.WithName("opr")
	logf.SetLogger(logger)

	logger.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	logger.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "b78a986e.mattermost.com",
	})
	if err != nil {
		logger.Error(err, "Unable to start manager")
		os.Exit(1)
	}

	logger.Info("Registering Components")

	if err = (&clusterinstallation.ClusterInstallationReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("ClusterInstallation"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		logger.Error(err, "Unable to create controller", "controller", "ClusterInstallation")
		os.Exit(1)
	}
	if err = (&mattermostrestoredb.MattermostRestoreDBReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("MattermostRestoreDB"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		logger.Error(err, "Unable to create controller", "controller", "MattermostRestoreDB")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	logger.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logger.Error(err, "Problem running manager")
		os.Exit(1)
	}
}
