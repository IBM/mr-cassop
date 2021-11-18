package integration

import (
	"context"
	"strconv"

	v1 "k8s.io/api/core/v1"

	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/names"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("multiple regions", func() {
	externalRegions := []v1alpha1.ExternalRegion{
		{
			Domain: "domain1.external.com",
		},
		{
			Domain: "domain2.external.com",
		},
	}
	cc := &v1alpha1.CassandraCluster{
		ObjectMeta: cassandraObjectMeta,
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
			ImagePullSecretName: "pull-secret-name",
			AdminRoleSecretName: "admin-role",
			HostPort: v1alpha1.HostPort{
				Enabled: true,
			},
			Ingress: v1alpha1.Ingress{
				Domain:      "domain.internal.com",
				Secret:      "ingress-secret",
				Annotations: map[string]string{"ingress-class": "nginx"},
			},
			ExternalRegions: externalRegions,
		},
	}

	It("startup", func() {
		createAdminSecret(cc)
		Expect(k8sClient.Create(ctx, cc)).To(Succeed())
		mockProberClient.err = nil
		mockProberClient.readyClusters = make(map[string]bool)
		mockProberClient.seeds = make(map[string][]string)
		mockProberClient.dcs = make(map[string][]v1alpha1.DC)
		mockNodetoolClient.err = nil
		mockCQLClient.err = nil
		mockCQLClient.cassandraRoles = []cql.Role{{Role: "cassandra", Password: "cassandra", Login: true, Super: true}}
		mockCQLClient.keyspaces = []cql.Keyspace{{
			Name: "system_auth",
			Replication: map[string]string{
				"class": cql.ReplicationClassNetworkTopologyStrategy,
				"dc1":   "3",
			},
		}}

		for i, externalRegion := range cc.Spec.ExternalRegions {
			if len(externalRegion.Domain) != 0 {
				mockProberClient.readyClusters[names.ProberIngressDomain(cc.Name, externalRegion.Domain, cc.Namespace)] = false
				mockProberClient.seeds[names.ProberIngressDomain(cc.Name, externalRegion.Domain, cc.Namespace)] = []string{"13.432.13" + strconv.Itoa(i) + ".3", "13.432.13" + strconv.Itoa(i) + ".4"}
				mockProberClient.dcs[names.ProberIngressDomain(cc.Name, externalRegion.Domain, cc.Namespace)] = []v1alpha1.DC{
					{
						Name:     "ext-dc" + "-" + strconv.Itoa(i),
						Replicas: proto.Int32(3),
					},
				}
			}
		}

		mockProberClient.ready = true
		sts := &appsv1.StatefulSet{}
		for _, dc := range cc.Spec.DCs {
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{Name: names.DC(cc.Name, dc.Name), Namespace: cc.Namespace}, sts)
			}, mediumTimeout, mediumRetry).Should(Succeed())
		}

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cc.Name, Namespace: cc.Namespace}, cc)).To(Succeed())
		createCassandraPods(cc)
		markAllDCsReady(cc)

		podsConfigCM := &v1.ConfigMap{}
		Eventually(func() map[string]string {
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.PodsConfigConfigmap(cc.Name), Namespace: cc.Namespace}, podsConfigCM)).To(Succeed())
			return podsConfigCM.Data
		}, mediumTimeout, mediumRetry).ShouldNot(BeEmpty())
		//Expect(podsConfigCM.Data).ToNot(BeEmpty())
		for _, value := range podsConfigCM.Data {
			Expect(value).To(
				ContainSubstring("CASSANDRA_SEEDS=10.3.23.41,10.3.23.42,10.3.23.41,10.3.23.42,13.432.130.3,13.432.130.4,13.432.131.3,13.432.131.4"),
				"should include seeds from all regions")
			break
		}

		By("reaper shouldn't be deployed until all DCs ready")
		Consistently(func() error {
			return k8sClient.Get(context.Background(), types.NamespacedName{Name: names.ReaperDeployment(cc.Name, cc.Spec.DCs[0].Name), Namespace: cc.Namespace}, &appsv1.Deployment{})
		}, shortTimeout, shortRetry).ShouldNot(Succeed())

		By("reaper should be deployed after regions are ready")

		for _, externalRegion := range cc.Spec.ExternalRegions {
			if len(externalRegion.Domain) != 0 {
				mockProberClient.readyClusters[names.ProberIngressDomain(cc.Name, externalRegion.Domain, cc.Namespace)] = true
			}
		}

		for index, dc := range cc.Spec.DCs {
			// Check if first reaper deployment has been created
			if index == 0 {
				// Wait for the operator to create the first reaper deployment
				validateNumberOfDeployments(cc.Namespace, reaperDeploymentLabels, 1)
			}

			markDeploymentAsReady(types.NamespacedName{Name: names.ReaperDeployment(cc.Name, dc.Name), Namespace: cc.Namespace})
		}

		mockReaperClient.isRunning = true
		mockReaperClient.err = nil

		Eventually(func() bool {
			return mockReaperClient.clusterExists
		}, shortTimeout, shortRetry).Should(BeTrue())

		keyspaces, err := mockCQLClient.GetKeyspacesInfo()
		Expect(err).ToNot(HaveOccurred())
		Expect(keyspaces).To(BeEquivalentTo([]cql.Keyspace{
			{
				Name: "system_auth",
				Replication: map[string]string{
					"dc1":      "3",
					"dc2":      "3",
					"ext-dc-0": "3",
					"ext-dc-1": "3",
					"class":    cql.ReplicationClassNetworkTopologyStrategy,
				},
			},
		}))
	})
})
