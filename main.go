/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"github.com/ibm/cassandra-operator/controllers/names"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/go-logr/zapr"
	"github.com/gocql/gocql"
	"github.com/ibm/cassandra-operator/controllers/webhooks"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	operatorCfg "github.com/ibm/cassandra-operator/controllers/config"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/events"
	"github.com/ibm/cassandra-operator/controllers/logger"
	"github.com/ibm/cassandra-operator/controllers/prober"
	"github.com/ibm/cassandra-operator/controllers/reaper"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers"
	// +kubebuilder:scaffold:imports
)

var (
	Version = "undefined"
	scheme  = runtime.NewScheme()

	netTransport = &http.Transport{
		TLSHandshakeTimeout: 5 * time.Second,
	}

	httpClient = &http.Client{
		Transport: netTransport,
		Timeout:   time.Second * 30,
	}
)

const (
	leaderElectionID = "cassandra-operator-leader-election-lock"

	healthCheckBindAddress = 8042
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(dbv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	operatorConfig, err := operatorCfg.LoadConfig()
	if err != nil {
		fmt.Printf("unable to load operator config: %s", err.Error())
		os.Exit(1)
	}

	logr := logger.NewLogger(operatorConfig.LogFormat, operatorConfig.LogLevel)

	logr.Infof("Version: %s", Version)
	logr.Infof("Leader election enabled: %t", operatorConfig.LeaderElectionEnabled)
	logr.Infof("Log level: %s", operatorConfig.LogLevel.String())
	logr.Infof("Prometheus metrics exporter port: %d", operatorConfig.MetricsPort)
	logr.Infof("Health probe bind address: :%d", healthCheckBindAddress)

	logr = logr.With(logger.FieldOperatorVersion, Version)

	ctrl.SetLogger(zapr.NewLogger(logr.Desugar()))

	restCfg := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(restCfg, ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      fmt.Sprintf(":%d", operatorConfig.MetricsPort),
		LeaderElection:          operatorConfig.LeaderElectionEnabled,
		LeaderElectionID:        leaderElectionID,
		LeaderElectionNamespace: operatorConfig.Namespace,
		HealthProbeBindAddress:  fmt.Sprintf(":%d", healthCheckBindAddress),
	})
	if err != nil {
		logr.With(zap.Error(err)).Error("unable to create manager")
		os.Exit(1)
	}

	err = mgr.AddReadyzCheck("ready", healthz.Ping)
	if err != nil {
		logr.With(zap.Error(err)).Error("unable to setup readiness check")
		os.Exit(1)
	}

	eventRecorder := events.NewEventRecorder(mgr.GetEventRecorderFor(events.EventRecorderNameCassandraCluster))

	cassandraReconciler := &controllers.CassandraClusterReconciler{
		Client:       mgr.GetClient(),
		Log:          logr,
		Scheme:       mgr.GetScheme(),
		Cfg:          *operatorConfig,
		Events:       eventRecorder,
		ProberClient: func(url *url.URL) prober.ProberClient { return prober.NewProberClient(url, httpClient) },
		CqlClient:    func(cluster *gocql.ClusterConfig) (cql.CqlClient, error) { return cql.NewCQLClient(cluster) },
		ReaperClient: func(url *url.URL, clusterName string, defaultRepairThreadCount int32) reaper.ReaperClient {
			return reaper.NewReaperClient(url, clusterName, httpClient, defaultRepairThreadCount)
		},
	}
	err = controllers.SetupCassandraReconciler(cassandraReconciler, mgr, logr)
	if err != nil {
		logr.With(zap.Error(err)).Error("unable to create controller", "controller", "CassandraCluster")
		os.Exit(1)
	}

	// We use k8s.io/client-go client due to the fact that we require Create and Update operations.
	// So we can't use sigs.k8s.io/controller-runtime rest client as it requires running manager with cache.
	kubeClient, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		logr.With(zap.Error(err)).Error("failed to create client set")
		os.Exit(1)
	}

	if operatorConfig.WebhooksEnabled {
		logr.Infof("creating webhooks assets...")
		if err = webhooks.CreateWebhookAssets(kubeClient, operatorConfig); err != nil {
			logr.With(zap.Error(err)).Fatal("failed to create webhooks assets")
			os.Exit(1)
		}

		logr.Infof("admission webhooks container port: %d", int(operatorConfig.WebhooksPort))
		mgr.GetWebhookServer().Port = int(operatorConfig.WebhooksPort)
		mgr.GetWebhookServer().CertDir = names.OperatorWebhookTLSDir()
		if err = (&dbv1alpha1.CassandraCluster{}).SetupWebhookWithManager(mgr); err != nil {
			logr.With(zap.Error(err)).Fatal("failed to setup webhook with manager")
			os.Exit(1)
		}
		dbv1alpha1.SetWebhookLogger(logr)
	} else {
		logr.Infof("deleting webhooks assests if they exist")
		if err = webhooks.DeleteWebhookAssets(kubeClient, operatorConfig); err != nil {
			logr.With(zap.Error(err)).Fatal("failed to delete webhooks assets")
			os.Exit(1)
		}
	}

	logr.Info("Starting manager")
	if err = mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logr.With(zap.Error(err)).Error("problem running manager")
		os.Exit(1)
	}
}
