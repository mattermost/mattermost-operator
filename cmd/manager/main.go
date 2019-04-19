package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/mattermost/mattermost-operator/pkg/apis"
	"github.com/mattermost/mattermost-operator/pkg/controller"
	"github.com/mattermost/mattermost-operator/pkg/log"
	"github.com/mattermost/mattermost-operator/version"

	"github.com/operator-framework/operator-sdk/pkg/leader"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	"github.com/operator-framework/operator-sdk/pkg/metrics"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/spf13/pflag"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

// Change below variables to serve metrics on different host or port.
var (
	metricsHost       = "0.0.0.0"
	metricsPort int32 = 8383
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
		Namespace:          "",
		MetricsBindAddress: fmt.Sprintf("%s:%d", metricsHost, metricsPort),
	})
	if err != nil {
		logger.Error(err, "Unable to create manager")
		os.Exit(1)
	}

	logger.Info("Registering Components.")

	// Setup Scheme for all resources
	if errAddToScheme := apis.AddToScheme(mgr.GetScheme()); errAddToScheme != nil {
		logger.Error(errAddToScheme, "Unable to setup scheme")
		os.Exit(1)
	}

	// Setup all Controllers
	if errAddToManager := controller.AddToManager(mgr); errAddToManager != nil {
		logger.Error(errAddToManager, "Unable to setup controllers")
		os.Exit(1)
	}

	// Create Service object to expose the metrics port.
	_, err = metrics.ExposeMetricsPort(ctx, metricsPort)
	if err != nil {
		logger.Info(err.Error())
	}

	logger.Info("Starting the Cmd.")

	// Start the Cmd
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		logger.Error(err, "Manager exited non-zero")
		os.Exit(1)
	}
}
