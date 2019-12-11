package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/mattermost/mattermost-operator/pkg/apis"
	"github.com/mattermost/mattermost-operator/pkg/controller"
	"github.com/mattermost/mattermost-operator/pkg/log"
	"github.com/mattermost/mattermost-operator/version"

	"github.com/operator-framework/operator-sdk/pkg/leader"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	"github.com/operator-framework/operator-sdk/pkg/metrics"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/spf13/pflag"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

// Change below variables to serve metrics on different host or port.
var (
	// metricsHost specifies host to bind to for serving prometheus metrics
	metricsHost = "0.0.0.0"
	// metricsPort specifies port to bind to for serving prometheus metrics
	metricsPort int32 = 8383
	// syncPeriod specifies the period to reconcile resources in order to reduce spread between local cache and k8s
	syncPeriod = time.Duration(180 * time.Second)
)

func main() {
	// Add the zap logger flag set to the CLI. The flag set must
	// be added before calling pflag.Parse().
	pflag.CommandLine.AddFlagSet(zap.FlagSet())

	// Add flags registered by imported packages (e.g. glog and
	// controller-runtime)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Parse()

	// Setup logging.
	// This logger wraps logrus in a 'logr.Logger' interface. This is required
	// for the deferred logging required by the various operator packages.
	logger := log.InitLogger()
	logger = logger.WithName("opr")
	logf.SetLogger(logger)

	logger.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	logger.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	logger.Info(fmt.Sprintf("operator-sdk Version: %v", sdkVersion.Version))
	logger.Info(version.GetVersionString())

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		logger.Error(err, "Unable to get config")
		os.Exit(1)
	}

	ctx := context.TODO()

	// Become the leader before proceeding
	err = leader.Become(ctx, "mattermost-operator-lock")
	if err != nil {
		logger.Error(err, "Unable to become leader")
		os.Exit(1)
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, manager.Options{
		SyncPeriod:         &syncPeriod,
		Namespace:          "",
		MetricsBindAddress: fmt.Sprintf("%s:%d", metricsHost, metricsPort),
	})
	if err != nil {
		logger.Error(err, "Unable to create manager")
		os.Exit(1)
	}

	logger.Info("Registering Components.")

	// Setup Scheme for all resources
	err = apis.AddToScheme(mgr.GetScheme())
	if err != nil {
		logger.Error(err, "Unable to setup scheme")
		os.Exit(1)
	}

	// Setup all Controllers
	err = controller.AddToManager(mgr)
	if err != nil {
		logger.Error(err, "Unable to setup controllers")
		os.Exit(1)
	}

	// Add to the below struct any other metrics ports you want to expose.
	servicePorts := []v1.ServicePort{
		{Port: metricsPort, Name: metrics.OperatorPortName, Protocol: v1.ProtocolTCP, TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: metricsPort}},
	}

	// Create Service object to expose the metrics port.
	_, err = metrics.CreateMetricsService(context.TODO(), cfg, servicePorts)
	if err != nil {
		logger.Info(err.Error())
	}

	logger.Info("Starting the Cmd.")

	// Start the Cmd
	err = mgr.Start(signals.SetupSignalHandler())
	if err != nil {
		logger.Error(err, "Manager exited non-zero")
		os.Exit(1)
	}
}
