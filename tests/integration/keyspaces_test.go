package integration

import (
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("system keyspaces settings", func() {
	Context("if doesn't match the spec", func() {
		It("should be updated", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pull-secret-name",
					AdminRoleSecretName: "admin-role",
					SystemKeyspaces: v1alpha1.SystemKeyspaces{
						Keyspaces: []v1alpha1.KeyspaceName{"system_auth"},
						DCs: []v1alpha1.SystemKeyspaceDC{{
							Name: "dc1",
							RF:   3,
						}},
					},
				},
			}

			createReadyCluster(cc)
			mockCQLClient.keyspaces = []cql.Keyspace{{
				Name: "system_auth",
				Replication: map[string]string{
					"class": cql.ReplicationClassSimpleTopologyStrategy,
				},
			}}

			Eventually(mockCQLClient.GetKeyspacesInfo, time.Second*5, time.Millisecond*100).Should(Equal([]cql.Keyspace{{
				Name: "system_auth",
				Replication: map[string]string{
					"class": cql.ReplicationClassNetworkTopologyStrategy,
					"dc1":   "3",
				},
			}},
			))
		})
	})

	Context("that are not specified in the spec", func() {
		It("should not be updated", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pull-secret-name",
					AdminRoleSecretName: "admin-role",
					SystemKeyspaces: v1alpha1.SystemKeyspaces{
						Keyspaces: []v1alpha1.KeyspaceName{"system_auth", "system_traces"},
						DCs: []v1alpha1.SystemKeyspaceDC{{
							Name: "dc1",
							RF:   3,
						}},
					},
				},
			}

			createReadyCluster(cc)
			mockCQLClient.keyspaces = []cql.Keyspace{
				{
					Name: "system_auth",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
				{
					Name: "system",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
				{
					Name: "system_traces",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
			}

			Eventually(mockCQLClient.GetKeyspacesInfo, time.Second*15, time.Millisecond*100).Should(ContainElements([]cql.Keyspace{
				{
					Name: "system_auth",
					Replication: map[string]string{
						"class": cql.ReplicationClassNetworkTopologyStrategy,
						"dc1":   "3",
					},
				},
				{
					Name: "system_traces",
					Replication: map[string]string{
						"class": cql.ReplicationClassNetworkTopologyStrategy,
						"dc1":   "3",
					},
				},
				{
					Name: "system",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
			},
			))
		})
	})

	Context("with non existing keyspace specified", func() {
		It("should be skipped and still update existing", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pull-secret-name",
					AdminRoleSecretName: "admin-role",
					SystemKeyspaces: v1alpha1.SystemKeyspaces{
						Keyspaces: []v1alpha1.KeyspaceName{"system_auth", "system_traces", "non_existing"},
						DCs: []v1alpha1.SystemKeyspaceDC{{
							Name: "dc1",
							RF:   3,
						}},
					},
				},
			}

			createReadyCluster(cc)
			mockCQLClient.keyspaces = []cql.Keyspace{
				{
					Name: "system_auth",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
				{
					Name: "system",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
				{
					Name: "system_traces",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
			}

			Eventually(func() []cql.Keyspace {
				keyspaces, _ := mockCQLClient.GetKeyspacesInfo()
				return keyspaces
			}, time.Second*5, time.Millisecond*100).Should(ContainElements([]cql.Keyspace{
				{
					Name: "system_auth",
					Replication: map[string]string{
						"class": cql.ReplicationClassNetworkTopologyStrategy,
						"dc1":   "3",
					},
				},
				{
					Name: "system_traces",
					Replication: map[string]string{
						"class": cql.ReplicationClassNetworkTopologyStrategy,
						"dc1":   "3",
					},
				},
				{
					Name: "system",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
			},
			))
		})
	})

	Context("with settings matching the spec", func() {
		It("should not be updated", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pull-secret-name",
					AdminRoleSecretName: "admin-role",
					SystemKeyspaces: v1alpha1.SystemKeyspaces{
						Keyspaces: []v1alpha1.KeyspaceName{"system_auth", "system_traces"},
						DCs: []v1alpha1.SystemKeyspaceDC{{
							Name: "dc1",
							RF:   3,
						}},
					},
				},
			}

			createReadyCluster(cc)
			mockCQLClient.keyspaces = []cql.Keyspace{
				{
					Name: "system_auth",
					Replication: map[string]string{
						"class": cql.ReplicationClassNetworkTopologyStrategy,
						"dc1":   "3",
					},
				},
				{
					Name: "system",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
				{
					Name: "system_traces",
					Replication: map[string]string{
						"class": cql.ReplicationClassNetworkTopologyStrategy,
						"dc1":   "3",
					},
				},
			}

			Eventually(func() []cql.Keyspace {
				keyspaces, _ := mockCQLClient.GetKeyspacesInfo()
				return keyspaces
			}, time.Second*5, time.Millisecond*100).Should(ContainElements([]cql.Keyspace{
				{
					Name: "system_auth",
					Replication: map[string]string{
						"class": cql.ReplicationClassNetworkTopologyStrategy,
						"dc1":   "3",
					},
				},
				{
					Name: "system_traces",
					Replication: map[string]string{
						"class": cql.ReplicationClassNetworkTopologyStrategy,
						"dc1":   "3",
					},
				},
				{
					Name: "system",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
			},
			))
		})
	})

	Context("with no keyspaces specified", func() {
		It("should still update `system_auth`, `system_traces` and `system_distributed` keyspaces", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(4),
						},
					},
					ImagePullSecretName: "pull-secret-name",
					AdminRoleSecretName: "admin-role",
				},
			}

			createReadyCluster(cc)
			mockCQLClient.keyspaces = []cql.Keyspace{
				{
					Name: "system_auth",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
				{
					Name: "system",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
				{
					Name: "system_traces",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
				{
					Name: "system_distributed",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
			}

			Eventually(mockCQLClient.GetKeyspacesInfo, time.Second*5, time.Millisecond*100).Should(ContainElements([]cql.Keyspace{
				{
					Name: "system_auth",
					Replication: map[string]string{
						"class": cql.ReplicationClassNetworkTopologyStrategy,
						"dc1":   "3",
					},
				},
				{
					Name: "system_traces",
					Replication: map[string]string{
						"class": cql.ReplicationClassNetworkTopologyStrategy,
						"dc1":   "3",
					},
				},
				{
					Name: "system_distributed",
					Replication: map[string]string{
						"class": cql.ReplicationClassNetworkTopologyStrategy,
						"dc1":   "3",
					},
				},
				{
					Name: "system",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
			},
			))
		})
	})

	Context("with only `system_trace` keyspace specified", func() {
		It("should default `system_auth` + `system_distributed`, and override `system_traces`", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(4),
						},
					},
					SystemKeyspaces: v1alpha1.SystemKeyspaces{
						Keyspaces: []v1alpha1.KeyspaceName{
							"system_traces",
						},
						DCs: []v1alpha1.SystemKeyspaceDC{
							{
								Name: "dc1",
								RF:   3,
							},
							{
								Name: "dc2",
								RF:   3,
							},
						},
					},
					ImagePullSecretName: "pull-secret-name",
					AdminRoleSecretName: "admin-role",
				},
			}

			createReadyCluster(cc)
			mockCQLClient.keyspaces = []cql.Keyspace{
				{
					Name: "system_auth",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
				{
					Name: "system",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
				{
					Name: "system_traces",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
				{
					Name: "system_distributed",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
			}

			Eventually(mockCQLClient.GetKeyspacesInfo, time.Second*5, time.Millisecond*100).Should(ContainElements([]cql.Keyspace{
				{
					Name: "system_auth",
					Replication: map[string]string{
						"class": cql.ReplicationClassNetworkTopologyStrategy,
						"dc1":   "3",
					},
				},
				{
					Name: "system_traces",
					Replication: map[string]string{
						"class": cql.ReplicationClassNetworkTopologyStrategy,
						"dc1":   "3",
						"dc2":   "3",
					},
				},
				{
					Name: "system_distributed",
					Replication: map[string]string{
						"class": cql.ReplicationClassNetworkTopologyStrategy,
						"dc1":   "3",
					},
				},
				{
					Name: "system",
					Replication: map[string]string{
						"class": cql.ReplicationClassSimpleTopologyStrategy,
					},
				},
			},
			))
		})
	})
})
