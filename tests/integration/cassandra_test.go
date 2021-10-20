package integration

import (
	"fmt"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/names"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("operator configmaps", func() {
	Context("when tests start", func() {
		It("should exist", func() {
			operatorConfigMaps := []string{
				names.OperatorCassandraConfigCM(),
				names.OperatorScriptsCM(),
				names.OperatorShiroCM(),
			}

			for _, cmName := range operatorConfigMaps {
				cm := &v1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: operatorConfig.Namespace}, cm)
				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("ConfigMap %q should exist", cmName))
			}
		})
	})
})

var _ = Describe("cassandra statefulset deployment", func() {
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
					Replicas: proto.Int32(6),
				},
			},
			ImagePullSecretName: "pullSecretName",
			AdminRoleSecretName: "admin-role",
		},
	}

	Context("when cassandracluster created with only required values", func() {
		It("should be created with defaulted values", func() {
			createAdminSecret(cc)
			Expect(k8sClient.Create(ctx, cc)).To(Succeed())
			markMocksAsReady(cc)

			for _, dc := range cc.Spec.DCs {
				sts := &appsv1.StatefulSet{}
				cassandraLabels := map[string]string{
					"cassandra-cluster-component": "cassandra",
					"cassandra-cluster-instance":  "test-cassandra-cluster",
					"cassandra-cluster-dc":        dc.Name,
				}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: names.DC(cc.Name, dc.Name), Namespace: cc.Namespace}, sts)
				}, mediumTimeout, mediumRetry).Should(Succeed())

				Expect(sts.Labels).To(BeEquivalentTo(cassandraLabels))
				Expect(sts.Spec.Replicas).To(BeEquivalentTo(dc.Replicas))
				Expect(sts.Spec.Selector.MatchLabels).To(BeEquivalentTo(cassandraLabels))
				Expect(sts.Spec.ServiceName).To(Equal("test-cassandra-cluster" + "-cassandra-" + dc.Name))
				Expect(sts.Spec.Template.Labels).To(Equal(cassandraLabels))
				Expect(sts.OwnerReferences[0].Controller).To(Equal(proto.Bool(true)))
				Expect(sts.OwnerReferences[0].Kind).To(Equal("CassandraCluster"))
				Expect(sts.OwnerReferences[0].APIVersion).To(Equal("db.ibm.com/v1alpha1"))
				Expect(sts.OwnerReferences[0].Name).To(Equal(cc.Name))
				Expect(sts.OwnerReferences[0].BlockOwnerDeletion).To(Equal(proto.Bool(true)))
				Expect(sts.Spec.VolumeClaimTemplates).To(BeEmpty())
				cassandraContainer, found := getContainerByName(sts.Spec.Template.Spec, "cassandra")
				Expect(found).To(BeTrue())
				Expect(cassandraContainer.Image).To(Equal(operatorConfig.DefaultCassandraImage), "default values")
				Expect(cassandraContainer.ImagePullPolicy).To(Equal(v1.PullIfNotPresent), "default values")
				dataVolumeMount, found := getVolumeMountByName(cassandraContainer.VolumeMounts, "data")
				Expect(found).To(BeTrue())
				Expect(dataVolumeMount.MountPath).To(Equal("/var/lib/cassandra"), "default values")
				dataVolume, found := getVolumeByName(sts.Spec.Template.Spec.Volumes, "data")
				Expect(found).To(BeTrue())
				Expect(dataVolume.EmptyDir).ToNot(BeNil())
				Expect(dataVolume.EmptyDir).To(Equal(&v1.EmptyDirVolumeSource{}), "default values")
				_, found = getVolumeMountByName(cassandraContainer.VolumeMounts, "commitlog")
				Expect(found).To(BeFalse())
			}
		})
	})
})

var _ = Describe("cassandra statefulset", func() {
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
					Replicas: proto.Int32(6),
				},
			},
			ImagePullSecretName: "pullSecretName",
			AdminRoleSecretName: "admin-role",
			Cassandra: &v1alpha1.Cassandra{
				Persistence: v1alpha1.Persistence{
					Enabled:         true,
					CommitLogVolume: true,
					Labels:          map[string]string{"region": "us-south"},
					Annotations:     map[string]string{"storage-type": "Performance"},
					DataVolumeClaimSpec: v1.PersistentVolumeClaimSpec{
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceStorage: resource.MustParse("20Gi"),
							},
						},
					},
					CommitLogVolumeClaimSpec: v1.PersistentVolumeClaimSpec{
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceStorage: resource.MustParse("20Gi"),
							},
						},
					},
				},
			},
		},
	}

	Context("with persistence enabled", func() {
		It("should be created with volume claim template and without data volume", func() {
			createAdminSecret(cc)
			Expect(k8sClient.Create(ctx, cc)).To(Succeed())
			markMocksAsReady(cc)

			for _, dc := range cc.Spec.DCs {
				sts := &appsv1.StatefulSet{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: names.DC(cc.Name, dc.Name), Namespace: cc.Namespace}, sts)
				}, time.Second*5, time.Millisecond*100).Should(Succeed())

				cassandraContainer, found := getContainerByName(sts.Spec.Template.Spec, "cassandra")
				Expect(found).To(BeTrue())
				dataVolumeMount, found := getVolumeMountByName(cassandraContainer.VolumeMounts, "data")
				Expect(found).To(BeTrue())
				Expect(dataVolumeMount.MountPath).To(Equal("/var/lib/cassandra"))
				commitLogVolumeMount, found := getVolumeMountByName(cassandraContainer.VolumeMounts, "commitlog")
				Expect(found).To(BeTrue())
				Expect(commitLogVolumeMount.MountPath).To(Equal("/var/lib/cassandra-commitlog"))
				_, found = getVolumeByName(sts.Spec.Template.Spec.Volumes, "data")
				Expect(found).ToNot(BeTrue())
				Expect(sts.Spec.VolumeClaimTemplates).ToNot(BeEmpty())
				Expect(sts.Spec.VolumeClaimTemplates).To(HaveLen(2))
				Expect(sts.Spec.VolumeClaimTemplates[0].Name).To(Equal("data"))
				Expect(sts.Spec.VolumeClaimTemplates[0].Labels).To(Equal(map[string]string{
					v1alpha1.CassandraClusterInstance:  cc.Name,
					v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
					"region":                           "us-south",
				}))
				Expect(sts.Spec.VolumeClaimTemplates[0].Annotations).To(Equal(map[string]string{"storage-type": "Performance"}))
				Expect(sts.Spec.VolumeClaimTemplates[0].Spec.AccessModes).To(Equal([]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce}))
				Expect(sts.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[v1.ResourceStorage]).To(Equal(resource.MustParse("20Gi")))
				Expect(sts.Spec.VolumeClaimTemplates[1].Name).To(Equal("commitlog"))
				Expect(sts.Spec.VolumeClaimTemplates[1].Labels).To(Equal(map[string]string{
					v1alpha1.CassandraClusterInstance:  cc.Name,
					v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
					"region":                           "us-south",
				}))
				Expect(sts.Spec.VolumeClaimTemplates[1].Annotations).To(Equal(map[string]string{"storage-type": "Performance"}))
				Expect(sts.Spec.VolumeClaimTemplates[1].Spec.AccessModes).To(Equal([]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce}))
				Expect(sts.Spec.VolumeClaimTemplates[1].Spec.Resources.Requests[v1.ResourceStorage]).To(Equal(resource.MustParse("20Gi")))
			}
		})
	})
})

var _ = Describe("cassandra statefulset", func() {
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
					Replicas: proto.Int32(6),
				},
			},
			ImagePullSecretName: "pullSecretName",
			AdminRoleSecretName: "admin-role",
			Cassandra: &v1alpha1.Cassandra{
				Persistence: v1alpha1.Persistence{
					Enabled:         true,
					CommitLogVolume: false,
					Labels:          map[string]string{"region": "us-south"},
					Annotations:     map[string]string{"storage-type": "Performance"},
					DataVolumeClaimSpec: v1.PersistentVolumeClaimSpec{
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceStorage: resource.MustParse("20Gi"),
							},
						},
					},
					CommitLogVolumeClaimSpec: v1.PersistentVolumeClaimSpec{
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceStorage: resource.MustParse("20Gi"),
							},
						},
					},
				},
			},
		},
	}

	Context("with persistence enabled but commitlog disabled", func() {
		It("should be created with volume claim template with only data volume", func() {
			createAdminSecret(cc)
			Expect(k8sClient.Create(ctx, cc)).To(Succeed())

			mockProberClient.err = nil
			mockProberClient.ready = true
			mockNodetoolClient.err = nil
			mockReaperClient.err = nil
			mockReaperClient.isRunning = true
			mockReaperClient.clusterExists = true
			mockCQLClient.err = nil
			mockCQLClient.cassandraRoles = []cql.Role{{Role: "cassandra", Super: true}}
			mockCQLClient.keyspaces = []cql.Keyspace{{
				Name: "system_auth",
				Replication: map[string]string{
					"class": "org.apache.cassandra.locator.SimpleTopologyStrategy",
				},
			}}

			for _, dc := range cc.Spec.DCs {
				sts := &appsv1.StatefulSet{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: names.DC(cc.Name, dc.Name), Namespace: cc.Namespace}, sts)
				}, time.Second*10, time.Millisecond*100).Should(Succeed())

				cassandraContainer, found := getContainerByName(sts.Spec.Template.Spec, "cassandra")
				Expect(found).To(BeTrue())
				dataVolumeMount, found := getVolumeMountByName(cassandraContainer.VolumeMounts, "data")
				Expect(found).To(BeTrue())
				Expect(dataVolumeMount.MountPath).To(Equal("/var/lib/cassandra"), "default values")
				_, found = getVolumeMountByName(cassandraContainer.VolumeMounts, "commitlog")
				Expect(found).To(BeFalse())
				_, found = getVolumeByName(sts.Spec.Template.Spec.Volumes, "data")
				Expect(found).To(BeFalse())
				Expect(sts.Spec.VolumeClaimTemplates).To(HaveLen(1))
				Expect(sts.Spec.VolumeClaimTemplates[0].Name).To(Equal("data"))
				Expect(sts.Spec.VolumeClaimTemplates[0].Labels).To(Equal(map[string]string{
					v1alpha1.CassandraClusterInstance:  cc.Name,
					v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra,
					"region":                           "us-south",
				}))
				Expect(sts.Spec.VolumeClaimTemplates[0].Annotations).To(Equal(map[string]string{"storage-type": "Performance"}))
				Expect(sts.Spec.VolumeClaimTemplates[0].Spec.AccessModes).To(Equal([]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce}))
				Expect(sts.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[v1.ResourceStorage]).To(Equal(resource.MustParse("20Gi")))
			}
		})
	})
})
