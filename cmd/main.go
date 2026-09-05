package main

import (
	"flag"
	"os"

	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/hesam-fattahi/airo/api/v1alpha1"
	"github.com/hesam-fattahi/airo/internal/attribution"
	"github.com/hesam-fattahi/airo/internal/controller"
	"github.com/hesam-fattahi/airo/internal/remediation"
	"github.com/hesam-fattahi/airo/internal/telemetry"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(discoveryv1.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var prometheusURL string

	flag.StringVar(
		&metricsAddr,
		"metrics-bind-address",
		":8081",
		"The address the metrics endpoint binds to.",
	)

	flag.StringVar(
		&prometheusURL,
		"prometheus-url",
		"http://prometheus.monitoring.svc:9090",
		"Prometheus HTTP API URL",
	)

	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	setupLog.Info("Starting AIRO - Autonomous Incident Remediation Operator")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}

	telemetryClient, err := telemetry.NewPrometheusClient(prometheusURL)
	if err != nil {
		setupLog.Error(err, "unable to initialize Prometheus telemetry client")
		os.Exit(1)
	}

	reconciler := &controller.RemediationPolicyReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		Recorder:          mgr.GetEventRecorderFor("airo-operator"),
		TelemetryClient:   telemetryClient,
		Isolator:          remediation.NewIsolator(mgr.GetClient()),
		Executor:          remediation.NewExecutor(mgr.GetClient()),
		AttributionEngine: attribution.NewAttributionEngine(10.0, 0.30),
	}

	if err := reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(
			err,
			"unable to create controller",
			"controller",
			"RemediationPolicy",
		)
		os.Exit(1)
	}

	setupLog.Info(
		"Starting controller manager loop",
		"metricsAddress",
		metricsAddr,
		"prometheusURL",
		prometheusURL,
	)

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
