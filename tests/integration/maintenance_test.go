package integration

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/names"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

var _ = Describe("maintenance mode request", func() {
	Context("no requests", func() {
		It("should be created", func() {
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
				},
			}
			Expect(k8sClient.Create(ctx, cc)).To(Succeed())
			Expect(initializeReadyCluster(cc)).To(Succeed())
			actualCC := getCassandraCluster(cc)
			Expect(actualCC.Spec.Maintenance).To(BeNil())
			Expect(actualCC.Status.MaintenanceState).To(BeNil())
		})
	})

	Context("single pod", func() {
		It("should update status", func() {
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
				},
			}
			Expect(k8sClient.Create(ctx, cc)).To(Succeed())
			Expect(initializeReadyCluster(cc)).To(Succeed())

			When("pod is added to spec", func() {
				actualCC := getCassandraCluster(cc)
				maintenancePatch := client.MergeFrom(actualCC.DeepCopy())
				actualCC.Spec.Maintenance = []v1alpha1.Maintenance{
					{
						DC: "dc1",
						Pods: []v1alpha1.PodName{
							v1alpha1.PodName(names.DC(actualCC.Name, actualCC.Spec.DCs[0].Name) + "-0"),
						},
					},
				}
				Expect(k8sClient.Patch(ctx, actualCC, maintenancePatch)).To(Succeed())
				compareMaintenanceSpecWithStatus(cc)
			})

			When("pod is removed from spec", func() {
				actualCC := getCassandraCluster(cc)
				maintenancePatch := client.MergeFrom(actualCC.DeepCopy())
				actualCC.Spec.Maintenance = nil
				Expect(k8sClient.Patch(ctx, actualCC, maintenancePatch)).To(Succeed())
				compareMaintenanceSpecWithStatus(cc)
			})
		})
	})

	Context("multiple pods in single dc", func() {
		It("should update status", func() {
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
				},
			}
			Expect(k8sClient.Create(ctx, cc)).To(Succeed())
			Expect(initializeReadyCluster(cc)).To(Succeed())

			When("pod is added to spec", func() {
				actualCC := getCassandraCluster(cc)
				maintenancePatch := client.MergeFrom(actualCC.DeepCopy())
				actualCC.Spec.Maintenance = []v1alpha1.Maintenance{
					{
						DC: "dc1",
						Pods: []v1alpha1.PodName{
							v1alpha1.PodName(names.DC(actualCC.Name, actualCC.Spec.DCs[0].Name) + "-0"),
							v1alpha1.PodName(names.DC(actualCC.Name, actualCC.Spec.DCs[0].Name) + "-1"),
						},
					},
				}
				Expect(k8sClient.Patch(ctx, actualCC, maintenancePatch)).To(Succeed())
				compareMaintenanceSpecWithStatus(cc)
			})

			When("pod is removed from spec", func() {
				actualCC := getCassandraCluster(cc)
				maintenancePatch := client.MergeFrom(actualCC.DeepCopy())
				actualCC.Spec.Maintenance = nil
				Expect(k8sClient.Patch(ctx, actualCC, maintenancePatch)).To(Succeed())
				compareMaintenanceSpecWithStatus(cc)
			})
		})
	})

	Context("multiple pods in multiple dcs", func() {
		It("should update status", func() {
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
				},
			}
			Expect(k8sClient.Create(ctx, cc)).To(Succeed())
			Expect(initializeReadyCluster(cc)).To(Succeed())

			When("pods are added to spec", func() {
				actualCC := getCassandraCluster(cc)
				maintenancePatch := client.MergeFrom(actualCC.DeepCopy())
				actualCC.Spec.Maintenance = []v1alpha1.Maintenance{
					{
						DC: "dc1",
						Pods: []v1alpha1.PodName{
							v1alpha1.PodName(names.DC(actualCC.Name, actualCC.Spec.DCs[0].Name) + "-0"),
							v1alpha1.PodName(names.DC(actualCC.Name, actualCC.Spec.DCs[0].Name) + "-1"),
						},
					},
					{
						DC: "dc2",
						Pods: []v1alpha1.PodName{
							v1alpha1.PodName(names.DC(actualCC.Name, actualCC.Spec.DCs[1].Name) + "-0"),
						},
					},
				}
				Expect(k8sClient.Patch(ctx, actualCC, maintenancePatch)).To(Succeed())
				compareMaintenanceSpecWithStatus(cc)
			})

			When("pods are removed from spec", func() {
				actualCC := getCassandraCluster(cc)
				maintenancePatch := client.MergeFrom(actualCC.DeepCopy())
				actualCC.Spec.Maintenance = []v1alpha1.Maintenance{
					{
						DC: "dc1",
						Pods: []v1alpha1.PodName{
							v1alpha1.PodName(names.DC(actualCC.Name, actualCC.Spec.DCs[0].Name) + "-0"),
							v1alpha1.PodName(names.DC(actualCC.Name, actualCC.Spec.DCs[0].Name) + "-1"),
						},
					},
				}
				Expect(k8sClient.Patch(ctx, actualCC, maintenancePatch)).To(Succeed())
				compareMaintenanceSpecWithStatus(cc)
			})
		})
	})

	Context("entire dc", func() {
		It("should update status", func() {
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
				},
			}
			Expect(k8sClient.Create(ctx, cc)).To(Succeed())
			Expect(initializeReadyCluster(cc)).To(Succeed())

			When("dc is added to spec", func() {
				actualCC := getCassandraCluster(cc)
				maintenancePatch := client.MergeFrom(actualCC.DeepCopy())
				actualCC.Spec.Maintenance = []v1alpha1.Maintenance{
					{
						DC: "dc2",
					},
				}
				Expect(k8sClient.Patch(ctx, actualCC, maintenancePatch)).To(Succeed())
				compareMaintenanceSpecWithStatus(cc)
			})

			When("dc is removed from spec", func() {
				actualCC := getCassandraCluster(cc)
				maintenancePatch := client.MergeFrom(actualCC.DeepCopy())
				actualCC.Spec.Maintenance = nil
				Expect(k8sClient.Patch(ctx, actualCC, maintenancePatch)).To(Succeed())
				compareMaintenanceSpecWithStatus(cc)
			})
		})
	})

	Context("change pods in single dc", func() {
		It("should update status", func() {
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
				},
			}
			Expect(k8sClient.Create(ctx, cc)).To(Succeed())
			Expect(initializeReadyCluster(cc)).To(Succeed())

			When("pod is added to spec", func() {
				actualCC := getCassandraCluster(cc)
				maintenancePatch := client.MergeFrom(actualCC.DeepCopy())
				actualCC.Spec.Maintenance = []v1alpha1.Maintenance{
					{
						DC: "dc1",
						Pods: []v1alpha1.PodName{
							v1alpha1.PodName(names.DC(actualCC.Name, actualCC.Spec.DCs[0].Name) + "-0"),
						},
					},
				}
				Expect(k8sClient.Patch(ctx, actualCC, maintenancePatch)).To(Succeed())
				compareMaintenanceSpecWithStatus(cc)
			})

			When("pod is changed in spec", func() {
				actualCC := getCassandraCluster(cc)
				maintenancePatch := client.MergeFrom(actualCC.DeepCopy())
				actualCC.Spec.Maintenance = []v1alpha1.Maintenance{
					{
						DC: "dc1",
						Pods: []v1alpha1.PodName{
							v1alpha1.PodName(names.DC(actualCC.Name, actualCC.Spec.DCs[0].Name) + "-1"),
						},
					},
				}
				Expect(k8sClient.Patch(ctx, actualCC, maintenancePatch)).To(Succeed())
				compareMaintenanceSpecWithStatus(cc)
			})
		})
	})
})

func compareMaintenanceSpecWithStatus(desiredCC *v1alpha1.CassandraCluster) {
	Eventually(func() []v1alpha1.Maintenance {
		actualCC := getCassandraCluster(desiredCC)
		return actualCC.Status.MaintenanceState
	}, time.Second*5, time.Millisecond*100).Should(Equal(desiredCC.Spec.Maintenance))
}

func getCassandraCluster(desiredCC *v1alpha1.CassandraCluster) *v1alpha1.CassandraCluster {
	actualCC := &v1alpha1.CassandraCluster{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: desiredCC.Name, Namespace: desiredCC.Namespace}, actualCC)
	Expect(err).ToNot(HaveOccurred())
	return actualCC
}
