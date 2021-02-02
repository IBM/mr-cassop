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
	"flag"
	"fmt"
	"github.com/go-logr/zapr"
	"github.com/gocql/gocql"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers"
	"github.com/ibm/cassandra-operator/controllers/config"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/logger"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/nodetool"
	"github.com/ibm/cassandra-operator/controllers/prober"
	"github.com/ibm/cassandra-operator/controllers/reaper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"net/url"
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
var mockReaperClient = &reaperMock{}
var operatorConfig = config.Config{}
var ctx = context.Background()
var logr = zap.NewNop()
var reconcileInProgress = false
var testFinished = false
var enableOperatorLogs bool

var (
	cassandraObjectMeta = metav1.ObjectMeta{
		Namespace: "default",
		Name:      "test-cassandra-cluster",
	}
)

const (
	longTimeout   = time.Second * 10
	mediumTimeout = time.Second * 5
	shortTimeout  = time.Second * 1

	mediumRetry = time.Second * 2
	shortRetry  = time.Millisecond * 300
)

func init() {
	flag.BoolVar(&enableOperatorLogs, "enableOperatorLogs", false, "set tot true to print operator logs during tests")
}

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	var err error

	if enableOperatorLogs {
		logr = logger.NewLogger("console", zap.DebugLevel).Desugar()
	}

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
		RetryDelay:            time.Second * 1,
		DefaultCassandraImage: "cassandra/image",
		DefaultProberImage:    "prober/image",
		DefaultJolokiaImage:   "jolokia/image",
		DefaultReaperImage:    "reaper/image",
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
		ProberClient: func(url *url.URL) prober.ProberClient {
			return mockProberClient
		},
		CqlClient: func(clusterConfig *gocql.ClusterConfig) (cql.CqlClient, error) {
			return mockCQLClient, nil
		},
		NodetoolClient: func(clientset *kubernetes.Clientset, config *rest.Config) nodetool.NodetoolClient {
			return mockNodetoolClient
		},
		ReaperClient: func(url *url.URL) reaper.ReaperClient {
			return mockReaperClient
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
	testFinished = true      //to not trigger other reconcile events from the queue
	Eventually(func() bool { // to wait until the last reconcile loop finishes
		return reconcileInProgress
	}, mediumTimeout, mediumRetry).Should(BeFalse(), "Test didn't stop triggering reconcile events. See operator logs for more details.")
	CleanUpCreatedResources(cassandraObjectMeta.Name, cassandraObjectMeta.Namespace)
	mockProberClient = &proberMock{}
	mockNodetoolClient = &nodetoolMock{}
	mockCQLClient = &cqlMock{}
	mockReaperClient = &reaperMock{}
	testFinished = false
})

func SetupTestReconcile(inner reconcile.Reconciler) reconcile.Reconciler {
	fn := reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) {
		// do not start reconcile events if we're shutting down. Otherwise those events will fail because of shut down apiserver
		// also do not start reconcile events if the test is finished. Otherwise reconcile events can create resources after cleanup logic
		if shutdown || testFinished {
			return reconcile.Result{}, nil
		}
		reconcileInProgress = true
		waitGroup.Add(1) //makes sure the in flight reconcile events are handled gracefully
		result, err := inner.Reconcile(req)
		reconcileInProgress = false
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
	cc := &dbv1alpha1.CassandraCluster{}

	err := k8sClient.Get(ctx, types.NamespacedName{Name: ccName, Namespace: ccNamespace}, cc)
	if err != nil && errors.IsNotFound(err) {
		return
	}
	Expect(err).ToNot(HaveOccurred())

	// delete cassandracluster separately as there's no guarantee that it'll come first in the for loop
	Expect(deleteResource(types.NamespacedName{Namespace: ccNamespace, Name: ccName}, &dbv1alpha1.CassandraCluster{})).To(Succeed())
	expectResourceIsDeleted(types.NamespacedName{Name: ccName, Namespace: ccNamespace}, &dbv1alpha1.CassandraCluster{})

	cc.Name = ccName
	cc.Namespace = ccNamespace
	type resourceToDelete struct {
		name    string
		objType runtime.Object
	}

	resourcesToDelete := []resourceToDelete{
		{name: names.ProberService(cc.Name), objType: &v1.Service{}},
		{name: names.ProberDeployment(cc.Name), objType: &apps.Deployment{}},
		{name: names.ProberServiceAccount(cc.Name), objType: &v1.ServiceAccount{}},
		{name: names.ProberRole(cc.Name), objType: &rbac.Role{}},
		{name: names.ProberRoleBinding(cc.Name), objType: &rbac.RoleBinding{}},
		{name: names.ProberSources(cc.Name), objType: &v1.ConfigMap{}},
		{name: names.ReaperService(cc.Name), objType: &v1.Service{}},
		{name: names.ReaperCqlConfigMap(cc.Name), objType: &v1.ConfigMap{}},
		{name: names.ShiroConfigMap(cc.Name), objType: &v1.ConfigMap{}},
		{name: names.ScriptsConfigMap(cc.Name), objType: &v1.ConfigMap{}},
		{name: names.MaintenanceConfigMap(cc.Name), objType: &v1.ConfigMap{}},
		{name: names.RepairsConfigMap(cc.Name), objType: &v1.ConfigMap{}},
		{name: names.RolesSecret(cc.Name), objType: &v1.Secret{}},
		{name: names.JMXRemoteSecret(cc.Name), objType: &v1.Secret{}},
	}

	// add DC specific resources
	for _, dc := range cc.Spec.DCs {
		resourcesToDelete = append(resourcesToDelete, resourceToDelete{name: names.DC(cc.Name, dc.Name), objType: &apps.StatefulSet{}})
		resourcesToDelete = append(resourcesToDelete, resourceToDelete{name: names.DCService(cc.Name, dc.Name), objType: &v1.Service{}})
		resourcesToDelete = append(resourcesToDelete, resourceToDelete{name: names.ReaperDeployment(cc.Name, dc.Name), objType: &apps.Deployment{}})
		resourcesToDelete = append(resourcesToDelete, resourceToDelete{name: names.ConfigMap(cc.Name), objType: &v1.ConfigMap{}})
	}

	for _, resource := range resourcesToDelete {
		logr.Debug(fmt.Sprintf("deleting resource %T:%s", resource.objType, resource.name))
		Expect(deleteResource(types.NamespacedName{Namespace: ccNamespace, Name: resource.name}, resource.objType)).To(Succeed())
	}

	for _, resource := range resourcesToDelete {
		expectResourceIsDeleted(types.NamespacedName{Name: resource.name, Namespace: ccNamespace}, resource.objType)
	}
}

func getContainerByName(pod v1.PodSpec, containerName string) (v1.Container, bool) {
	for _, container := range pod.Containers {
		if container.Name == containerName {
			return container, true
		}
	}

	return v1.Container{}, false
}

func expectResourceIsDeleted(name types.NamespacedName, obj runtime.Object) {
	Eventually(func() metav1.StatusReason {
		err := k8sClient.Get(context.Background(), name, obj)
		if err != nil {
			if statusErr, ok := err.(*errors.StatusError); ok {
				return statusErr.ErrStatus.Reason
			}
			return metav1.StatusReason(err.Error())
		}

		return "Found"
	}, longTimeout, mediumRetry).Should(Equal(metav1.StatusReasonNotFound), fmt.Sprintf("%T %s should be deleted", obj, name))
}

func deleteResource(name types.NamespacedName, obj runtime.Object) error {
	err := k8sClient.Get(context.Background(), name, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return k8sClient.Delete(context.Background(), obj)
}

func getVolumeMountByName(volumeMounts []v1.VolumeMount, volumeMountName string) (v1.VolumeMount, bool) {
	for _, volumeMount := range volumeMounts {
		if volumeMount.Name == volumeMountName {
			return volumeMount, true
		}
	}

	return v1.VolumeMount{}, false
}

func getVolumeByName(volumes []v1.Volume, volumeName string) (v1.Volume, bool) {
	for _, volume := range volumes {
		if volume.Name == volumeName {
			return volume, true
		}
	}

	return v1.Volume{}, false
}
