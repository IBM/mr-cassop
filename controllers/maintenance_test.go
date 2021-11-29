package controllers

import (
	"context"
	"fmt"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	baseCC = &v1alpha1.CassandraCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
		Spec: v1alpha1.CassandraClusterSpec{
			DCs: []v1alpha1.DC{
				{
					Name:     "dc1",
					Replicas: proto.Int32(3),
				},
				{
					Name:     "dc2",
					Replicas: proto.Int32(3),
				},
			},
		},
	}
)

func initializeReconciler(cc *v1alpha1.CassandraCluster) *CassandraClusterReconciler {
	reconciler := createBasicMockedReconciler()
	reconciler.defaultCassandraCluster(cc)
	reconciler.Scheme = baseScheme
	return reconciler
}

func mockedMaintenanceConfigMap(cc *v1alpha1.CassandraCluster) *v1.ConfigMap {
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.MaintenanceConfigMap(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra),
		},
		Data: map[string]string{},
	}
}

func mockedRunningCassandraPod(cc *v1alpha1.CassandraCluster, dc v1alpha1.DC, i int) *v1.Pod {
	stsLabels := labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra)
	stsLabels = labels.WithDCLabel(stsLabels, dc.Name)
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%d", names.DC(cc.Name, dc.Name), i),
			Namespace: cc.Namespace,
			Labels:    stsLabels,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				cassandraContainer(cc, dc, tlsSecretChecksum{}, &v1.Secret{}),
			},
			InitContainers: []v1.Container{
				maintenanceContainer(cc),
			},
			ImagePullSecrets: imagePullSecrets(cc),
			Volumes: []v1.Volume{
				maintenanceVolume(cc),
				cassandraConfigVolume(cc),
				podsConfigVolume(cc),
			},
			RestartPolicy:                 v1.RestartPolicyAlways,
			TerminationGracePeriodSeconds: proto.Int64(30),
			DNSPolicy:                     v1.DNSClusterFirst,
			SecurityContext:               &v1.PodSecurityContext{},
		},
		Status: v1.PodStatus{
			Phase: "Running",
			InitContainerStatuses: []v1.ContainerStatus{
				{
					Name: "maintenance-mode",
					State: v1.ContainerState{
						Terminated: &v1.ContainerStateTerminated{
							ExitCode: 0,
							Reason:   "Completed",
						},
					},
				},
			},
			ContainerStatuses: []v1.ContainerStatus{
				{
					Name: "cassandra",
					State: v1.ContainerState{
						Running: &v1.ContainerStateRunning{},
					},
				},
			},
		},
	}
}

func mockedRunningCassandraPods(cc *v1alpha1.CassandraCluster) []*v1.Pod {
	pods := make([]*v1.Pod, 0)
	for _, dc := range cc.Spec.DCs {
		for j := 0; j < int(*dc.Replicas); j++ {
			pods = append(pods, mockedRunningCassandraPod(cc, dc, j))
		}
	}
	return pods
}

func mockedRunningMaintenancePod(cc *v1alpha1.CassandraCluster, dc v1alpha1.DC, i int) *v1.Pod {
	cassandraPod := mockedRunningCassandraPod(cc, dc, i)
	cassandraPod.Status.InitContainerStatuses[0] = v1.ContainerStatus{
		Name: "maintenance-mode",
		State: v1.ContainerState{
			Running: &v1.ContainerStateRunning{},
		},
	}
	cassandraPod.Status.ContainerStatuses[0] = v1.ContainerStatus{
		Name: "cassandra",
		State: v1.ContainerState{
			Terminated: &v1.ContainerStateTerminated{
				ExitCode: 0,
				Reason:   "Completed",
			},
		},
	}
	return cassandraPod
}

func findResource(name string, objs []client.Object) int {
	for j := 0; j < len(objs); j++ {
		if objs[j].GetName() == name {
			return j
		}
	}
	return -1
}

type test struct {
	name           string
	cc             *v1alpha1.CassandraCluster
	errorMatcher   types.GomegaMatcher
	expectedResult interface{}
	params         map[string]interface{}
}

func TestReconcileMaintenance(t *testing.T) {
	asserts := NewGomegaWithT(t)
	reconciler := initializeReconciler(baseCC)
	k8sResources := []client.Object{
		baseCC,
		mockedMaintenanceConfigMap(baseCC),
	}
	pods := mockedRunningCassandraPods(baseCC)
	for _, pod := range pods {
		k8sResources = append(k8sResources, client.Object(pod))
	}
	tests := []test{
		{
			name:         "returns nil if no maintenance requests are made",
			cc:           baseCC,
			errorMatcher: BeNil(),
			params: map[string]interface{}{
				"maintenance": []v1alpha1.Maintenance{},
			},
		},
		{
			name:         "returns nil if single pod maintenance request is successfully processed",
			cc:           baseCC,
			errorMatcher: BeNil(),
			params: map[string]interface{}{
				"maintenance": []v1alpha1.Maintenance{
					{
						DC: "dc1",
						Pods: []v1alpha1.PodName{
							"test-cassandra-dc1-0",
						},
					},
				},
			},
		},
		{
			name:         "returns nil if single dc maintenance request is successfully processed",
			cc:           baseCC,
			errorMatcher: BeNil(),
			params: map[string]interface{}{
				"maintenance": []v1alpha1.Maintenance{
					{
						DC: "dc1",
					},
				},
			},
		},
		{
			name:         "returns nil if multiple maintenance requests are successfully processed",
			cc:           baseCC,
			errorMatcher: BeNil(),
			params: map[string]interface{}{
				"maintenance": []v1alpha1.Maintenance{
					{
						DC: "dc1",
						Pods: []v1alpha1.PodName{
							"test-cassandra-dc1-0",
						},
					},
					{
						DC: "dc2",
						Pods: []v1alpha1.PodName{
							"test-cassandra-dc2-0",
						},
					},
				},
			},
		},
	}
	// Run all tests
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.cc.Spec.Maintenance = tc.params["maintenance"].([]v1alpha1.Maintenance)
			tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
			reconciler.Client = tClient
			err := reconciler.reconcileMaintenance(context.Background(), tc.cc)
			asserts.Expect(err).To(tc.errorMatcher)
		})
	}
	// Enable maintenance mode for one of the mocked running C* pods
	k8sResources[findResource("test-cassandra-dc1-0", k8sResources)] = mockedRunningMaintenancePod(baseCC, baseCC.Spec.DCs[0], 0)
	// Run the tests again
	for _, tc := range tests {
		t.Run(tc.name+" with pod already in maintenance mode", func(t *testing.T) {
			tc.cc.Spec.Maintenance = tc.params["maintenance"].([]v1alpha1.Maintenance)
			tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
			reconciler.Client = tClient
			err := reconciler.reconcileMaintenance(context.Background(), tc.cc)
			asserts.Expect(err).To(tc.errorMatcher)
		})
	}
}

func TestReconcileMaintenanceConfigmap(t *testing.T) {
	asserts := NewGomegaWithT(t)
	tc := test{
		name:         "create maintenance config map if it does not exist",
		cc:           baseCC,
		errorMatcher: BeNil(),
	}
	t.Run(tc.name, func(t *testing.T) {
		reconciler := initializeReconciler(tc.cc)
		tClient := fake.NewClientBuilder().WithScheme(baseScheme).Build()
		reconciler.Client = tClient
		err := reconciler.reconcileMaintenanceConfigMap(context.Background(), tc.cc)
		asserts.Expect(err).To(tc.errorMatcher)
	})
	tc = test{
		name:         "do nothing if maintenance config map already exists",
		cc:           baseCC,
		errorMatcher: BeNil(),
	}
	t.Run(tc.name, func(t *testing.T) {
		reconciler := initializeReconciler(tc.cc)
		tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(mockedMaintenanceConfigMap(baseCC)).Build()
		reconciler.Client = tClient
		err := reconciler.reconcileMaintenanceConfigMap(context.Background(), tc.cc)
		asserts.Expect(err).To(tc.errorMatcher)
	})
}

func TestGetPod(t *testing.T) {
	asserts := NewGomegaWithT(t)
	reconciler := initializeReconciler(baseCC)
	k8sResources := []client.Object{
		baseCC,
	}
	pods := mockedRunningCassandraPods(baseCC)
	for _, pod := range pods {
		k8sResources = append(k8sResources, client.Object(pod))
	}
	tests := []test{
		{
			name: "returns pod if it exists",
			cc:   baseCC,
			expectedResult: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-%d", names.DC(baseCC.Name, baseCC.Spec.DCs[0].Name), 0),
					Namespace: baseCC.Namespace,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						cassandraContainer(baseCC, baseCC.Spec.DCs[0], tlsSecretChecksum{}, &v1.Secret{}),
					},
					InitContainers: []v1.Container{
						maintenanceContainer(baseCC),
					},
				},
			},
			errorMatcher: BeNil(),
			params: map[string]interface{}{
				"podName": "test-cassandra-dc1-0",
			},
		},
		{
			name:           "returns error if it does not exist",
			cc:             baseCC,
			expectedResult: nil,
			errorMatcher:   Not(BeNil()),
			params: map[string]interface{}{
				"podName": "invalid",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
			reconciler.Client = tClient
			pod, err := reconciler.getPod(context.Background(), tc.cc.Namespace, tc.params["podName"].(string))
			asserts.Expect(err).To(tc.errorMatcher)
			if err == nil {
				asserts.Expect(pod).To(BeAssignableToTypeOf(tc.expectedResult))
				asserts.Expect(pod.Name).To(BeEquivalentTo(tc.expectedResult.(*v1.Pod).Name))
				asserts.Expect(pod.Namespace).To(BeEquivalentTo(tc.expectedResult.(*v1.Pod).Namespace))
				asserts.Expect(pod.Spec.Containers[0].Name).To(BeEquivalentTo(tc.expectedResult.(*v1.Pod).Spec.Containers[0].Name))
				asserts.Expect(pod.Spec.InitContainers[0].Name).To(BeEquivalentTo(tc.expectedResult.(*v1.Pod).Spec.InitContainers[0].Name))
			}
		})
	}
}

func TestGetCassandraPods(t *testing.T) {
	asserts := NewGomegaWithT(t)
	reconciler := initializeReconciler(baseCC)
	k8sResources := []client.Object{
		baseCC,
	}
	pods := mockedRunningCassandraPods(baseCC)
	for _, pod := range pods {
		k8sResources = append(k8sResources, client.Object(pod))
	}
	tests := []test{
		{
			name: "returns C* pods if they exist in the specified namespace",
			cc:   baseCC,
			expectedResult: &v1.PodList{
				Items: []v1.Pod{
					*mockedRunningCassandraPod(baseCC, baseCC.Spec.DCs[0], 0),
					*mockedRunningCassandraPod(baseCC, baseCC.Spec.DCs[0], 1),
					*mockedRunningCassandraPod(baseCC, baseCC.Spec.DCs[0], 2),
					*mockedRunningCassandraPod(baseCC, baseCC.Spec.DCs[1], 0),
					*mockedRunningCassandraPod(baseCC, baseCC.Spec.DCs[1], 1),
					*mockedRunningCassandraPod(baseCC, baseCC.Spec.DCs[1], 2),
				},
			},
			errorMatcher: BeNil(),
			params: map[string]interface{}{
				"namespace": baseCC.Namespace,
			},
		},
		{
			name:           "returns an empty list if pods do not exist in the namespace",
			cc:             baseCC,
			expectedResult: &v1.PodList{},
			errorMatcher:   BeNil(),
			params: map[string]interface{}{
				"namespace": "invalid",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
			reconciler.Client = tClient
			tc.cc.Namespace = tc.params["namespace"].(string)
			pods, err := reconciler.getCassandraPods(context.Background(), tc.cc)
			asserts.Expect(err).To(tc.errorMatcher)
			asserts.Expect(pods).To(BeAssignableToTypeOf(tc.expectedResult))
			asserts.Expect(len(pods.Items)).To(BeEquivalentTo(len(tc.expectedResult.(*v1.PodList).Items)))
			for j := 0; j < len(pods.Items); j++ {
				asserts.Expect(pods.Items[j].Name).To(BeEquivalentTo(tc.expectedResult.(*v1.PodList).Items[j].Name))
			}
		})
	}
}

func TestPatchPodStatusPhase(t *testing.T) {
	asserts := NewGomegaWithT(t)
	reconciler := initializeReconciler(baseCC)
	k8sResources := []client.Object{
		baseCC,
	}
	pods := mockedRunningCassandraPods(baseCC)
	for _, pod := range pods {
		k8sResources = append(k8sResources, client.Object(pod))
	}
	tests := []test{
		{
			name:         "returns nil if patch pod status phase is successful",
			cc:           baseCC,
			errorMatcher: BeNil(),
			params: map[string]interface{}{
				"namespace": baseCC.Namespace,
				"podName":   "test-cassandra-dc1-0",
				"phase":     v1.PodFailed,
			},
		},
		{
			name:         "returns error if get pod fails",
			cc:           baseCC,
			errorMatcher: Not(BeNil()),
			params: map[string]interface{}{
				"namespace": baseCC.Namespace,
				"podName":   "invalid",
				"phase":     v1.PodFailed,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
			reconciler.Client = tClient
			err := reconciler.patchPodStatusPhase(context.Background(), tc.params["namespace"].(string), tc.params["podName"].(string), tc.params["phase"].(v1.PodPhase))
			asserts.Expect(err).To(tc.errorMatcher)
		})
	}
}

func TestPatchConfigMap(t *testing.T) {
	asserts := NewGomegaWithT(t)
	reconciler := initializeReconciler(baseCC)
	k8sResources := []client.Object{
		baseCC,
		mockedMaintenanceConfigMap(baseCC),
	}
	pods := mockedRunningCassandraPods(baseCC)
	for _, pod := range pods {
		k8sResources = append(k8sResources, client.Object(pod))
	}
	tests := []test{
		{
			name:         "returns nil if patch config map is successful",
			cc:           baseCC,
			errorMatcher: BeNil(),
			params: map[string]interface{}{
				"namespace": baseCC.Namespace,
				"mapName":   names.MaintenanceConfigMap(baseCC.Name),
				"data": map[string]string{
					"test-cassandra-dc1-0": "true",
				},
			},
		},
		{
			name:         "returns error if get config map fails",
			cc:           baseCC,
			errorMatcher: Not(BeNil()),
			params: map[string]interface{}{
				"namespace": baseCC.Namespace,
				"mapName":   "invalid",
				"data": map[string]string{
					"test-cassandra-dc1-0": "true",
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
			reconciler.Client = tClient
			err := reconciler.patchConfigMap(context.Background(), tc.params["namespace"].(string), tc.params["mapName"].(string), tc.params["data"].(map[string]string))
			asserts.Expect(err).To(tc.errorMatcher)
		})
	}
}

func TestUpdateMaintenanceMode(t *testing.T) {
	asserts := NewGomegaWithT(t)
	reconciler := initializeReconciler(baseCC)
	k8sResources := []client.Object{
		baseCC,
		mockedMaintenanceConfigMap(baseCC),
	}
	pods := mockedRunningCassandraPods(baseCC)
	for _, pod := range pods {
		k8sResources = append(k8sResources, client.Object(pod))
	}
	tests := []test{
		{
			name:         "returns nil if enabling maintenance mode for running C* pod is successful",
			cc:           baseCC,
			errorMatcher: BeNil(),
			params: map[string]interface{}{
				"enabled": true,
				"pod":     *mockedRunningCassandraPod(baseCC, baseCC.Spec.DCs[0], 0),
			},
		},
		{
			name:         "returns nil if disabling maintenance mode for running C* pod does nothing",
			cc:           baseCC,
			errorMatcher: BeNil(),
			params: map[string]interface{}{
				"enabled": false,
				"pod":     *mockedRunningCassandraPod(baseCC, baseCC.Spec.DCs[0], 0),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
			reconciler.Client = tClient
			err := reconciler.updateMaintenanceMode(context.Background(), tc.cc, tc.params["enabled"].(bool), tc.params["pod"].(v1.Pod))
			asserts.Expect(err).To(tc.errorMatcher)
		})
	}
	// Enable maintenance mode for one of the mocked running C* pods
	k8sResources[findResource("test-cassandra-dc1-0", k8sResources)] = mockedRunningMaintenancePod(baseCC, baseCC.Spec.DCs[0], 0)
	tests = []test{
		{
			name:         "returns nil if enabling maintenance mode for running maintenance pod does nothing",
			cc:           baseCC,
			errorMatcher: BeNil(),
			params: map[string]interface{}{
				"enabled": true,
				"pod":     *mockedRunningMaintenancePod(baseCC, baseCC.Spec.DCs[0], 0),
			},
		},
		{
			name:         "returns nil if disabling maintenance mode for running maintenance pod is successful",
			cc:           baseCC,
			errorMatcher: BeNil(),
			params: map[string]interface{}{
				"enabled": false,
				"pod":     *mockedRunningMaintenancePod(baseCC, baseCC.Spec.DCs[0], 0),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
			reconciler.Client = tClient
			err := reconciler.updateMaintenanceMode(context.Background(), tc.cc, tc.params["enabled"].(bool), tc.params["pod"].(v1.Pod))
			asserts.Expect(err).To(tc.errorMatcher)
		})
	}
}

func TestUpdateMaintenanceConfigMap(t *testing.T) {
	asserts := NewGomegaWithT(t)
	reconciler := initializeReconciler(baseCC)
	k8sResources := []client.Object{
		baseCC,
		mockedMaintenanceConfigMap(baseCC),
	}
	pods := mockedRunningCassandraPods(baseCC)
	for _, pod := range pods {
		k8sResources = append(k8sResources, client.Object(pod))
	}
	tests := []test{
		{
			name:         "returns nil if enabling maintenance mode for running C* pod updates config map",
			cc:           baseCC,
			errorMatcher: BeNil(),
			params: map[string]interface{}{
				"namespace": baseCC.Namespace,
				"cmName":    names.MaintenanceConfigMap(baseCC.Name),
				"enabled":   true,
				"pod":       "test-cassandra-dc1-0",
			},
		},
		{
			name:         "returns nil if disabling maintenance mode for running C* pod does nothing",
			cc:           baseCC,
			errorMatcher: BeNil(),
			params: map[string]interface{}{
				"namespace": baseCC.Namespace,
				"cmName":    names.MaintenanceConfigMap(baseCC.Name),
				"enabled":   false,
				"pod":       "test-cassandra-dc1-0",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
			reconciler.Client = tClient
			err := reconciler.updateMaintenanceConfigMap(context.Background(), tc.params["namespace"].(string), tc.params["cmName"].(string), tc.params["enabled"].(bool), tc.params["pod"].(string))
			asserts.Expect(err).To(tc.errorMatcher)
		})
	}
	// Enable maintenance mode for one of the mocked running C* pods
	k8sResources[findResource("test-cassandra-dc1-0", k8sResources)] = mockedRunningMaintenancePod(baseCC, baseCC.Spec.DCs[0], 0)
	tests = []test{
		{
			name:         "returns nil if enabling maintenance mode for running maintenance pod does nothing",
			cc:           baseCC,
			errorMatcher: BeNil(),
			params: map[string]interface{}{
				"namespace": baseCC.Namespace,
				"cmName":    names.MaintenanceConfigMap(baseCC.Name),
				"enabled":   true,
				"pod":       "test-cassandra-dc1-0",
			},
		},
		{
			name:         "returns nil if disabling maintenance mode for running maintenance pod updates config map",
			cc:           baseCC,
			errorMatcher: BeNil(),
			params: map[string]interface{}{
				"namespace": baseCC.Namespace,
				"cmName":    names.MaintenanceConfigMap(baseCC.Name),
				"enabled":   false,
				"pod":       "test-cassandra-dc1-0",
			},
		},
		{
			name:         "returns error if get config map fails",
			cc:           baseCC,
			errorMatcher: Not(BeNil()),
			params: map[string]interface{}{
				"namespace": baseCC.Namespace,
				"cmName":    "invalid",
				"enabled":   false,
				"pod":       "test-cassandra-dc1-0",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
			reconciler.Client = tClient
			err := reconciler.updateMaintenanceConfigMap(context.Background(), tc.params["namespace"].(string), tc.params["cmName"].(string), tc.params["enabled"].(bool), tc.params["pod"].(string))
			asserts.Expect(err).To(tc.errorMatcher)
		})
	}
}

func TestGenerateMaintenanceStatus(t *testing.T) {
	asserts := NewGomegaWithT(t)
	reconciler := initializeReconciler(baseCC)
	k8sResources := []client.Object{
		baseCC,
		mockedMaintenanceConfigMap(baseCC),
	}
	tests := []test{
		{
			name:           "returns empty status if maintenance requests list is empty",
			cc:             baseCC,
			errorMatcher:   BeNil(),
			expectedResult: []v1alpha1.Maintenance{},
			params: map[string]interface{}{
				"maintenance": make([]v1alpha1.Maintenance, 0),
				"namespace":   baseCC.Namespace,
			},
		},
		{
			name:         "returns matching status if one pod in maintenance mode",
			cc:           baseCC,
			errorMatcher: BeNil(),
			expectedResult: []v1alpha1.Maintenance{
				{
					DC: "dc1",
					Pods: []v1alpha1.PodName{
						"test-cassandra-dc1-0",
					},
				},
			},
			params: map[string]interface{}{
				"maintenance": []v1alpha1.Maintenance{
					{
						DC: "dc1",
						Pods: []v1alpha1.PodName{
							"test-cassandra-dc1-0",
						},
					},
				},
				"namespace": baseCC.Namespace,
			},
		},
		{
			name:         "returns status with all pods in dc listed if only dc name is provided",
			cc:           baseCC,
			errorMatcher: BeNil(),
			expectedResult: []v1alpha1.Maintenance{
				{
					DC: "dc1",
					Pods: []v1alpha1.PodName{
						"test-cassandra-dc1-0",
						"test-cassandra-dc1-1",
						"test-cassandra-dc1-2",
					},
				},
			},
			params: map[string]interface{}{
				"maintenance": []v1alpha1.Maintenance{
					{
						DC: "dc1",
					},
				},
				"namespace": baseCC.Namespace,
			},
		},
		{
			name:         "returns matching status if maintenance list has multiple requests",
			cc:           baseCC,
			errorMatcher: BeNil(),
			expectedResult: []v1alpha1.Maintenance{
				{
					DC: "dc1",
					Pods: []v1alpha1.PodName{
						"test-cassandra-dc1-0",
					},
				},
				{
					DC: "dc2",
					Pods: []v1alpha1.PodName{
						"test-cassandra-dc2-0",
					},
				},
			},
			params: map[string]interface{}{
				"maintenance": []v1alpha1.Maintenance{
					{
						DC: "dc1",
						Pods: []v1alpha1.PodName{
							"test-cassandra-dc1-0",
						},
					},
					{
						DC: "dc2",
						Pods: []v1alpha1.PodName{
							"test-cassandra-dc2-0",
						},
					},
				},
				"namespace": baseCC.Namespace,
			},
		},
	}
	pods := mockedRunningCassandraPods(baseCC)
	for _, pod := range pods {
		k8sResources = append(k8sResources, client.Object(pod))
	}
	tc := tests[0]
	t.Run(tc.name, func(t *testing.T) {
		tc.cc.Spec.Maintenance = tc.params["maintenance"].([]v1alpha1.Maintenance)
		tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
		reconciler.Client = tClient
		maintenance, err := reconciler.generateMaintenanceStatus(context.Background(), tc.cc)
		asserts.Expect(err).To(tc.errorMatcher)
		asserts.Expect(maintenance).To(BeEquivalentTo(tc.expectedResult))
	})
	k8sResources[findResource("test-cassandra-dc1-0", k8sResources)] = mockedRunningMaintenancePod(baseCC, baseCC.Spec.DCs[0], 0)
	tc = tests[1]
	t.Run(tc.name, func(t *testing.T) {
		tc.cc.Spec.Maintenance = tc.params["maintenance"].([]v1alpha1.Maintenance)
		tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
		reconciler.Client = tClient
		maintenance, err := reconciler.generateMaintenanceStatus(context.Background(), tc.cc)
		asserts.Expect(err).To(tc.errorMatcher)
		asserts.Expect(maintenance).To(BeEquivalentTo(tc.expectedResult))
	})
	k8sResources[findResource("test-cassandra-dc1-1", k8sResources)] = mockedRunningMaintenancePod(baseCC, baseCC.Spec.DCs[0], 1)
	k8sResources[findResource("test-cassandra-dc1-2", k8sResources)] = mockedRunningMaintenancePod(baseCC, baseCC.Spec.DCs[0], 2)
	tc = tests[2]
	t.Run(tc.name, func(t *testing.T) {
		tc.cc.Spec.Maintenance = tc.params["maintenance"].([]v1alpha1.Maintenance)
		tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
		reconciler.Client = tClient
		maintenance, err := reconciler.generateMaintenanceStatus(context.Background(), tc.cc)
		asserts.Expect(err).To(tc.errorMatcher)
		asserts.Expect(maintenance).To(BeEquivalentTo(tc.expectedResult))
	})
}
