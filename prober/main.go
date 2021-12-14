package main

import (
	"context"
	"fmt"
	"os"
	"reflect"

	"github.com/ibm/cassandra-operator/prober/logger"

	"github.com/ibm/cassandra-operator/prober/config"
	"github.com/ibm/cassandra-operator/prober/prober"

	"github.com/ibm/cassandra-operator/prober/jolokia"
	"go.uber.org/zap/zapcore"

	"github.com/caarlos0/env/v6"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	Version = "undefined"
)

func main() {
	var cfg config.Config
	err := env.ParseWithFuncs(&cfg, map[reflect.Type]env.ParserFunc{reflect.TypeOf(zapcore.DebugLevel): config.LevelParser})
	if err != nil {
		fmt.Printf("unable to read configs: %s", err.Error())
		os.Exit(1)
	}

	logr, err := logger.NewLogger(cfg.LogFormat, cfg.LogLevel)
	if err != nil {
		fmt.Printf("unable to create logger: %s", err.Error())
		os.Exit(1)
	}

	logr = logr.With("version", Version)

	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		logr.Error(err, "Unable to get InCluster config")
		os.Exit(1)
	}

	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		logr.Error(err, "Unable to create kube client")
		os.Exit(1)
	}

	authSecret, err := kubeClient.CoreV1().Secrets(cfg.PodNamespace).Get(context.Background(), cfg.AdminRoleSecretName, v1.GetOptions{})
	if err != nil {
		logr.Error(err, "unable to get auth secret")
		os.Exit(1)
	}

	username := string(authSecret.Data["admin-role"])
	password := string(authSecret.Data["admin-password"])
	auth := prober.UserAuth{
		User:     username,
		Password: password,
	}

	jolokiaClient := jolokia.NewClient(cfg.JolokiaPort, cfg.JmxPort, cfg.JmxPollingInterval, logr, username, password)

	proberApp := prober.NewProber(cfg, jolokiaClient, auth, kubeClient, logr)

	err = proberApp.Run()
	if err != nil {
		logr.Error(err, "failed to start prober")
	}
}
