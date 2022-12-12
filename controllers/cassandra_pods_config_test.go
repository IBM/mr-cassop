package controllers

import (
	"context"
	"net/url"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	"github.com/google/go-cmp/cmp"

	"github.com/ibm/cassandra-operator/controllers/events"
	"k8s.io/client-go/tools/record"

	"go.uber.org/zap"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/mock/gomock"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/mocks"
	"github.com/ibm/cassandra-operator/controllers/prober"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPodsConfigMapData(t *testing.T) {
	ccName := "test-cluster"
	ccNamespace := "default"
	ccObjMeta := metav1.ObjectMeta{
		Name:      ccName,
		Namespace: ccNamespace,
	}

	cLabels := func(ccName, dc string, isSeed bool) map[string]string {
		l := map[string]string{
			v1alpha1.CassandraClusterInstance:  ccName,
			v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
			v1alpha1.CassandraClusterDC:        dc,
		}

		if isSeed {
			l[v1alpha1.CassandraClusterSeed] = "seed"
		}

		return l
	}

	stsObjectMeta := metav1.ObjectMeta{
		Name:      "test-cluster-cassandra-dc1",
		Namespace: "default",
	}
	readyStatefulSet := &appsv1.StatefulSet{
		ObjectMeta: stsObjectMeta,
		Spec: appsv1.StatefulSetSpec{
			Replicas: proto.Int32(6),
		},
		Status: appsv1.StatefulSetStatus{
			Replicas:      6,
			ReadyReplicas: 2,
		},
	}

	asserts := NewGomegaWithT(t)
	cases := []struct {
		name                     string
		k8sObjects               []client.Object
		k8sLists                 []client.ObjectList
		podList                  *v1.PodList
		nodeList                 *v1.NodeList
		cc                       *v1alpha1.CassandraCluster
		externalSeeds            map[string][]string
		expectedCMData           map[string]string
		externalRegionsSeeds     map[string][]string
		externalRegionsReadiness map[string]bool
		expectedError            error
	}{
		{
			name: "simple case with hostport disabled",
			cc: &v1alpha1.CassandraCluster{
				ObjectMeta: ccObjMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int(3),
						},
					},
				},
			},
			podList: &v1.PodList{
				Items: []v1.Pod{
					createTestPod("test-cluster-cassandra-dc1-0", ccNamespace, "uid1", "10.1.1.3", "node1", false, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-1", ccNamespace, "uid2", "10.1.1.4", "node2", false, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-2", ccNamespace, "uid3", "10.1.1.5", "node3", false, cLabels(ccName, "dc1", false)),
				},
			},
			nodeList: &v1.NodeList{},
			k8sObjects: []client.Object{&appsv1.StatefulSet{
				ObjectMeta: stsObjectMeta,
				Spec: appsv1.StatefulSetSpec{
					Replicas: proto.Int32(3),
				},
				Status: appsv1.StatefulSetStatus{
					Replicas:      3,
					ReadyReplicas: 0,
				},
			},
			},
			expectedCMData: map[string]string{
				"test-cluster-cassandra-dc1-0_uid1.sh": `export CASSANDRA_BROADCAST_ADDRESS=10.1.1.3
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.3
export CASSANDRA_SEEDS=test-cluster-cassandra-dc1-0.test-cluster-cassandra-dc1.default.svc.cluster.local,test-cluster-cassandra-dc1-1.test-cluster-cassandra-dc1.default.svc.cluster.local
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				"test-cluster-cassandra-dc1-1_uid2.sh": `export CASSANDRA_BROADCAST_ADDRESS=10.1.1.4
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.4
export CASSANDRA_SEEDS=test-cluster-cassandra-dc1-0.test-cluster-cassandra-dc1.default.svc.cluster.local,test-cluster-cassandra-dc1-1.test-cluster-cassandra-dc1.default.svc.cluster.local
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				"test-cluster-cassandra-dc1-2_uid3.sh": `export CASSANDRA_BROADCAST_ADDRESS=10.1.1.5
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.5
export CASSANDRA_SEEDS=test-cluster-cassandra-dc1-0.test-cluster-cassandra-dc1.default.svc.cluster.local,test-cluster-cassandra-dc1-1.test-cluster-cassandra-dc1.default.svc.cluster.local
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
export PAUSE_REASON="waiting for seed nodes to init"
`,
			},
			expectedError: nil,
		},
		{
			name: "non seed nodes should boot one at a time",
			cc: &v1alpha1.CassandraCluster{
				ObjectMeta: ccObjMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int(6),
						},
					},
				},
			},
			podList: &v1.PodList{
				Items: []v1.Pod{
					createTestPod("test-cluster-cassandra-dc1-0", ccNamespace, "uid1", "10.1.1.3", "node1", true, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-1", ccNamespace, "uid2", "10.1.1.4", "node2", true, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-2", ccNamespace, "uid3", "10.1.1.5", "node3", true, cLabels(ccName, "dc1", false)),
					createTestPod("test-cluster-cassandra-dc1-3", ccNamespace, "uid4", "10.1.1.6", "node4", false, cLabels(ccName, "dc1", false)),
					createTestPod("test-cluster-cassandra-dc1-4", ccNamespace, "uid5", "10.1.1.7", "node5", false, cLabels(ccName, "dc1", false)),
					createTestPod("test-cluster-cassandra-dc1-5", ccNamespace, "uid6", "10.1.1.8", "node6", false, cLabels(ccName, "dc1", false)),
				},
			},
			nodeList: &v1.NodeList{},
			k8sObjects: []client.Object{&appsv1.StatefulSet{
				ObjectMeta: stsObjectMeta,
				Spec: appsv1.StatefulSetSpec{
					Replicas: proto.Int32(6),
				},
				Status: appsv1.StatefulSetStatus{
					Replicas:      6,
					ReadyReplicas: 2,
				},
			},
			},
			expectedCMData: map[string]string{
				"test-cluster-cassandra-dc1-0_uid1.sh": `export CASSANDRA_BROADCAST_ADDRESS=10.1.1.3
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.3
export CASSANDRA_SEEDS=test-cluster-cassandra-dc1-0.test-cluster-cassandra-dc1.default.svc.cluster.local,test-cluster-cassandra-dc1-1.test-cluster-cassandra-dc1.default.svc.cluster.local
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				"test-cluster-cassandra-dc1-1_uid2.sh": `export CASSANDRA_BROADCAST_ADDRESS=10.1.1.4
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.4
export CASSANDRA_SEEDS=test-cluster-cassandra-dc1-0.test-cluster-cassandra-dc1.default.svc.cluster.local,test-cluster-cassandra-dc1-1.test-cluster-cassandra-dc1.default.svc.cluster.local
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				"test-cluster-cassandra-dc1-2_uid3.sh": `export CASSANDRA_BROADCAST_ADDRESS=10.1.1.5
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.5
export CASSANDRA_SEEDS=test-cluster-cassandra-dc1-0.test-cluster-cassandra-dc1.default.svc.cluster.local,test-cluster-cassandra-dc1-1.test-cluster-cassandra-dc1.default.svc.cluster.local
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				"test-cluster-cassandra-dc1-3_uid4.sh": `export CASSANDRA_BROADCAST_ADDRESS=10.1.1.6
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.6
export CASSANDRA_SEEDS=test-cluster-cassandra-dc1-0.test-cluster-cassandra-dc1.default.svc.cluster.local,test-cluster-cassandra-dc1-1.test-cluster-cassandra-dc1.default.svc.cluster.local
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				"test-cluster-cassandra-dc1-4_uid5.sh": `export CASSANDRA_BROADCAST_ADDRESS=10.1.1.7
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.7
export CASSANDRA_SEEDS=test-cluster-cassandra-dc1-0.test-cluster-cassandra-dc1.default.svc.cluster.local,test-cluster-cassandra-dc1-1.test-cluster-cassandra-dc1.default.svc.cluster.local
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
export PAUSE_REASON="waiting for other non seed nodes since only one non seed node can start at a time"
`,
				"test-cluster-cassandra-dc1-5_uid6.sh": `export CASSANDRA_BROADCAST_ADDRESS=10.1.1.8
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.8
export CASSANDRA_SEEDS=test-cluster-cassandra-dc1-0.test-cluster-cassandra-dc1.default.svc.cluster.local,test-cluster-cassandra-dc1-1.test-cluster-cassandra-dc1.default.svc.cluster.local
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
export PAUSE_REASON="waiting for other non seed nodes since only one non seed node can start at a time"
`,
			},
			expectedError: nil,
		},
		{
			name: "with not found no error should be returned",
			cc: &v1alpha1.CassandraCluster{
				ObjectMeta: ccObjMeta,
			},
			podList:        &v1.PodList{},
			nodeList:       &v1.NodeList{},
			k8sLists:       []client.ObjectList{},
			expectedCMData: nil,
			expectedError:  nil,
		},
		{
			name: "hostport enabled, no external DC domains",
			cc: &v1alpha1.CassandraCluster{
				ObjectMeta: ccObjMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int(3),
						},
					},
					HostPort: v1alpha1.HostPort{
						Enabled: true,
					},
				},
			},
			k8sObjects: []client.Object{&appsv1.StatefulSet{
				ObjectMeta: stsObjectMeta,
				Spec: appsv1.StatefulSetSpec{
					Replicas: proto.Int32(3),
				},
				Status: appsv1.StatefulSetStatus{
					Replicas:      3,
					ReadyReplicas: 3,
				},
			}},
			podList: &v1.PodList{
				Items: []v1.Pod{
					createTestPod("test-cluster-cassandra-dc1-0", ccNamespace, "uid1", "10.1.1.3", "node1", true, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-1", ccNamespace, "uid2", "10.1.1.4", "node2", true, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-2", ccNamespace, "uid3", "10.1.1.5", "node3", true, cLabels(ccName, "dc1", false)),
				},
			},
			nodeList: &v1.NodeList{
				Items: []v1.Node{
					createTestNode("node1", "12.43.22.143", "54.32.141.231", nil),
					createTestNode("node2", "12.43.22.153", "54.32.141.232", nil),
					createTestNode("node3", "12.43.22.142", "54.32.141.233", nil),
				},
			},
			expectedCMData: map[string]string{
				`test-cluster-cassandra-dc1-0_uid1.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.143
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.3
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				`test-cluster-cassandra-dc1-1_uid2.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.153
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.4
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				`test-cluster-cassandra-dc1-2_uid3.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.142
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.5
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
			},
			expectedError: nil,
		},
		{
			name: "hostport enabled, use external IP",
			cc: &v1alpha1.CassandraCluster{
				ObjectMeta: ccObjMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int(3),
						},
					},
					HostPort: v1alpha1.HostPort{
						Enabled:           true,
						UseExternalHostIP: true,
					},
				},
			},
			k8sObjects: []client.Object{&appsv1.StatefulSet{
				ObjectMeta: stsObjectMeta,
				Spec: appsv1.StatefulSetSpec{
					Replicas: proto.Int32(3),
				},
				Status: appsv1.StatefulSetStatus{
					Replicas:      3,
					ReadyReplicas: 3,
				},
			}},
			podList: &v1.PodList{
				Items: []v1.Pod{
					createTestPod("test-cluster-cassandra-dc1-0", ccNamespace, "uid1", "10.1.1.3", "node1", true, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-1", ccNamespace, "uid2", "10.1.1.4", "node2", true, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-2", ccNamespace, "uid3", "10.1.1.5", "node3", true, cLabels(ccName, "dc1", false)),
				},
			},
			nodeList: &v1.NodeList{
				Items: []v1.Node{
					createTestNode("node1", "12.43.22.143", "54.32.141.231", nil),
					createTestNode("node2", "12.43.22.153", "54.32.141.232", nil),
					createTestNode("node3", "12.43.22.142", "54.32.141.233", nil),
				},
			},
			expectedCMData: map[string]string{
				`test-cluster-cassandra-dc1-0_uid1.sh`: `export CASSANDRA_BROADCAST_ADDRESS=54.32.141.231
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.3
export CASSANDRA_SEEDS=54.32.141.231,54.32.141.232
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				`test-cluster-cassandra-dc1-1_uid2.sh`: `export CASSANDRA_BROADCAST_ADDRESS=54.32.141.232
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.4
export CASSANDRA_SEEDS=54.32.141.231,54.32.141.232
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				`test-cluster-cassandra-dc1-2_uid3.sh`: `export CASSANDRA_BROADCAST_ADDRESS=54.32.141.233
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.5
export CASSANDRA_SEEDS=54.32.141.231,54.32.141.232
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
			},
			expectedError: nil,
		},
		{
			name: "ExternalRegions.Seeds should be added too",
			cc: &v1alpha1.CassandraCluster{
				ObjectMeta: ccObjMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int(3),
						},
					},
					HostPort: v1alpha1.HostPort{
						Enabled:           true,
						UseExternalHostIP: true,
					},
					ExternalRegions: v1alpha1.ExternalRegions{
						Unmanaged: []v1alpha1.UnmanagedRegion{
							{
								Seeds: []string{"10.10.10.121", "10.10.9.121"},
							},
							{
								Seeds: []string{"10.11.10.121", "10.11.9.121"},
							},
						},
					},
				},
			},
			k8sObjects: []client.Object{readyStatefulSet},
			podList: &v1.PodList{
				Items: []v1.Pod{
					createTestPod("test-cluster-cassandra-dc1-0", ccNamespace, "uid1", "10.1.1.3", "node1", true, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-1", ccNamespace, "uid2", "10.1.1.4", "node2", true, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-2", ccNamespace, "uid3", "10.1.1.5", "node3", true, cLabels(ccName, "dc1", false)),
				},
			},
			nodeList: &v1.NodeList{
				Items: []v1.Node{
					createTestNode("node1", "12.43.22.143", "54.32.141.231", nil),
					createTestNode("node2", "12.43.22.153", "54.32.141.232", nil),
					createTestNode("node3", "12.43.22.142", "54.32.141.233", nil),
				},
			},
			expectedCMData: map[string]string{
				`test-cluster-cassandra-dc1-0_uid1.sh`: `export CASSANDRA_BROADCAST_ADDRESS=54.32.141.231
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.3
export CASSANDRA_SEEDS=54.32.141.231,54.32.141.232,10.10.10.121,10.10.9.121,10.11.10.121,10.11.9.121
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				`test-cluster-cassandra-dc1-1_uid2.sh`: `export CASSANDRA_BROADCAST_ADDRESS=54.32.141.232
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.4
export CASSANDRA_SEEDS=54.32.141.231,54.32.141.232,10.10.10.121,10.10.9.121,10.11.10.121,10.11.9.121
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				`test-cluster-cassandra-dc1-2_uid3.sh`: `export CASSANDRA_BROADCAST_ADDRESS=54.32.141.233
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.5
export CASSANDRA_SEEDS=54.32.141.231,54.32.141.232,10.10.10.121,10.10.9.121,10.11.10.121,10.11.9.121
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
			},
			expectedError: nil,
		},
		{
			name: "ErrPodNotScheduled error if pod has nodename empty",
			cc: &v1alpha1.CassandraCluster{
				ObjectMeta: ccObjMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int(3),
						},
					},
					HostPort: v1alpha1.HostPort{
						Enabled:           true,
						UseExternalHostIP: true,
					},
				},
			},
			podList: &v1.PodList{
				Items: []v1.Pod{
					createTestPod("test-cluster-cassandra-dc1-0", ccNamespace, "uid1", "10.1.1.3", "node1", true, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-1", ccNamespace, "uid2", "10.1.1.4", "node2", true, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-2", ccNamespace, "uid3", "10.1.1.5", "", true, cLabels(ccName, "dc1", false)),
				},
			},
			nodeList: &v1.NodeList{
				Items: []v1.Node{
					createTestNode("node1", "12.43.22.143", "54.32.141.231", nil),
					createTestNode("node2", "12.43.22.153", "54.32.141.232", nil),
					createTestNode("node3", "12.43.22.142", "54.32.141.233", nil),
				},
			},
			expectedCMData: nil,
			expectedError:  errors.New("error getting broadcast addresses: cannot get pod's broadcast address: One of pods is not scheduled yet"),
		},
		{
			name: "ErrPodNotScheduled error if pod has PodIP empty",
			cc: &v1alpha1.CassandraCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int(3),
						},
					},
					HostPort: v1alpha1.HostPort{
						Enabled: false,
					},
				},
			},
			podList: &v1.PodList{
				Items: []v1.Pod{
					createTestPod("test-cluster-cassandra-dc1-0", ccNamespace, "uid1", "", "node1", false, map[string]string{
						v1alpha1.CassandraClusterInstance:  "test-cluster",
						v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
						v1alpha1.CassandraClusterSeed:      "test-cluster-cassandra-dc1-0",
						v1alpha1.CassandraClusterDC:        "dc1",
					}),
					createTestPod("test-cluster-cassandra-dc1-1", ccNamespace, "uid2", "10.1.1.4", "node2", true, map[string]string{
						v1alpha1.CassandraClusterInstance:  "test-cluster",
						v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
						v1alpha1.CassandraClusterSeed:      "test-cluster-cassandra-dc1-1",
						v1alpha1.CassandraClusterDC:        "dc1",
					}),
					createTestPod("test-cluster-cassandra-dc1-2", ccNamespace, "uid3", "10.1.1.5", "node3", true, map[string]string{
						v1alpha1.CassandraClusterInstance:  "test-cluster",
						v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
						v1alpha1.CassandraClusterDC:        "dc1",
					}),
				},
			},
			nodeList: &v1.NodeList{
				Items: []v1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node1",
						},
						Status: v1.NodeStatus{
							Addresses: []v1.NodeAddress{
								{
									Type:    v1.NodeInternalIP,
									Address: "12.43.22.143",
								},
								{
									Type:    v1.NodeExternalIP,
									Address: "54.32.141.231",
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node2",
						},
						Status: v1.NodeStatus{
							Addresses: []v1.NodeAddress{
								{
									Type:    v1.NodeInternalIP,
									Address: "12.43.22.153",
								},
								{
									Type:    v1.NodeExternalIP,
									Address: "54.32.141.232",
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node3",
						},
						Status: v1.NodeStatus{
							Addresses: []v1.NodeAddress{
								{
									Type:    v1.NodeInternalIP,
									Address: "12.43.22.142",
								},
								{
									Type:    v1.NodeExternalIP,
									Address: "54.32.141.233",
								},
							},
						},
					},
				},
			},
			expectedCMData: nil,
			expectedError:  errors.New("error getting broadcast addresses: cannot get pod's broadcast address: One of pods is not scheduled yet"),
		},
		{
			name: "multi-region with ready regions and DCs",
			cc: &v1alpha1.CassandraCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int(3),
						},
					},
					HostPort: v1alpha1.HostPort{
						Enabled: true,
					},
					Ingress: v1alpha1.Ingress{
						Domain: "region1.ingress.domain",
						Secret: "region1.ingress.domain-secret",
					},
					ExternalRegions: v1alpha1.ExternalRegions{
						Managed: []v1alpha1.ManagedRegion{
							{
								Domain: "region2.ingress.domain",
							},
						},
					},
				},
			},
			externalRegionsReadiness: map[string]bool{
				"default-test-cluster-cassandra-prober.region2.ingress.domain": true,
			},
			externalRegionsSeeds: map[string][]string{
				"default-test-cluster-cassandra-prober.region2.ingress.domain": {"42.32.34.111", "42.32.34.113"},
			},
			podList: &v1.PodList{
				Items: []v1.Pod{
					createTestPod("test-cluster-cassandra-dc1-0", ccNamespace, "uid1", "10.1.1.3", "node1", true, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-1", ccNamespace, "uid2", "10.1.1.4", "node2", true, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-2", ccNamespace, "uid3", "10.1.1.5", "node3", true, cLabels(ccName, "dc1", false)),
				},
			},
			nodeList: &v1.NodeList{
				Items: []v1.Node{
					createTestNode("node1", "12.43.22.143", "54.32.141.231", nil),
					createTestNode("node2", "12.43.22.153", "54.32.141.232", nil),
					createTestNode("node3", "12.43.22.142", "54.32.141.233", nil),
				},
			},
			k8sLists: []client.ObjectList{
				&appsv1.StatefulSetList{
					Items: []appsv1.StatefulSet{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1",
								Namespace: "default",
							},
							Spec: appsv1.StatefulSetSpec{
								Replicas: proto.Int(3),
							},
							Status: appsv1.StatefulSetStatus{
								ReadyReplicas: 3,
							},
						},
					},
				},
			},
			expectedCMData: map[string]string{
				`test-cluster-cassandra-dc1-0_uid1.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.143
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.3
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				`test-cluster-cassandra-dc1-1_uid2.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.153
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.4
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				`test-cluster-cassandra-dc1-2_uid3.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.142
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.5
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
			},
			expectedError: nil,
		},
		{
			name: "multi-region with two not ready regions",
			cc: &v1alpha1.CassandraCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int(3),
						},
						{
							Name:     "dc3",
							Replicas: proto.Int(3),
						},
					},
					HostPort: v1alpha1.HostPort{
						Enabled: true,
					},
					Ingress: v1alpha1.Ingress{
						Domain: "region1.ingress.domain",
						Secret: "region1.ingress.domain-secret",
					},
					ExternalRegions: v1alpha1.ExternalRegions{
						Managed: []v1alpha1.ManagedRegion{
							{
								Domain: "region2.ingress.domain",
							},
						},
					},
				},
			},
			externalRegionsReadiness: map[string]bool{
				"default-test-cluster-cassandra-prober.region2.ingress.domain": false,
			},
			externalRegionsSeeds: map[string][]string{
				"default-test-cluster-cassandra-prober.region2.ingress.domain": {"42.32.34.111", "42.32.34.113"},
			},
			podList: &v1.PodList{
				Items: []v1.Pod{
					createTestPod("test-cluster-cassandra-dc1-0", ccNamespace, "uid1", "10.1.1.3", "node1", false, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-1", ccNamespace, "uid2", "10.1.1.4", "node2", false, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-2", ccNamespace, "uid3", "10.1.1.5", "node3", false, cLabels(ccName, "dc1", false)),
					createTestPod("test-cluster-cassandra-dc3-0", ccNamespace, "uid1", "10.1.2.3", "node1", false, cLabels(ccName, "dc3", true)),
					createTestPod("test-cluster-cassandra-dc3-1", ccNamespace, "uid2", "10.1.2.4", "node2", false, cLabels(ccName, "dc3", true)),
					createTestPod("test-cluster-cassandra-dc3-2", ccNamespace, "uid3", "10.1.2.5", "node3", false, cLabels(ccName, "dc3", false)),
				},
			},
			nodeList: &v1.NodeList{
				Items: []v1.Node{
					createTestNode("node1", "12.43.22.143", "54.32.141.231", nil),
					createTestNode("node2", "12.43.22.153", "54.32.141.232", nil),
					createTestNode("node3", "12.43.22.142", "54.32.141.233", nil),
				},
			},
			k8sLists: []client.ObjectList{
				&appsv1.StatefulSetList{
					Items: []appsv1.StatefulSet{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1",
								Namespace: "default",
							},
							Spec: appsv1.StatefulSetSpec{
								Replicas: proto.Int(3),
							},
							Status: appsv1.StatefulSetStatus{
								ReadyReplicas: 1,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc3",
								Namespace: "default",
							},
							Spec: appsv1.StatefulSetSpec{
								Replicas: proto.Int(3),
							},
							Status: appsv1.StatefulSetStatus{
								ReadyReplicas: 0,
							},
						},
					},
				},
			},
			expectedCMData: map[string]string{
				`test-cluster-cassandra-dc1-0_uid1.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.143
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.3
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				`test-cluster-cassandra-dc1-1_uid2.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.153
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.4
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				`test-cluster-cassandra-dc1-2_uid3.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.142
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.5
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
export PAUSE_REASON="waiting for seed nodes to init"
`,
				`test-cluster-cassandra-dc3-0_uid1.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.143
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.2.3
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
export PAUSE_REASON="waiting for other DCs to init"
`,
				`test-cluster-cassandra-dc3-1_uid2.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.153
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.2.4
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
export PAUSE_REASON="waiting for other DCs to init"
`,
				`test-cluster-cassandra-dc3-2_uid3.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.142
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.2.5
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
export PAUSE_REASON="waiting for other DCs to init"
`,
			},
			expectedError: nil,
		},
		{
			name: "multi-region with current region initialized",
			cc: &v1alpha1.CassandraCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int(3),
						},
						{
							Name:     "dc3",
							Replicas: proto.Int(3),
						},
					},
					HostPort: v1alpha1.HostPort{
						Enabled: true,
					},
					Ingress: v1alpha1.Ingress{
						Domain: "region1.ingress.domain",
						Secret: "region1.ingress.domain-secret",
					},
					ExternalRegions: v1alpha1.ExternalRegions{
						Managed: []v1alpha1.ManagedRegion{
							{
								Domain: "region2.ingress.domain",
							},
						},
					},
				},
			},
			externalRegionsReadiness: map[string]bool{
				"default-test-cluster-cassandra-prober.region2.ingress.domain": false,
			},
			externalRegionsSeeds: map[string][]string{
				"default-test-cluster-cassandra-prober.region2.ingress.domain": {"42.32.34.111", "42.32.34.113"},
			},
			podList: &v1.PodList{
				Items: []v1.Pod{
					createTestPod("test-cluster-cassandra-dc1-0", ccNamespace, "uid1", "10.1.1.3", "node1", true, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-1", ccNamespace, "uid2", "10.1.1.4", "node2", true, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-2", ccNamespace, "uid3", "10.1.1.5", "node3", true, cLabels(ccName, "dc1", false)),
					createTestPod("test-cluster-cassandra-dc3-0", ccNamespace, "uid1", "10.1.2.3", "node1", true, cLabels(ccName, "dc3", true)),
					createTestPod("test-cluster-cassandra-dc3-1", ccNamespace, "uid2", "10.1.2.4", "node2", true, cLabels(ccName, "dc3", true)),
					createTestPod("test-cluster-cassandra-dc3-2", ccNamespace, "uid3", "10.1.2.5", "node3", true, cLabels(ccName, "dc3", false)),
				},
			},
			nodeList: &v1.NodeList{
				Items: []v1.Node{
					createTestNode("node1", "12.43.22.143", "54.32.141.231", nil),
					createTestNode("node2", "12.43.22.153", "54.32.141.232", nil),
					createTestNode("node3", "12.43.22.142", "54.32.141.233", nil),
				},
			},
			k8sLists: []client.ObjectList{
				&appsv1.StatefulSetList{
					Items: []appsv1.StatefulSet{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1",
								Namespace: "default",
							},
							Spec: appsv1.StatefulSetSpec{
								Replicas: proto.Int(3),
							},
							Status: appsv1.StatefulSetStatus{
								ReadyReplicas: 3,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc3",
								Namespace: "default",
							},
							Spec: appsv1.StatefulSetSpec{
								Replicas: proto.Int(3),
							},
							Status: appsv1.StatefulSetStatus{
								ReadyReplicas: 3,
							},
						},
					},
				},
			},
			expectedCMData: map[string]string{
				`test-cluster-cassandra-dc1-2_uid3.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.142
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.5
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				`test-cluster-cassandra-dc3-0_uid1.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.143
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.2.3
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				`test-cluster-cassandra-dc3-1_uid2.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.153
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.2.4
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				`test-cluster-cassandra-dc3-2_uid3.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.142
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.2.5
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				`test-cluster-cassandra-dc1-0_uid1.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.143
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.3
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				`test-cluster-cassandra-dc1-1_uid2.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.153
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.4
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
			},
			expectedError: nil,
		},
		{
			name: "multi-region with current region on pause",
			cc: &v1alpha1.CassandraCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int(3),
						},
						{
							Name:     "dc3",
							Replicas: proto.Int(3),
						},
					},
					HostPort: v1alpha1.HostPort{
						Enabled: true,
					},
					Ingress: v1alpha1.Ingress{
						Domain: "region2.ingress.domain",
						Secret: "region2.ingress.domain-secret",
					},
					ExternalRegions: v1alpha1.ExternalRegions{
						Managed: []v1alpha1.ManagedRegion{
							{
								Domain: "region1.ingress.domain",
							},
						},
					},
				},
			},
			externalRegionsReadiness: map[string]bool{
				"default-test-cluster-cassandra-prober.region1.ingress.domain": false,
			},
			externalRegionsSeeds: map[string][]string{
				"default-test-cluster-cassandra-prober.region1.ingress.domain": {"42.32.34.111", "42.32.34.113"},
			},
			podList: &v1.PodList{
				Items: []v1.Pod{
					createTestPod("test-cluster-cassandra-dc1-0", ccNamespace, "uid1", "10.1.1.3", "node1", false, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-1", ccNamespace, "uid2", "10.1.1.4", "node2", false, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-2", ccNamespace, "uid3", "10.1.1.5", "node3", false, cLabels(ccName, "dc1", false)),
					createTestPod("test-cluster-cassandra-dc3-0", ccNamespace, "uid1", "10.1.2.3", "node1", false, cLabels(ccName, "dc3", true)),
					createTestPod("test-cluster-cassandra-dc3-1", ccNamespace, "uid2", "10.1.2.4", "node2", false, cLabels(ccName, "dc3", true)),
					createTestPod("test-cluster-cassandra-dc3-2", ccNamespace, "uid3", "10.1.2.5", "node3", false, cLabels(ccName, "dc3", false)),
				},
			},
			nodeList: &v1.NodeList{
				Items: []v1.Node{
					createTestNode("node1", "12.43.22.143", "54.32.141.231", nil),
					createTestNode("node2", "12.43.22.153", "54.32.141.232", nil),
					createTestNode("node3", "12.43.22.142", "54.32.141.233", nil),
				},
			},
			k8sLists: []client.ObjectList{
				&appsv1.StatefulSetList{
					Items: []appsv1.StatefulSet{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1",
								Namespace: "default",
							},
							Spec: appsv1.StatefulSetSpec{
								Replicas: proto.Int(3),
							},
							Status: appsv1.StatefulSetStatus{
								ReadyReplicas: 0,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc3",
								Namespace: "default",
							},
							Spec: appsv1.StatefulSetSpec{
								Replicas: proto.Int(3),
							},
							Status: appsv1.StatefulSetStatus{
								ReadyReplicas: 0,
							},
						},
					},
				},
			},
			expectedCMData: map[string]string{
				`test-cluster-cassandra-dc1-2_uid3.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.142
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.5
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
export PAUSE_REASON="waiting for other regions to init"
`,
				`test-cluster-cassandra-dc3-0_uid1.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.143
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.2.3
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
export PAUSE_REASON="waiting for other regions to init"
`,
				`test-cluster-cassandra-dc3-1_uid2.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.153
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.2.4
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
export PAUSE_REASON="waiting for other regions to init"
`,
				`test-cluster-cassandra-dc3-2_uid3.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.142
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.2.5
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
export PAUSE_REASON="waiting for other regions to init"
`,
				`test-cluster-cassandra-dc1-0_uid1.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.143
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.3
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
export PAUSE_REASON="waiting for other regions to init"
`,
				`test-cluster-cassandra-dc1-1_uid2.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.153
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.4
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
export PAUSE_REASON="waiting for other regions to init"
`,
			},
			expectedError: nil,
		},
		{
			name: "fail to get node info",
			cc: &v1alpha1.CassandraCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int(3),
						},
						{
							Name:     "dc3",
							Replicas: proto.Int(3),
						},
					},
					HostPort: v1alpha1.HostPort{
						Enabled: true,
					},
					Ingress: v1alpha1.Ingress{
						Domain: "region2.ingress.domain",
						Secret: "region2.ingress.domain-secret",
					},
					ExternalRegions: v1alpha1.ExternalRegions{
						Managed: []v1alpha1.ManagedRegion{
							{
								Domain: "region1.ingress.domain",
							},
						},
					},
				},
			},
			podList: &v1.PodList{
				Items: []v1.Pod{
					createTestPod("test-cluster-cassandra-dc1-0", ccNamespace, "uid1", "10.1.1.3", "node1", false, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-1", ccNamespace, "uid2", "10.1.1.4", "node2", false, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-2", ccNamespace, "uid3", "10.1.1.5", "node3", false, cLabels(ccName, "dc1", false)),
					createTestPod("test-cluster-cassandra-dc3-0", ccNamespace, "uid1", "10.1.2.3", "node1", false, cLabels(ccName, "dc3", true)),
					createTestPod("test-cluster-cassandra-dc3-1", ccNamespace, "uid2", "10.1.2.4", "node2", false, cLabels(ccName, "dc3", true)),
					createTestPod("test-cluster-cassandra-dc3-2", ccNamespace, "uid3", "10.1.2.5", "node3", false, cLabels(ccName, "dc3", false)),
				},
			},
			nodeList: &v1.NodeList{
				Items: []v1.Node{
					createTestNode("node1", "12.43.22.143", "54.32.141.231", nil),
					createTestNode("node2", "12.43.22.153", "54.32.141.232", nil),
					// node3 is missing
				},
			},
			k8sLists: []client.ObjectList{
				&appsv1.StatefulSetList{
					Items: []appsv1.StatefulSet{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1",
								Namespace: "default",
							},
							Spec: appsv1.StatefulSetSpec{
								Replicas: proto.Int(3),
							},
							Status: appsv1.StatefulSetStatus{
								ReadyReplicas: 0,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc3",
								Namespace: "default",
							},
							Spec: appsv1.StatefulSetSpec{
								Replicas: proto.Int(3),
							},
							Status: appsv1.StatefulSetStatus{
								ReadyReplicas: 0,
							},
						},
					},
				},
			},
			expectedCMData: nil,
			expectedError:  errors.New("error getting broadcast addresses: cannot get pod's broadcast address: Node \"node3\" not found"),
		},
		{
			name: "zone as racks enabled",
			cc: &v1alpha1.CassandraCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int(3),
						},
					},
					HostPort: v1alpha1.HostPort{
						Enabled: false,
					},
					Cassandra: &v1alpha1.Cassandra{
						ZonesAsRacks: true,
					},
				},
			},
			k8sObjects: []client.Object{readyStatefulSet},
			podList: &v1.PodList{
				Items: []v1.Pod{
					createTestPod("test-cluster-cassandra-dc1-0", ccNamespace, "uid1", "10.1.1.3", "node1", true, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-1", ccNamespace, "uid2", "10.1.1.4", "node2", true, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-2", ccNamespace, "uid3", "10.1.1.5", "node3", true, cLabels(ccName, "dc1", false)),
				},
			},
			nodeList: &v1.NodeList{
				Items: []v1.Node{
					createTestNode("node1", "12.43.22.143", "54.32.141.231", map[string]string{v1.LabelTopologyZone: "zone1"}),
					createTestNode("node2", "12.43.22.153", "54.32.141.232", map[string]string{v1.LabelTopologyZone: "zone1"}),
					createTestNode("node3", "12.43.22.142", "54.32.141.233", map[string]string{v1.LabelTopologyZone: "zone2"}),
				},
			},
			expectedCMData: map[string]string{
				`test-cluster-cassandra-dc1-0_uid1.sh`: `export CASSANDRA_RACK=zone1
export CASSANDRA_ENDPOINT_SNITCH=GossipingPropertyFileSnitch
export CASSANDRA_BROADCAST_ADDRESS=10.1.1.3
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.3
export CASSANDRA_SEEDS=test-cluster-cassandra-dc1-0.test-cluster-cassandra-dc1.default.svc.cluster.local,test-cluster-cassandra-dc1-1.test-cluster-cassandra-dc1.default.svc.cluster.local
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				`test-cluster-cassandra-dc1-1_uid2.sh`: `export CASSANDRA_RACK=zone1
export CASSANDRA_ENDPOINT_SNITCH=GossipingPropertyFileSnitch
export CASSANDRA_BROADCAST_ADDRESS=10.1.1.4
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.4
export CASSANDRA_SEEDS=test-cluster-cassandra-dc1-0.test-cluster-cassandra-dc1.default.svc.cluster.local,test-cluster-cassandra-dc1-1.test-cluster-cassandra-dc1.default.svc.cluster.local
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
				`test-cluster-cassandra-dc1-2_uid3.sh`: `export CASSANDRA_RACK=zone2
export CASSANDRA_ENDPOINT_SNITCH=GossipingPropertyFileSnitch
export CASSANDRA_BROADCAST_ADDRESS=10.1.1.5
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.5
export CASSANDRA_SEEDS=test-cluster-cassandra-dc1-0.test-cluster-cassandra-dc1.default.svc.cluster.local,test-cluster-cassandra-dc1-1.test-cluster-cassandra-dc1.default.svc.cluster.local
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
export PAUSE_REASON="pod is not paused"
`,
			},
			expectedError: nil,
		},
		{
			name: "region returns zero seeds",
			cc: &v1alpha1.CassandraCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int(3),
						},
						{
							Name:     "dc3",
							Replicas: proto.Int(3),
						},
					},
					HostPort: v1alpha1.HostPort{
						Enabled: true,
					},
					Ingress: v1alpha1.Ingress{
						Domain: "region2.ingress.domain",
						Secret: "region2.ingress.domain-secret",
					},
					ExternalRegions: v1alpha1.ExternalRegions{
						Managed: []v1alpha1.ManagedRegion{
							{
								Domain: "region1.ingress.domain",
							},
						},
					},
				},
			},
			externalRegionsSeeds: map[string][]string{
				"default-test-cluster-cassandra-prober.region1.ingress.domain": nil, //no seeds yet, should fail
			},
			externalRegionsReadiness: map[string]bool{},
			podList: &v1.PodList{
				Items: []v1.Pod{
					createTestPod("test-cluster-cassandra-dc1-0", ccNamespace, "uid1", "10.1.1.3", "node1", false, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-1", ccNamespace, "uid2", "10.1.1.4", "node2", false, cLabels(ccName, "dc1", true)),
					createTestPod("test-cluster-cassandra-dc1-2", ccNamespace, "uid3", "10.1.1.5", "node3", false, cLabels(ccName, "dc1", false)),
					createTestPod("test-cluster-cassandra-dc3-0", ccNamespace, "uid1", "10.1.2.3", "node1", false, cLabels(ccName, "dc3", true)),
					createTestPod("test-cluster-cassandra-dc3-1", ccNamespace, "uid2", "10.1.2.4", "node2", false, cLabels(ccName, "dc3", true)),
					createTestPod("test-cluster-cassandra-dc3-2", ccNamespace, "uid3", "10.1.2.5", "node3", false, cLabels(ccName, "dc3", false)),
				},
			},
			nodeList: &v1.NodeList{
				Items: []v1.Node{
					createTestNode("node1", "12.43.22.143", "54.32.141.231", nil),
					createTestNode("node2", "12.43.22.153", "54.32.141.232", nil),
					createTestNode("node3", "12.43.22.155", "54.32.141.233", nil),
				},
			},
			k8sLists: []client.ObjectList{
				&appsv1.StatefulSetList{
					Items: []appsv1.StatefulSet{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1",
								Namespace: "default",
							},
							Spec: appsv1.StatefulSetSpec{
								Replicas: proto.Int(3),
							},
							Status: appsv1.StatefulSetStatus{
								ReadyReplicas: 0,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc3",
								Namespace: "default",
							},
							Spec: appsv1.StatefulSetSpec{
								Replicas: proto.Int(3),
							},
							Status: appsv1.StatefulSetStatus{
								ReadyReplicas: 0,
							},
						},
					},
				},
			},
			expectedCMData: nil,
			expectedError:  errors.New("failed to get seeds list: One of the regions is not ready"),
		},
	}

	for _, c := range cases {
		t.Log(c.name)
		clietnBuilder := fake.NewClientBuilder().WithScheme(baseScheme)
		clietnBuilder.WithLists(c.k8sLists...)
		clietnBuilder.WithObjects(c.k8sObjects...)
		tClient := clietnBuilder.Build()

		mCtrl := gomock.NewController(t)
		proberClient := mocks.NewMockProberClient(mCtrl)
		proberClient.EXPECT().UpdateSeeds(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
		for region, seeds := range c.externalRegionsSeeds {
			proberClient.EXPECT().GetSeeds(gomock.Any(), region).Times(1).Return(seeds, nil)
		}
		for region, ready := range c.externalRegionsReadiness {
			proberClient.EXPECT().RegionReady(gomock.Any(), region).Times(1).Return(ready, nil)
		}

		reconciler := &CassandraClusterReconciler{
			Client: tClient,
			ProberClient: func(url *url.URL, user, password string) prober.ProberClient {
				return proberClient
			},
			Scheme: baseScheme,
			Events: events.NewEventRecorder(&record.FakeRecorder{}),
			Log:    zap.NewNop().Sugar(),
		}

		reconciler.defaultCassandraCluster(c.cc)

		cmData, err := reconciler.podsConfigMapData(context.Background(), c.cc, c.podList, c.nodeList, proberClient)
		if c.expectedError == nil {
			asserts.Expect(err).To(BeNil())
		} else {
			asserts.Expect(err.Error()).To(Equal(c.expectedError.Error()), "match error message")
		}
		asserts.Expect(cmData).To(BeEquivalentTo(c.expectedCMData), cmp.Diff(c.expectedCMData, cmData))
	}

}

func createTestPod(name, namespace, uid, ip, nodeName string, ready bool, labels map[string]string) v1.Pod {
	return v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
			UID:       types.UID(uid),
		},
		Spec: v1.PodSpec{
			NodeName: nodeName,
		},
		Status: v1.PodStatus{
			PodIP: ip,
			ContainerStatuses: []v1.ContainerStatus{
				{
					Name:  "cassandra",
					Ready: ready,
				},
			},
		},
	}
}

func createTestNode(name, internalIP, externalIP string, labels map[string]string) v1.Node {
	node := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{
				{
					Type:    v1.NodeInternalIP,
					Address: internalIP,
				},
			},
		},
	}

	if len(externalIP) > 0 {
		node.Status.Addresses = append(node.Status.Addresses, v1.NodeAddress{Address: externalIP, Type: v1.NodeExternalIP})
	}

	if len(labels) > 0 {
		node.Labels = labels
	}

	return node
}
