package controllers

import (
	"context"
	"net/url"
	"testing"

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
	readyStatefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-cassandra-dc1",
			Namespace: "default",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: proto.Int32(3),
		},
		Status: appsv1.StatefulSetStatus{
			Replicas:      3,
			ReadyReplicas: 3,
		},
	}

	asserts := NewGomegaWithT(t)
	cases := []struct {
		name                 string
		k8sObjects           []client.Object
		k8sLists             []client.ObjectList
		cc                   *v1alpha1.CassandraCluster
		externalSeeds        map[string][]string
		expectedCMData       map[string]string
		externalDCsSeeds     map[string][]string
		externalDCsReadiness map[string]bool
		expectedError        error
	}{
		{
			name: "simple case with hostport disabled",
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
				},
			},
			k8sLists: []client.ObjectList{
				&v1.PodList{
					Items: []v1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-0",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid1",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.3",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-1",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid2",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.4",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-2",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid3",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.5",
							},
						},
					},
				},
			},
			k8sObjects: []client.Object{readyStatefulSet},
			expectedCMData: map[string]string{
				"test-cluster-cassandra-dc1-0_uid1.sh": `export CASSANDRA_BROADCAST_ADDRESS=10.1.1.3
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.3
export CASSANDRA_SEEDS=test-cluster-cassandra-dc1-0.test-cluster-cassandra-dc1.default.svc.cluster.local,test-cluster-cassandra-dc1-1.test-cluster-cassandra-dc1.default.svc.cluster.local
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
`,
				"test-cluster-cassandra-dc1-1_uid2.sh": `export CASSANDRA_BROADCAST_ADDRESS=10.1.1.4
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.4
export CASSANDRA_SEEDS=test-cluster-cassandra-dc1-0.test-cluster-cassandra-dc1.default.svc.cluster.local,test-cluster-cassandra-dc1-1.test-cluster-cassandra-dc1.default.svc.cluster.local
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
`,
				"test-cluster-cassandra-dc1-2_uid3.sh": `export CASSANDRA_BROADCAST_ADDRESS=10.1.1.5
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.5
export CASSANDRA_SEEDS=test-cluster-cassandra-dc1-0.test-cluster-cassandra-dc1.default.svc.cluster.local,test-cluster-cassandra-dc1-1.test-cluster-cassandra-dc1.default.svc.cluster.local
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
`,
			},
			expectedError: nil,
		},
		{
			name: "with not found no error should be returned",
			cc: &v1alpha1.CassandraCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
			},
			k8sLists:       []client.ObjectList{},
			expectedCMData: nil,
			expectedError:  nil,
		},
		{
			name: "hostport enabled, no external DC domains",
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
				},
			},
			k8sObjects: []client.Object{readyStatefulSet},
			k8sLists: []client.ObjectList{
				&v1.PodList{
					Items: []v1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-0",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid1",
							},
							Spec: v1.PodSpec{
								NodeName: "node1",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.3",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-1",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid2",
							},
							Spec: v1.PodSpec{
								NodeName: "node2",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.4",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-2",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid3",
							},
							Spec: v1.PodSpec{
								NodeName: "node3",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.5",
							},
						},
					},
				},
				&v1.NodeList{
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
								},
							},
						},
					},
				},
			},
			expectedCMData: map[string]string{
				`test-cluster-cassandra-dc1-0_uid1.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.143
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.3
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
`,
				`test-cluster-cassandra-dc1-1_uid2.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.153
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.4
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
`,
				`test-cluster-cassandra-dc1-2_uid3.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.142
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.5
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
`,
			},
			expectedError: nil,
		},
		{
			name: "hostport enabled, use external IP",
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
						Enabled:           true,
						UseExternalHostIP: true,
					},
				},
			},
			k8sObjects: []client.Object{readyStatefulSet},
			k8sLists: []client.ObjectList{
				&v1.PodList{
					Items: []v1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-0",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid1",
							},
							Spec: v1.PodSpec{
								NodeName: "node1",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.3",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-1",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid2",
							},
							Spec: v1.PodSpec{
								NodeName: "node2",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.4",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-2",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid3",
							},
							Spec: v1.PodSpec{
								NodeName: "node3",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.5",
							},
						},
					},
				},
				&v1.NodeList{
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
			},
			expectedCMData: map[string]string{
				`test-cluster-cassandra-dc1-0_uid1.sh`: `export CASSANDRA_BROADCAST_ADDRESS=54.32.141.231
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.3
export CASSANDRA_SEEDS=54.32.141.231,54.32.141.232
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
`,
				`test-cluster-cassandra-dc1-1_uid2.sh`: `export CASSANDRA_BROADCAST_ADDRESS=54.32.141.232
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.4
export CASSANDRA_SEEDS=54.32.141.231,54.32.141.232
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
`,
				`test-cluster-cassandra-dc1-2_uid3.sh`: `export CASSANDRA_BROADCAST_ADDRESS=54.32.141.233
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.5
export CASSANDRA_SEEDS=54.32.141.231,54.32.141.232
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
`,
			},
			expectedError: nil,
		},
		{
			name: "ErrPodNotScheduled error if pod has nodename empty",
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
						Enabled:           true,
						UseExternalHostIP: true,
					},
				},
			},
			k8sLists: []client.ObjectList{
				&v1.PodList{
					Items: []v1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-0",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid1",
							},
							Spec: v1.PodSpec{
								NodeName: "node1",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.3",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-1",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid2",
							},
							Spec: v1.PodSpec{
								NodeName: "node2",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.4",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-2",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid3",
							},
							Spec: v1.PodSpec{
								NodeName: "",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.5",
							},
						},
					},
				},
				&v1.NodeList{
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
			},
			expectedCMData: nil,
			expectedError:  ErrPodNotScheduled,
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
			k8sLists: []client.ObjectList{
				&v1.PodList{
					Items: []v1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-0",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid1",
							},
							Spec: v1.PodSpec{
								NodeName: "node1",
							},
							Status: v1.PodStatus{
								PodIP: "", // not scheduled yet
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-1",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid2",
							},
							Spec: v1.PodSpec{
								NodeName: "node2",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.4",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-2",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid3",
							},
							Spec: v1.PodSpec{
								NodeName: "node3",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.5",
							},
						},
					},
				},
				&v1.NodeList{
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
			},
			expectedCMData: nil,
			expectedError:  ErrPodNotScheduled,
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
					ExternalRegions: []v1alpha1.ExternalRegion{
						{
							Domain: "region2.ingress.domain",
						},
					},
				},
			},
			externalDCsReadiness: map[string]bool{
				"default-test-cluster-cassandra-prober.region2.ingress.domain": true,
			},
			externalDCsSeeds: map[string][]string{
				"default-test-cluster-cassandra-prober.region2.ingress.domain": {"42.32.34.111", "42.32.34.113"},
			},
			k8sLists: []client.ObjectList{
				&v1.PodList{
					Items: []v1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-0",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid1",
							},
							Spec: v1.PodSpec{
								NodeName: "node1",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.3",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-1",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid2",
							},
							Spec: v1.PodSpec{
								NodeName: "node2",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.4",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-2",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid3",
							},
							Spec: v1.PodSpec{
								NodeName: "node3",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.5",
							},
						},
					},
				},
				&v1.NodeList{
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
`,
				`test-cluster-cassandra-dc1-1_uid2.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.153
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.4
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
`,
				`test-cluster-cassandra-dc1-2_uid3.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.142
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.5
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
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
					ExternalRegions: []v1alpha1.ExternalRegion{
						{
							Domain: "region2.ingress.domain",
						},
					},
				},
			},
			externalDCsReadiness: map[string]bool{
				"default-test-cluster-cassandra-prober.region2.ingress.domain": false,
			},
			externalDCsSeeds: map[string][]string{
				"default-test-cluster-cassandra-prober.region2.ingress.domain": {"42.32.34.111", "42.32.34.113"},
			},
			k8sLists: []client.ObjectList{
				&v1.PodList{
					Items: []v1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-0",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc1",
								},
								UID: "uid1",
							},
							Spec: v1.PodSpec{
								NodeName: "node1",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.3",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-1",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc1",
								},
								UID: "uid2",
							},
							Spec: v1.PodSpec{
								NodeName: "node2",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.4",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-2",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc1",
								},
								UID: "uid3",
							},
							Spec: v1.PodSpec{
								NodeName: "node3",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.5",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc3-0",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc3",
								},
								UID: "uid1",
							},
							Spec: v1.PodSpec{
								NodeName: "node1",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.2.3",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc3-1",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc3",
								},
								UID: "uid2",
							},
							Spec: v1.PodSpec{
								NodeName: "node2",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.2.4",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc3-2",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc3",
								},
								UID: "uid3",
							},
							Spec: v1.PodSpec{
								NodeName: "node3",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.2.5",
							},
						},
					},
				},
				&v1.NodeList{
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
				`test-cluster-cassandra-dc1-2_uid3.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.142
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.5
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
`,
				`test-cluster-cassandra-dc3-0_uid1.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.143
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.2.3
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
`,
				`test-cluster-cassandra-dc3-1_uid2.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.153
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.2.4
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
`,
				`test-cluster-cassandra-dc3-2_uid3.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.142
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.2.5
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
`,
				`test-cluster-cassandra-dc1-0_uid1.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.143
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.3
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
`,
				`test-cluster-cassandra-dc1-1_uid2.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.153
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.4
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
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
					ExternalRegions: []v1alpha1.ExternalRegion{
						{
							Domain: "region2.ingress.domain",
						},
					},
				},
			},
			externalDCsReadiness: map[string]bool{
				"default-test-cluster-cassandra-prober.region2.ingress.domain": false,
			},
			externalDCsSeeds: map[string][]string{
				"default-test-cluster-cassandra-prober.region2.ingress.domain": {"42.32.34.111", "42.32.34.113"},
			},
			k8sLists: []client.ObjectList{
				&v1.PodList{
					Items: []v1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-0",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc1",
								},
								UID: "uid1",
							},
							Spec: v1.PodSpec{
								NodeName: "node1",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.3",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-1",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc1",
								},
								UID: "uid2",
							},
							Spec: v1.PodSpec{
								NodeName: "node2",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.4",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-2",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc1",
								},
								UID: "uid3",
							},
							Spec: v1.PodSpec{
								NodeName: "node3",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.5",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc3-0",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc3",
								},
								UID: "uid1",
							},
							Spec: v1.PodSpec{
								NodeName: "node1",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.2.3",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc3-1",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc3",
								},
								UID: "uid2",
							},
							Spec: v1.PodSpec{
								NodeName: "node2",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.2.4",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc3-2",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc3",
								},
								UID: "uid3",
							},
							Spec: v1.PodSpec{
								NodeName: "node3",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.2.5",
							},
						},
					},
				},
				&v1.NodeList{
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
`,
				`test-cluster-cassandra-dc3-0_uid1.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.143
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.2.3
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
`,
				`test-cluster-cassandra-dc3-1_uid2.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.153
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.2.4
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
`,
				`test-cluster-cassandra-dc3-2_uid3.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.142
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.2.5
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
`,
				`test-cluster-cassandra-dc1-0_uid1.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.143
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.3
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
`,
				`test-cluster-cassandra-dc1-1_uid2.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.153
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.4
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
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
					ExternalRegions: []v1alpha1.ExternalRegion{
						{
							Domain: "region1.ingress.domain",
						},
					},
				},
			},
			externalDCsReadiness: map[string]bool{
				"default-test-cluster-cassandra-prober.region1.ingress.domain": false,
			},
			externalDCsSeeds: map[string][]string{
				"default-test-cluster-cassandra-prober.region1.ingress.domain": {"42.32.34.111", "42.32.34.113"},
			},
			k8sLists: []client.ObjectList{
				&v1.PodList{
					Items: []v1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-0",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc1",
								},
								UID: "uid1",
							},
							Spec: v1.PodSpec{
								NodeName: "node1",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.3",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-1",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc1",
								},
								UID: "uid2",
							},
							Spec: v1.PodSpec{
								NodeName: "node2",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.4",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-2",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc1",
								},
								UID: "uid3",
							},
							Spec: v1.PodSpec{
								NodeName: "node3",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.5",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc3-0",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc3",
								},
								UID: "uid1",
							},
							Spec: v1.PodSpec{
								NodeName: "node1",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.2.3",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc3-1",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc3",
								},
								UID: "uid2",
							},
							Spec: v1.PodSpec{
								NodeName: "node2",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.2.4",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc3-2",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc3",
								},
								UID: "uid3",
							},
							Spec: v1.PodSpec{
								NodeName: "node3",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.2.5",
							},
						},
					},
				},
				&v1.NodeList{
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
`,
				`test-cluster-cassandra-dc3-0_uid1.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.143
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.2.3
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
`,
				`test-cluster-cassandra-dc3-1_uid2.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.153
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.2.4
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
`,
				`test-cluster-cassandra-dc3-2_uid3.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.142
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.2.5
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
`,
				`test-cluster-cassandra-dc1-0_uid1.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.143
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.3
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
`,
				`test-cluster-cassandra-dc1-1_uid2.sh`: `export CASSANDRA_BROADCAST_ADDRESS=12.43.22.153
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.4
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153,12.43.22.143,12.43.22.153,42.32.34.111,42.32.34.113
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=true
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
					ExternalRegions: []v1alpha1.ExternalRegion{
						{
							Domain: "region1.ingress.domain",
						},
					},
				},
			},
			k8sLists: []client.ObjectList{
				&v1.PodList{
					Items: []v1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-0",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc1",
								},
								UID: "uid1",
							},
							Spec: v1.PodSpec{
								NodeName: "node1",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.3",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-1",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc1",
								},
								UID: "uid2",
							},
							Spec: v1.PodSpec{
								NodeName: "node2",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.4",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-2",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc1",
								},
								UID: "uid3",
							},
							Spec: v1.PodSpec{
								NodeName: "node3",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.5",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc3-0",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc3",
								},
								UID: "uid1",
							},
							Spec: v1.PodSpec{
								NodeName: "node1",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.2.3",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc3-1",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc3",
								},
								UID: "uid2",
							},
							Spec: v1.PodSpec{
								NodeName: "node2",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.2.4",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc3-2",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
									v1alpha1.CassandraClusterDC:        "dc3",
								},
								UID: "uid3",
							},
							Spec: v1.PodSpec{
								NodeName: "node3",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.2.5",
							},
						},
					},
				},
				&v1.NodeList{
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
						// node3 is missing
					},
				},
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
			expectedError:  errors.New("Cannot get node: node3"),
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
						Enabled: true,
					},
					Cassandra: &v1alpha1.Cassandra{
						ZonesAsRacks: true,
					},
				},
			},
			k8sObjects: []client.Object{readyStatefulSet},
			k8sLists: []client.ObjectList{
				&v1.PodList{
					Items: []v1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-0",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid1",
							},
							Spec: v1.PodSpec{
								NodeName: "node1",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.3",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-1",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid2",
							},
							Spec: v1.PodSpec{
								NodeName: "node2",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.4",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster-cassandra-dc1-2",
								Namespace: "default",
								Labels: map[string]string{
									v1alpha1.CassandraClusterInstance:  "test-cluster",
									v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
								},
								UID: "uid3",
							},
							Spec: v1.PodSpec{
								NodeName: "node3",
							},
							Status: v1.PodStatus{
								PodIP: "10.1.1.5",
							},
						},
					},
				},
				&v1.NodeList{
					Items: []v1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node1",
								Labels: map[string]string{
									v1.LabelTopologyZone: "zone1",
								},
							},
							Status: v1.NodeStatus{
								Addresses: []v1.NodeAddress{
									{
										Type:    v1.NodeInternalIP,
										Address: "12.43.22.143",
									},
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node2",
								Labels: map[string]string{
									v1.LabelTopologyZone: "zone1",
								},
							},
							Status: v1.NodeStatus{
								Addresses: []v1.NodeAddress{
									{
										Type:    v1.NodeInternalIP,
										Address: "12.43.22.153",
									},
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node3",
								Labels: map[string]string{
									v1.LabelTopologyZone: "zone2",
								},
							},
							Status: v1.NodeStatus{
								Addresses: []v1.NodeAddress{
									{
										Type:    v1.NodeInternalIP,
										Address: "12.43.22.142",
									},
								},
							},
						},
					},
				},
			},
			expectedCMData: map[string]string{
				`test-cluster-cassandra-dc1-0_uid1.sh`: `export CASSANDRA_RACK=zone1
export CASSANDRA_ENDPOINT_SNITCH=GossipingPropertyFileSnitch
export CASSANDRA_BROADCAST_ADDRESS=12.43.22.143
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.3
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
`,
				`test-cluster-cassandra-dc1-1_uid2.sh`: `export CASSANDRA_RACK=zone1
export CASSANDRA_ENDPOINT_SNITCH=GossipingPropertyFileSnitch
export CASSANDRA_BROADCAST_ADDRESS=12.43.22.153
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.4
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
`,
				`test-cluster-cassandra-dc1-2_uid3.sh`: `export CASSANDRA_RACK=zone2
export CASSANDRA_ENDPOINT_SNITCH=GossipingPropertyFileSnitch
export CASSANDRA_BROADCAST_ADDRESS=12.43.22.142
export CASSANDRA_BROADCAST_RPC_ADDRESS=10.1.1.5
export CASSANDRA_SEEDS=12.43.22.143,12.43.22.153
export CASSANDRA_NODE_PREVIOUS_IP=
export PAUSE_INIT=false
`,
			},
			expectedError: nil,
		},
	}

	for _, c := range cases {
		clietnBuilder := fake.NewClientBuilder().WithScheme(baseScheme)
		clietnBuilder.WithLists(c.k8sLists...)
		clietnBuilder.WithObjects(c.k8sObjects...)
		tClient := clietnBuilder.Build()

		mCtrl := gomock.NewController(t)
		proberClient := mocks.NewMockProberClient(mCtrl)
		proberClient.EXPECT().UpdateSeeds(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
		for region, seeds := range c.externalDCsSeeds {
			proberClient.EXPECT().GetSeeds(gomock.Any(), region).Times(1).Return(seeds, nil)
		}
		for region, ready := range c.externalDCsReadiness {
			proberClient.EXPECT().DCsReady(gomock.Any(), region).Times(1).Return(ready, nil)
		}

		reconciler := &CassandraClusterReconciler{
			Client: tClient,
			ProberClient: func(url *url.URL) prober.ProberClient {
				return proberClient
			},
			Scheme: baseScheme,
			Log:    zap.NewNop().Sugar(),
		}

		reconciler.defaultCassandraCluster(c.cc)

		cmData, err := reconciler.podsConfigMapData(context.Background(), c.cc, proberClient)
		if c.expectedError == nil {
			asserts.Expect(err).To(BeNil(), c.name)
		} else {
			asserts.Expect(err).To(MatchError(err), c.name)
		}
		asserts.Expect(cmData).To(BeEquivalentTo(c.expectedCMData), c.name)
	}

}
