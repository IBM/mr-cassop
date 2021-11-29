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
	"net/url"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	"github.com/gocql/gocql"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers"
	"github.com/ibm/cassandra-operator/controllers/config"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/events"
	"github.com/ibm/cassandra-operator/controllers/logger"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/prober"
	"github.com/ibm/cassandra-operator/controllers/reaper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

	reaperDeploymentLabels = map[string]string{
		v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentReaper,
		v1alpha1.CassandraClusterInstance:  cassandraObjectMeta.Name,
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
	flag.BoolVar(&enableOperatorLogs, "enableOperatorLogs", false, "set to true to print operator logs during tests")
}

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
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

	err = v1alpha1.AddToScheme(scheme.Scheme)
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
		Events: events.NewEventRecorder(&record.FakeRecorder{}),
		ProberClient: func(url *url.URL) prober.ProberClient {
			return mockProberClient
		},
		CqlClient: func(clusterConfig *gocql.ClusterConfig) (cql.CqlClient, error) {
			authenticator := clusterConfig.Authenticator.(*gocql.PasswordAuthenticator)
			roles := mockCQLClient.cassandraRoles
			for _, role := range roles {
				if role.Role == authenticator.Username {
					if role.Password != authenticator.Password {
						return nil, errors.New("password is incorrect")
					}
					if !role.Login {
						return nil, errors.New("user in not allowed to log in")
					}
					return mockCQLClient, nil
				}
			}

			return nil, errors.New("user not found")
		},
		ReaperClient: func(url *url.URL, clusterName string) reaper.ReaperClient {
			return mockReaperClient
		},
	}

	testReconciler := SetupTestReconcile(cassandraCtrl)
	err = controllers.SetupCassandraReconciler(testReconciler, mgr, zap.NewNop().Sugar())
	Expect(err).ToNot(HaveOccurred())

	mgrStopCh = StartTestManager(mgr)
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	shutdown = true
	close(mgrStopCh) //tell the manager to shutdown
	waitGroup.Wait() //wait for all reconcile loops to be finished
	//only log the error until https://github.com/kubernetes-sigs/controller-runtime/issues/1571 is resolved
	err := testEnv.Stop() //stop the test control plane (etcd, kube-apiserver)
	if err != nil {
		logr.Warn(fmt.Sprintf("Failed to stop testenv properly: %#v", err))
	}
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
	fn := reconcile.Func(func(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
		// do not start reconcile events if we're shutting down. Otherwise those events will fail because of shut down apiserver
		// also do not start reconcile events if the test is finished. Otherwise reconcile events can create resources after cleanup logic
		if shutdown || testFinished {
			return reconcile.Result{}, nil
		}
		reconcileInProgress = true
		waitGroup.Add(1) //makes sure the in flight reconcile events are handled gracefully
		result, err := inner.Reconcile(ctx, req)
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
		Expect(mgr.Start(ctx)).To(BeNil())
	}()
	return stop
}

func createOperatorConfigMaps() {
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

	prometheusCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.OperatorPrometheusConfigCM(),
			Namespace: operatorConfig.Namespace,
		},
	}

	Expect(k8sClient.Create(ctx, cassConfigCM)).To(Succeed())
	Expect(k8sClient.Create(ctx, shiroConfigCM)).To(Succeed())
	Expect(k8sClient.Create(ctx, prometheusCM)).To(Succeed())
}

// As the test control plane doesn't support garbage collection, this function is used to clean up resources
// Designed to not fail if the resource is not found
func CleanUpCreatedResources(ccName, ccNamespace string) {
	cc := &v1alpha1.CassandraCluster{}

	err := k8sClient.Get(ctx, types.NamespacedName{Name: ccName, Namespace: ccNamespace}, cc)
	if err != nil && kerrors.IsNotFound(err) {
		return
	}
	Expect(err).ToNot(HaveOccurred())

	// delete cassandracluster separately as there's no guarantee that it'll come first in the for loop
	Expect(deleteResource(types.NamespacedName{Namespace: ccNamespace, Name: ccName}, &v1alpha1.CassandraCluster{})).To(Succeed())
	expectResourceIsDeleted(types.NamespacedName{Name: ccName, Namespace: ccNamespace}, &v1alpha1.CassandraCluster{})

	cc.Name = ccName
	cc.Namespace = ccNamespace
	type resourceToDelete struct {
		name    string
		objType client.Object
	}

	resourcesToDelete := []resourceToDelete{
		{name: names.ProberService(cc.Name), objType: &v1.Service{}},
		{name: names.ProberDeployment(cc.Name), objType: &apps.Deployment{}},
		{name: names.ProberServiceAccount(cc.Name), objType: &v1.ServiceAccount{}},
		{name: names.ProberRole(cc.Name), objType: &rbac.Role{}},
		{name: names.ProberRoleBinding(cc.Name), objType: &rbac.RoleBinding{}},
		{name: names.ReaperService(cc.Name), objType: &v1.Service{}},
		{name: names.ReaperCqlConfigMap(cc.Name), objType: &v1.ConfigMap{}},
		{name: names.ShiroConfigMap(cc.Name), objType: &v1.ConfigMap{}},
		{name: names.PrometheusConfigMap(cc.Name), objType: &v1.ConfigMap{}},
		{name: names.MaintenanceConfigMap(cc.Name), objType: &v1.ConfigMap{}},
		{name: names.RepairsConfigMap(cc.Name), objType: &v1.ConfigMap{}},
		{name: names.RolesSecret(cc.Name), objType: &v1.Secret{}},
		{name: names.BaseAdminSecret(cc.Name), objType: &v1.Secret{}},
		{name: names.ActiveAdminSecret(cc.Name), objType: &v1.Secret{}},
		{name: names.AdminAuthConfigSecret(cc.Name), objType: &v1.Secret{}},
		{name: names.PodsConfigConfigmap(cc.Name), objType: &v1.ConfigMap{}},
		{name: cc.Spec.AdminRoleSecretName, objType: &v1.Secret{}},
	}

	if cc.Spec.Roles.SecretName != "" {
		resourcesToDelete = append(resourcesToDelete, resourceToDelete{name: cc.Spec.Roles.SecretName, objType: &v1.Secret{}})
	}

	// add DC specific resources
	for _, dc := range cc.Spec.DCs {
		resourcesToDelete = append(resourcesToDelete, resourceToDelete{name: names.DC(cc.Name, dc.Name), objType: &apps.StatefulSet{}})
		resourcesToDelete = append(resourcesToDelete, resourceToDelete{name: names.DCService(cc.Name, dc.Name), objType: &v1.Service{}})
		resourcesToDelete = append(resourcesToDelete, resourceToDelete{name: names.ReaperDeployment(cc.Name, dc.Name), objType: &apps.Deployment{}})
		resourcesToDelete = append(resourcesToDelete, resourceToDelete{name: names.ConfigMap(cc.Name), objType: &v1.ConfigMap{}})
	}

	// add Cassandra Pods
	for _, dc := range cc.Spec.DCs {
		for i := 0; i < int(*dc.Replicas); i++ {
			resourcesToDelete = append(resourcesToDelete, resourceToDelete{name: names.DC(cc.Name, dc.Name) + "-" + strconv.Itoa(i), objType: &v1.Pod{}})
		}
	}

	for _, resource := range resourcesToDelete {
		logr.Debug(fmt.Sprintf("deleting resource %T:%s", resource.objType, resource.name))
		Expect(deleteResource(types.NamespacedName{Namespace: ccNamespace, Name: resource.name}, resource.objType)).To(Succeed())
	}

	for _, resource := range resourcesToDelete {
		expectResourceIsDeleted(types.NamespacedName{Name: resource.name, Namespace: ccNamespace}, resource.objType)
	}

	nodesList := &v1.NodeList{}
	Expect(k8sClient.List(ctx, nodesList)).To(Succeed())
	if len(nodesList.Items) != 0 {
		for _, node := range nodesList.Items {
			Expect(k8sClient.Delete(ctx, &node)).To(Succeed())
		}
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

func expectResourceIsDeleted(name types.NamespacedName, obj client.Object) {
	Eventually(func() metav1.StatusReason {
		err := k8sClient.Get(context.Background(), name, obj)
		if err != nil {
			if statusErr, ok := err.(*kerrors.StatusError); ok {
				return statusErr.ErrStatus.Reason
			}
			return metav1.StatusReason(err.Error())
		}

		return "Found"
	}, longTimeout, mediumRetry).Should(Equal(metav1.StatusReasonNotFound), fmt.Sprintf("%T %s should be deleted", obj, name))
}

func deleteResource(name types.NamespacedName, obj client.Object) error {
	err := k8sClient.Get(context.Background(), name, obj)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return k8sClient.Delete(context.Background(), obj, client.GracePeriodSeconds(0))
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

func markDeploymentAsReady(namespacedName types.NamespacedName) *apps.Deployment {
	deployment := &apps.Deployment{}

	Eventually(func() error {
		return k8sClient.Get(ctx, namespacedName, deployment)
	}, mediumTimeout, mediumRetry).Should(Succeed())

	deployment.Status.Replicas = *deployment.Spec.Replicas
	deployment.Status.ReadyReplicas = *deployment.Spec.Replicas

	err := k8sClient.Status().Update(ctx, deployment)
	Expect(err).ToNot(HaveOccurred())

	return deployment
}

func validateNumberOfDeployments(namespace string, labels map[string]string, number int) {
	Eventually(func() bool {
		reaperDeployments := &apps.DeploymentList{}
		err := k8sClient.List(ctx, reaperDeployments, client.InNamespace(namespace), client.MatchingLabels(labels))
		Expect(err).NotTo(HaveOccurred())
		return len(reaperDeployments.Items) == number
	}, longTimeout, mediumRetry).Should(BeTrue())
}

func createCassandraPods(cc *v1alpha1.CassandraCluster) {
	nodeIPs := []string{
		"10.3.23.41",
		"10.3.23.42",
		"10.3.23.43",
		"10.3.23.44",
		"10.3.23.45",
		"10.3.23.46",
		"10.3.23.47",
	}
	createNodes(nodeIPs)
	for dcID, dc := range cc.Spec.DCs {
		sts := &apps.StatefulSet{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: names.DC(cc.Name, dc.Name), Namespace: cc.Namespace}, sts)
		Expect(err).ShouldNot(HaveOccurred())
		for replicaID := 0; replicaID < int(*sts.Spec.Replicas); replicaID++ {
			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sts.Name + "-" + strconv.Itoa(replicaID),
					Namespace: sts.Namespace,
					Labels:    sts.Labels,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "cassandra",
							Image: "cassandra:latest",
						},
					},
					NodeName: nodeIPs[replicaID],
				},
			}
			err := k8sClient.Create(ctx, pod)
			Expect(err).ShouldNot(HaveOccurred())
			logr.Debug(fmt.Sprintf("created pod %s", pod.Name))
			Eventually(func() error {
				actualPod := &v1.Pod{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, actualPod)).To(Succeed())
				actualPod.Status.PodIP = fmt.Sprintf("10.0.%d.%d", dcID, replicaID)
				actualPod.Status.ContainerStatuses = []v1.ContainerStatus{
					{
						Name:  "cassandra",
						Ready: true,
					},
				}
				return k8sClient.Status().Update(ctx, actualPod)
			}, mediumTimeout, mediumRetry).Should(Succeed())

			Expect(err).ShouldNot(HaveOccurred())
		}
	}
}

func createNodes(nodeIPs []string) {
	for _, nodeIP := range nodeIPs {
		node := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeIP,
			},
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeExternalIP,
						Address: nodeIP,
					},
					{
						Type:    v1.NodeInternalIP,
						Address: nodeIP,
					},
				},
			},
		}

		existingNode := &v1.Node{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeIP}, existingNode)
		if kerrors.IsNotFound(err) {
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
		}
	}
}

func markAllDCsReady(cc *v1alpha1.CassandraCluster) {
	dcs := &apps.StatefulSetList{}
	err := k8sClient.List(ctx, dcs, client.InNamespace(cc.Namespace), client.MatchingLabels{v1alpha1.CassandraClusterInstance: cc.Name})
	Expect(err).ShouldNot(HaveOccurred())
	for _, dc := range dcs.Items {
		dc := dc
		dc.Status.Replicas = *dc.Spec.Replicas
		dc.Status.ReadyReplicas = *dc.Spec.Replicas
		Expect(k8sClient.Status().Update(ctx, &dc)).To(Succeed())
	}
}

func waitForDCsToBeCreated(cc *v1alpha1.CassandraCluster) {
	Eventually(func() bool {
		dcs := &apps.StatefulSetList{}
		err := k8sClient.List(ctx, dcs, client.InNamespace(cc.Namespace), client.MatchingLabels{v1alpha1.CassandraClusterInstance: cc.Name})
		Expect(err).ShouldNot(HaveOccurred())

		return len(dcs.Items) == len(cc.Spec.DCs)
	}, mediumTimeout, mediumRetry).Should(BeTrue())
}

func waitForResourceToBeCreated(name types.NamespacedName, obj client.Object) {
	Eventually(func() error {
		return k8sClient.Get(ctx, name, obj)
	}, mediumTimeout, mediumRetry).Should(Succeed())
}

func createReadyCluster(cc *v1alpha1.CassandraCluster) {
	createAdminSecret(cc)
	Expect(k8sClient.Create(ctx, cc)).To(Succeed())
	markMocksAsReady(cc)
	waitForDCsToBeCreated(cc)
	markAllDCsReady(cc)
	createCassandraPods(cc)
	for _, dc := range cc.Spec.DCs {
		reaperDeploymentName := types.NamespacedName{Name: names.ReaperDeployment(cc.Name, dc.Name), Namespace: cc.Namespace}
		waitForResourceToBeCreated(reaperDeploymentName, &apps.Deployment{})
		markDeploymentAsReady(reaperDeploymentName)
	}
}

func createAdminSecret(cc *v1alpha1.CassandraCluster) {
	adminSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cc.Spec.AdminRoleSecretName,
			Namespace: cc.Namespace,
		},
		Data: map[string][]byte{
			v1alpha1.CassandraOperatorAdminRole:     []byte("admin-role"),
			v1alpha1.CassandraOperatorAdminPassword: []byte("admin-password"),
		},
	}
	Expect(k8sClient.Create(context.Background(), adminSecret)).To(Succeed())
}
