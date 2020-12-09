package integration

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"time"
)

var _ = Describe("rf settings", func() {
	cc := &v1alpha1.CassandraCluster{
		ObjectMeta: cassandraObjectMeta,
		Spec: v1alpha1.CassandraClusterSpec{
			DCs: []v1alpha1.DC{
				{
					Name:     "dc1",
					Replicas: proto.Int32(3),
				},
			},
			Cassandra: v1alpha1.Cassandra{
				UsersDir: "/etc/cassandra-users",
				Auth: v1alpha1.CassandraAuth{
					User:     "cassandra",
					Password: "cassandra",
				},
				Image:           "cassandra/image",
				ImagePullPolicy: "Never",
			},
			Kwatcher: v1alpha1.Kwatcher{
				Enabled:         true,
				Image:           "kwatcher/image",
				ImagePullPolicy: "Never",
			},
			Prober: v1alpha1.Prober{
				Image:           "prober/image",
				ImagePullPolicy: "Never",
				ServerPort:      9090,
				Debug:           false,
				Jolokia: v1alpha1.Jolokia{
					Image:           "jolokia/image",
					ImagePullPolicy: "Never",
				},
			},
			Reaper: v1alpha1.Reaper{
				Image:           "reaper/image",
				ImagePullPolicy: "Never",
				Keyspace:        "test",
				DCs: []v1alpha1.DC{
					{
						Name:     "dc1",
						Replicas: proto.Int32(2),
					},
				},
			},
			Config: v1alpha1.Config{
				NumSeeds:     2,
				InternalAuth: true,
			},
			HostPort: v1alpha1.HostPort{
				Enabled: false,
			},
			CQLConfigMapLabelKey: "cql-cm",
			ImagePullSecretName:  "pull-secret-name",
			SystemKeyspaces: v1alpha1.SystemKeyspaces{
				Names: []string{"system_auth"},
				DCs: []v1alpha1.SystemKeyspaceDC{{
					Name: "dc1",
					RF:   2,
				}},
			},
		},
	}

	Context("if doesn't match the spec", func() {
		It("should be updated", func() {
			Expect(k8sClient.Create(ctx, cc)).To(Succeed())

			mockProberClient.err = nil
			mockProberClient.readyAllDCs = true
			mockProberClient.ready = true
			mockNodetoolClient.err = nil
			mockCQLClient.err = nil
			mockCQLClient.cassandraUsers = []cql.CassandraUser{{Role: "cassandra", IsSuperuser: true}}
			mockCQLClient.keyspaces = []cql.Keyspace{{
				Name: "system_auth",
				Replication: map[string]string{
					"class": "org.apache.cassandra.locator.SimpleTopologyStrategy",
				},
			}}

			Eventually(func() []cql.Keyspace {
				keyspaces, _ := mockCQLClient.GetKeyspacesInfo()
				return keyspaces
			}, time.Second*5, time.Millisecond*100).Should(Equal([]cql.Keyspace{{
				Name: "system_auth",
				Replication: map[string]string{
					"class": "org.apache.cassandra.locator.NetworkTopologyStrategy",
					"dc1":   "3",
				},
			}},
			))
		})
	})
})
