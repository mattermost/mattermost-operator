package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/go-logr/logr"
	blubr "github.com/mattermost/blubr"
	"github.com/mattermost/mattermost-operator/controllers/mattermost/clusterinstallation"
	"github.com/mattermost/mattermost-operator/controllers/mattermost/mattermost"
	"github.com/mattermost/mattermost-operator/controllers/mattermost/mattermostrestoredb"
	mysqlv1alpha1 "github.com/mattermost/mattermost-operator/pkg/database/mysql_operator/v1alpha1"
	"github.com/mattermost/mattermost-operator/pkg/resources"
	v1beta1Minio "github.com/minio/minio-operator/pkg/apis/miniocontroller/v1beta1"
	"github.com/sirupsen/logrus"
	"github.com/vrischmann/envconfig"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	mattermostcomv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
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
	utilruntime.Must(mmv1beta.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme

	utilruntime.Must(v1beta1Minio.AddToScheme(scheme))
	utilruntime.Must(mysqlv1alpha1.SchemeBuilder.AddToScheme(scheme))
}

type Config struct {
	MaxReconcilingInstallations int           `envconfig:"default=20"`
	RequeueOnLimitDelay         time.Duration `envconfig:"default=20s"`
	MaxReconcileConcurrency     int           `envconfig:"default=1"`
}

func (c Config) String() string {
	return fmt.Sprintf(
		"MaxReconcilingInstallations=%d RequeueOnLimitDelay=%s MaxReconcileConcurrency=%d",
		c.MaxReconcilingInstallations, c.RequeueOnLimitDelay, c.MaxReconcileConcurrency,
	)
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", fmt.Sprintf("%s:%d", metricsHost, metricsPort), "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	// Setup logging.
	// This logger wraps logrus in a 'logr.Logger' interface. This is required
	// for the deferred logging required by the various operator packages.
	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logSink = logSink.WithName("opr")

	logger := logr.New(logSink)

	logf.SetLogger(logger)
	ctrl.SetLogger(logger)

	logger.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	logger.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))

	var config Config
	err := envconfig.Init(&config)
	if err != nil {
		logger.Error(err, "Unable to read environment configuration")
		os.Exit(1)
	}
	logger.Info("Loaded Operator env config", "config", config.String())

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		LeaderElection:   enableLeaderElection,
		LeaderElectionID: "b78a986e.mattermost.com",
	})
	if err != nil {
		logger.Error(err, "Unable to start manager")
		os.Exit(1)
	}

	logger.Info("Registering Components")

	if err = (&clusterinstallation.ClusterInstallationReconciler{
		Client:              mgr.GetClient(),
		NonCachedAPIReader:  mgr.GetAPIReader(),
		Log:                 ctrl.Log.WithName("controllers").WithName("ClusterInstallation"),
		Scheme:              mgr.GetScheme(),
		MaxReconciling:      config.MaxReconcilingInstallations,
		RequeueOnLimitDelay: config.RequeueOnLimitDelay,
		Resources:           resources.NewResourceHelper(mgr.GetClient(), mgr.GetScheme()),
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
	if err = mattermost.NewMattermostReconciler(
		mgr,
		config.MaxReconcilingInstallations,
		config.RequeueOnLimitDelay,
	).
		SetupWithManager(mgr, config.MaxReconcileConcurrency); err != nil {
		logger.Error(err, "Unable to create controller", "controller", "Mattermost")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	logger.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logger.Error(err, "Problem running manager")
		os.Exit(1)
	}
}
