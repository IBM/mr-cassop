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

package integration

import (
	"context"
	"github.com/go-logr/zapr"
	"github.com/gocql/gocql"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers"
	"github.com/ibm/cassandra-operator/controllers/config"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/nodetool"
	"github.com/ibm/cassandra-operator/controllers/prober"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sync"
	"testing"
	"time"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var shutdown = false                //to track if the shutdown process has been started. Used for graceful shutdown
var waitGroup = &sync.WaitGroup{}   //waits until the reconcile loops finish. Used to gracefully shutdown the environment
var mgrStopCh = make(chan struct{}) //stops the manager by sending a value to the channel
var mockProberClient = &proberMock{}
var mockNodetoolClient = &nodetoolMock{}
var mockCQLClient = &cqlMock{}
var operatorConfig = config.Config{}
var ctx = context.Background()

var (
	cassandraObjectMeta = metav1.ObjectMeta{
		Namespace: "default",
		Name:      "test-cassandra-cluster",
	}
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	var err error

	logr := zap.NewNop()
	//logr, err = zap.NewDevelopment() //uncomment if you want to see operator logs
	Expect(err).ToNot(HaveOccurred())
	ctrl.SetLogger(zapr.NewLogger(logr))
	logf.SetLogger(zapr.NewLogger(logr))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")},
	}

	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = dbv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	operatorConfig = config.Config{
		Namespace:             "default",
		RetryDelay:            time.Millisecond * 50,
		DefaultCassandraImage: "cassandra/image",
		DefaultProberImage:    "prober/image",
		DefaultJolokiaImage:   "jolokia/image",
		DefaultKwatcherImage:  "kwatcher/image",
	}
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	createOperatorConfigMaps()

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(mgr).ToNot(BeNil())

	cassandraCtrl := &controllers.CassandraClusterReconciler{
		Log:    logr.Sugar(),
		Scheme: scheme.Scheme,
		Client: k8sClient,
		Cfg:    operatorConfig,
		ProberClient: func(host string) prober.Client {
			return mockProberClient
		},
		CqlClient: func(clusterConfig *gocql.ClusterConfig) (cql.Client, error) {
			return mockCQLClient, nil
		},
		NodetoolClient: func(clientset *kubernetes.Clientset, config *rest.Config) nodetool.Client {
			return mockNodetoolClient
		},
		RESTConfig: cfg,
	}

	testReconciler := SetupTestReconcile(cassandraCtrl)
	err = controllers.SetupCassandraReconciler(testReconciler, mgr, nil)
	Expect(err).ToNot(HaveOccurred())

	mgrStopCh = StartTestManager(mgr)

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	shutdown = true
	close(mgrStopCh)      //tell the manager to shutdown
	waitGroup.Wait()      //wait for all reconcile loops to be finished
	err := testEnv.Stop() //stop the test control plane (etcd, kube-apiserver)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterEach(func() {
	CleanUpCreatedResources(cassandraObjectMeta.Name, cassandraObjectMeta.Namespace)
	mockProberClient = &proberMock{}
	mockNodetoolClient = &nodetoolMock{}
	mockCQLClient = &cqlMock{}
})

func SetupTestReconcile(inner reconcile.Reconciler) reconcile.Reconciler {
	fn := reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) {
		if shutdown { // do not start reconcile events if we're shutting down. Otherwise those events will fail because of shut down apiserver
			return reconcile.Result{}, nil
		}
		waitGroup.Add(1) //makes sure the in flight reconcile events are handled gracefully
		result, err := inner.Reconcile(req)
		waitGroup.Done()
		return result, err
	})
	return fn
}

func StartTestManager(mgr manager.Manager) chan struct{} {
	stop := make(chan struct{})
	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(stop)).NotTo(HaveOccurred())
	}()
	return stop
}

func createOperatorConfigMaps() {
	scriptsCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.OperatorScriptsCM(),
			Namespace: operatorConfig.Namespace,
		},
	}
	proberSourcesCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.OperatorProberSourcesCM(),
			Namespace: operatorConfig.Namespace,
		},
	}
	cassConfigCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.OperatorCassandraConfigCM(),
			Namespace: operatorConfig.Namespace,
		},
		Data: map[string]string{
			"cassandra.yaml": "",
		},
	}
	shiroConfigCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.OperatorShiroCM(),
			Namespace: operatorConfig.Namespace,
		},
	}

	Expect(k8sClient.Create(ctx, scriptsCM)).To(Succeed())
	Expect(k8sClient.Create(ctx, proberSourcesCM)).To(Succeed())
	Expect(k8sClient.Create(ctx, cassConfigCM)).To(Succeed())
	Expect(k8sClient.Create(ctx, shiroConfigCM)).To(Succeed())
}

// As the test control plane doesn't support garbage collection, this function is used to clean up resources
// Designed to not fail if the resource is not found
func CleanUpCreatedResources(ccName, ccNamespace string) {
	haveNoErrorOrNotFoundError := Or(BeNil(), BeAssignableToTypeOf(&errors.StatusError{}))
	cc := &dbv1alpha1.CassandraCluster{}
	var err error
	hasCassandraLabel := client.HasLabels{dbv1alpha1.CassandraClusterInstance}

	err = k8sClient.Get(ctx, types.NamespacedName{Name: ccName, Namespace: ccNamespace}, cc)
	Expect(err).To(haveNoErrorOrNotFoundError)

	err = k8sClient.DeleteAllOf(context.Background(), &dbv1alpha1.CassandraCluster{}, client.InNamespace(ccNamespace))
	Expect(err).To(BeNil())

	err = k8sClient.Delete(context.Background(), &v1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: ccNamespace, Name: names.ProberService(cc)}})
	Expect(err).To(haveNoErrorOrNotFoundError)

	for _, dc := range cc.Spec.DCs {
		err = k8sClient.Delete(context.Background(), &v1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: ccNamespace, Name: names.DCService(cc, dc.Name)}})
		Expect(err).To(haveNoErrorOrNotFoundError)
	}

	err = k8sClient.DeleteAllOf(context.Background(), &apps.StatefulSet{}, client.InNamespace(ccNamespace), hasCassandraLabel)
	Expect(err).To(BeNil())

	err = k8sClient.DeleteAllOf(context.Background(), &apps.Deployment{}, client.InNamespace(ccNamespace), hasCassandraLabel)
	Expect(err).To(BeNil())

	err = k8sClient.DeleteAllOf(context.Background(), &rbac.Role{}, client.InNamespace(ccNamespace), hasCassandraLabel)
	Expect(err).To(BeNil())
	err = k8sClient.DeleteAllOf(context.Background(), &rbac.RoleBinding{}, client.InNamespace(ccNamespace), hasCassandraLabel)
	Expect(err).To(BeNil())

	err = k8sClient.DeleteAllOf(context.Background(), &v1.ServiceAccount{}, client.InNamespace(ccNamespace), client.HasLabels{dbv1alpha1.CassandraClusterInstance}, hasCassandraLabel)
	Expect(err).To(BeNil())

	err = k8sClient.DeleteAllOf(context.Background(), &v1.ConfigMap{}, client.InNamespace(ccNamespace), hasCassandraLabel)
	Expect(err).To(BeNil())

	err = k8sClient.DeleteAllOf(context.Background(), &v1.Secret{}, client.InNamespace(ccNamespace), hasCassandraLabel)
	Expect(err).To(haveNoErrorOrNotFoundError)
}

func getContainerByName(pod v1.PodSpec, containerName string) (v1.Container, bool) {
	for _, container := range pod.Containers {
		if container.Name == containerName {
			return container, true
		}
	}

	return v1.Container{}, false
}
