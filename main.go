package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/slices-ri/custom-reflector-operator/controllers"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	var foreignKubeconfigPath string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8082", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8083", "The address the probe endpoint binds to.")
	flag.StringVar(&foreignKubeconfigPath, "foreign-kubeconfig", "/SlicesFile/kubeconfigs/edge1.yaml", "Path to Foreign Cluster Kubeconfig file.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// 1. Home Cluster Manager (Master)
	homeRestConfig := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(homeRestConfig, ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         false,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager on Home cluster")
		os.Exit(1)
	}

	// 2. Build Client for Foreign Cluster (Edge1)
	foreignRestConfig, err := clientcmd.BuildConfigFromFlags("", foreignKubeconfigPath)
	if err != nil {
		// Fallback check local relative path
		foreignRestConfig, err = clientcmd.BuildConfigFromFlags("", "./SlicesFile/kubeconfigs/edge1.yaml")
		if err != nil {
			setupLog.Error(err, "unable to load Foreign cluster kubeconfig", "path", foreignKubeconfigPath)
			os.Exit(1)
		}
	}

	foreignClient, err := client.New(foreignRestConfig, client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "unable to create Foreign cluster client")
		os.Exit(1)
	}

	// 3. Register Reflector Reconciler
	if err = (&controllers.S4TDeviceReflectorReconciler{
		HomeClient:    mgr.GetClient(),
		ForeignClient: foreignClient,
		Scheme:        mgr.GetScheme(),
		Log:           ctrl.Log.WithName("controllers").WithName("S4TDeviceReflector"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "S4TDeviceReflector")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("Starting Custom Reflector Operator (Spec & Status Closed-Loop Reflection)...")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running custom reflector manager")
		os.Exit(1)
	}
}
