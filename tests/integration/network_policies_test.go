package integration

import (
	"fmt"
	"github.com/gogo/protobuf/proto"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	nwv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var protocolTCP = v1.ProtocolTCP

var _ = Describe("network policies", func() {
	Context("when network policies are enabled", func() {
		It("network policies should be created", func() {
			nodeIP := nodeIPs[0]
			managedRegionNodeIP := "10.10.10.10"
			extraCassandraIps := []string{"10.10.10.1", "10.10.10.2"}
			cc := &dbv1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: dbv1alpha1.CassandraClusterSpec{
					DCs: []dbv1alpha1.DC{
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
					Cassandra: &dbv1alpha1.Cassandra{
						Monitoring: dbv1alpha1.Monitoring{Enabled: true, Agent: dbv1alpha1.CassandraAgentTlp},
					},
					AdminRoleSecretName: "admin-role",
					Prober:              dbv1alpha1.Prober{ServiceMonitor: dbv1alpha1.ServiceMonitor{Enabled: true}},
					Reaper:              &dbv1alpha1.Reaper{ServiceMonitor: dbv1alpha1.ServiceMonitor{Enabled: true}},
					HostPort:            dbv1alpha1.HostPort{Enabled: true},
					Ingress: dbv1alpha1.Ingress{
						Domain: "domain1.external.com",
						Secret: "secret1",
					},
					ExternalRegions: dbv1alpha1.ExternalRegions{
						Managed: []dbv1alpha1.ManagedRegion{
							{
								Domain: "domain2.external.com",
							},
						},
					},
					NetworkPolicies: dbv1alpha1.NetworkPolicies{
						Enabled: true,
						ExtraIngressRules: []dbv1alpha1.NetworkPolicyRule{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"kubernetes-custom-component": "custom-ingress-controller"},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"kubernetes.io/metadata.name": "ingress-namespace"},
								},
							},
						},
						ExtraPrometheusRules: []dbv1alpha1.NetworkPolicyRule{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"app.kubernetes.io/name": "prometheus-operator"},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"kubernetes.io/metadata.name": "prometheus-namespace"},
								},
							},
						},
						ExtraCassandraRules: []dbv1alpha1.NetworkPolicyRule{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"app.kubernetes.io/instance": "accounting1"},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"kubernetes.io/metadata.name": "namespace1"},
								},
							},
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"app.kubernetes.io/instance": "accounting2"},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"kubernetes.io/metadata.name": "namespace2"},
								},
								Ports: []int32{dbv1alpha1.ThriftPort, dbv1alpha1.TlsPort},
							},
						},
						ExtraCassandraIPs: extraCassandraIps,
					},
				},
			}

			mockProberClient.readyClusters = make(map[string]bool)
			mockProberClient.seeds = make(map[string][]string)
			mockProberClient.dcs = make(map[string][]dbv1alpha1.DC)
			mockProberClient.regionIps = make(map[string][]string)

			for i, managedRegion := range cc.Spec.ExternalRegions.Managed {
				ingressHost := names.ProberIngressDomain(cc, managedRegion)
				mockProberClient.readyClusters[ingressHost] = false
				mockProberClient.seeds[ingressHost] = []string{"13.432.13" + strconv.Itoa(i) + ".3", "13.432.13" + strconv.Itoa(i) + ".4"}
				mockProberClient.dcs[ingressHost] = []dbv1alpha1.DC{
					{
						Name:     "ext-dc" + "-" + strconv.Itoa(i),
						Replicas: proto.Int32(3),
					},
				}
				mockProberClient.regionIps[ingressHost] = []string{managedRegionNodeIP}

			}

			mockProberClient.ready = true

			for _, managedRegion := range cc.Spec.ExternalRegions.Managed {
				mockProberClient.readyClusters[names.ProberIngressDomain(cc, managedRegion)] = true
			}

			createReadyCluster(cc)

			Eventually(func() int {
				netPolList := &nwv1.NetworkPolicyList{}
				Expect(k8sClient.List(ctx, netPolList, client.InNamespace(cc.Namespace), client.MatchingLabels(labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterNetworkPolicy))))
				return len(netPolList.Items)
			}, mediumTimeout, mediumRetry).Should(BeEquivalentTo(9))

			netPol := &nwv1.NetworkPolicy{}

			By("Checking Cassandra Cluster network policy")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.CassandraClusterNetworkPolicyName(cc.Name), Namespace: cc.Namespace}, netPol)).To(Succeed())

			expectedCasNetPolSpec := nwv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentCassandra},
				},
				Ingress: []nwv1.NetworkPolicyIngressRule{
					{
						Ports: []nwv1.NetworkPolicyPort{
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.TlsPort},
								Protocol: &protocolTCP,
							},
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.IntraPort},
								Protocol: &protocolTCP,
							},
						},
						From: []nwv1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentCassandra},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{v1.LabelMetadataName: cc.Namespace},
								},
							},
						},
					},
					{
						Ports: []nwv1.NetworkPolicyPort{
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.CqlPort},
								Protocol: &protocolTCP,
							},
						},
						From: []nwv1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: dbv1alpha1.CassandraOperatorPodLabels,
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{v1.LabelMetadataName: operatorConfig.Namespace},
								},
							},
						},
					},
					{
						Ports: []nwv1.NetworkPolicyPort{
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.JmxPort},
								Protocol: &protocolTCP,
							},
						},
						From: []nwv1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentProber},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{v1.LabelMetadataName: cc.Namespace},
								},
							},
						},
					},
					{
						Ports: []nwv1.NetworkPolicyPort{
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.CqlPort},
								Protocol: &protocolTCP,
							},

							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.JmxPort},
								Protocol: &protocolTCP,
							},
						},
						From: []nwv1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentReaper},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{v1.LabelMetadataName: cc.Namespace},
								},
							},
						},
					},
				},
				PolicyTypes: []nwv1.PolicyType{"Ingress"},
			}

			Expect(netPol.Spec).To(BeEquivalentTo(expectedCasNetPolSpec))

			By("Checking Cassandra HostPort network policy")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.CassandraHostPortPolicyName(cc.Name), Namespace: cc.Namespace}, netPol)).To(Succeed())

			expectedCasNetPolSpec = nwv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentCassandra},
				},
				Ingress: []nwv1.NetworkPolicyIngressRule{
					{
						Ports: []nwv1.NetworkPolicyPort{
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.TlsPort},
								Protocol: &protocolTCP,
							},
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.IntraPort},
								Protocol: &protocolTCP,
							},
						},
						From: []nwv1.NetworkPolicyPeer{
							{
								IPBlock: &nwv1.IPBlock{CIDR: fmt.Sprintf("%s/32", nodeIP)},
							},
						},
					},
				},
				PolicyTypes: []nwv1.PolicyType{"Ingress"},
			}

			Expect(netPol.Spec).To(BeEquivalentTo(expectedCasNetPolSpec))

			By("Checking Cassandra HostPort Reaper network policy")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.CassandraHostPortReaperPolicyName(cc.Name), Namespace: cc.Namespace}, netPol)).To(Succeed())

			expectedCasNetPolSpec = nwv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentCassandra},
				},
				Ingress: []nwv1.NetworkPolicyIngressRule{
					{
						Ports: []nwv1.NetworkPolicyPort{
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.JmxPort},
								Protocol: &protocolTCP,
							},
						},
						From: []nwv1.NetworkPolicyPeer{
							{
								IPBlock: &nwv1.IPBlock{CIDR: fmt.Sprintf("%s/32", nodeIP)},
							},
						},
					},
				},
				PolicyTypes: []nwv1.PolicyType{"Ingress"},
			}

			Expect(netPol.Spec).To(BeEquivalentTo(expectedCasNetPolSpec))

			By("Checking Cassandra External Managed Regions network policy")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.CassandraExternalManagedRegionsPolicyName(cc.Name), Namespace: cc.Namespace}, netPol)).To(Succeed())

			expectedCasNetPolSpec = nwv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentCassandra},
				},
				Ingress: []nwv1.NetworkPolicyIngressRule{
					{
						Ports: []nwv1.NetworkPolicyPort{
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.TlsPort},
								Protocol: &protocolTCP,
							},
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.IntraPort},
								Protocol: &protocolTCP,
							},
						},
						From: []nwv1.NetworkPolicyPeer{
							{
								IPBlock: &nwv1.IPBlock{CIDR: fmt.Sprintf("%s/32", managedRegionNodeIP)},
							},
						},
					},
				},
				PolicyTypes: []nwv1.PolicyType{"Ingress"},
			}

			Expect(netPol.Spec).To(BeEquivalentTo(expectedCasNetPolSpec))

			By("Checking Cassandra Extra Rules network policy")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.CassandraExtraRulesPolicyName(cc.Name), Namespace: cc.Namespace}, netPol)).To(Succeed())

			expectedCasNetPolSpec = nwv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentCassandra},
				},
				Ingress: []nwv1.NetworkPolicyIngressRule{
					{
						Ports: []nwv1.NetworkPolicyPort{
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.CqlPort},
								Protocol: &protocolTCP,
							},
						},
						From: []nwv1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"app.kubernetes.io/instance": "accounting1"},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"kubernetes.io/metadata.name": "namespace1"},
								},
							},
						},
					},
					{
						Ports: []nwv1.NetworkPolicyPort{
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.ThriftPort},
								Protocol: &protocolTCP,
							},
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.TlsPort},
								Protocol: &protocolTCP,
							},
						},
						From: []nwv1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"app.kubernetes.io/instance": "accounting2"},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"kubernetes.io/metadata.name": "namespace2"},
								},
							},
						},
					},
				},
				PolicyTypes: []nwv1.PolicyType{"Ingress"},
			}

			Expect(netPol.Spec).To(BeEquivalentTo(expectedCasNetPolSpec))

			By("Checking Cassandra Extra Prometheus network policy")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.CassandraExtraPrometheusRulesPolicyName(cc.Name), Namespace: cc.Namespace}, netPol)).To(Succeed())

			expectedCasNetPolSpec = nwv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentCassandra},
				},
				Ingress: []nwv1.NetworkPolicyIngressRule{
					{
						Ports: []nwv1.NetworkPolicyPort{
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.TlpPort},
								Protocol: &protocolTCP,
							},
						},
						From: []nwv1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"app.kubernetes.io/name": "prometheus-operator"},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"kubernetes.io/metadata.name": "prometheus-namespace"},
								},
							},
						},
					},
				},
				PolicyTypes: []nwv1.PolicyType{"Ingress"},
			}

			Expect(netPol.Spec).To(BeEquivalentTo(expectedCasNetPolSpec))

			By("Checking Cassandra Extra IPs network policy")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.CassandraExtraIpsPolicyName(cc.Name), Namespace: cc.Namespace}, netPol)).To(Succeed())

			expectedCasNetPolSpec = nwv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentCassandra},
				},
				Ingress: []nwv1.NetworkPolicyIngressRule{
					{
						Ports: []nwv1.NetworkPolicyPort{
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.TlsPort},
								Protocol: &protocolTCP,
							},
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.IntraPort},
								Protocol: &protocolTCP,
							},
						},
						From: []nwv1.NetworkPolicyPeer{
							{
								IPBlock: &nwv1.IPBlock{CIDR: fmt.Sprintf("%s/32", extraCassandraIps[0])},
							},
							{
								IPBlock: &nwv1.IPBlock{CIDR: fmt.Sprintf("%s/32", extraCassandraIps[1])},
							},
						},
					},
				},
				PolicyTypes: []nwv1.PolicyType{"Ingress"},
			}

			Expect(netPol.Spec).To(BeEquivalentTo(expectedCasNetPolSpec))

			By("Checking Prober network policies")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.ProberNetworkPolicyName(cc.Name), Namespace: cc.Namespace}, netPol)).To(Succeed())

			expectedProberNetPol := nwv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentProber},
				},
				Ingress: []nwv1.NetworkPolicyIngressRule{
					{
						Ports: []nwv1.NetworkPolicyPort{
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.ProberContainerPort},
								Protocol: &protocolTCP,
							},
						},
						From: []nwv1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentCassandra},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{v1.LabelMetadataName: cc.Namespace},
								},
							},
						},
					},
					{
						Ports: []nwv1.NetworkPolicyPort{
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.ProberContainerPort},
								Protocol: &protocolTCP,
							},
						},
						From: []nwv1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: dbv1alpha1.CassandraOperatorPodLabels,
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{v1.LabelMetadataName: cc.Namespace},
								},
							},
						},
					},
					{
						Ports: []nwv1.NetworkPolicyPort{
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.ProberContainerPort},
								Protocol: &protocolTCP,
							},
						},
						From: []nwv1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"app.kubernetes.io/name": "prometheus-operator"},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{v1.LabelMetadataName: "prometheus-namespace"},
								},
							},
						},
					},
					{
						Ports: []nwv1.NetworkPolicyPort{
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.ProberContainerPort},
								Protocol: &protocolTCP,
							},
						},
						From: []nwv1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"kubernetes-custom-component": "custom-ingress-controller"},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{v1.LabelMetadataName: "ingress-namespace"},
								},
							},
						},
					},
				},
				PolicyTypes: []nwv1.PolicyType{"Ingress"},
			}

			Expect(netPol.Spec).To(BeEquivalentTo(expectedProberNetPol))

			By("Checking Reaper network policies")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.ReaperNetworkPolicyName(cc.Name), Namespace: cc.Namespace}, netPol)).To(Succeed())

			expectedReaperNetPol := nwv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentReaper},
				},
				Ingress: []nwv1.NetworkPolicyIngressRule{
					{
						Ports: []nwv1.NetworkPolicyPort{
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.ReaperAppPort},
								Protocol: &protocolTCP,
							},
						},
						From: []nwv1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: dbv1alpha1.CassandraOperatorPodLabels,
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{v1.LabelMetadataName: operatorConfig.Namespace},
								},
							},
						},
					},
					{
						Ports: []nwv1.NetworkPolicyPort{
							{
								Port:     &intstr.IntOrString{IntVal: dbv1alpha1.ReaperAppPort},
								Protocol: &protocolTCP,
							},
						},
						From: []nwv1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"app.kubernetes.io/name": "prometheus-operator"},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{v1.LabelMetadataName: "prometheus-namespace"},
								},
							},
						},
					},
				},
				PolicyTypes: []nwv1.PolicyType{"Ingress"},
			}

			Expect(netPol.Spec).To(BeEquivalentTo(expectedReaperNetPol))

		})
	})
})
