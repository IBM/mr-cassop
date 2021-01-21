package integration

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("cassandracluster validation", func() {
	expectToBeInvalidError := func(err error) {
		Expect(err).NotTo(BeNil())
		Expect(err).To(BeAssignableToTypeOf(&errors.StatusError{}))
		Expect(err.(*errors.StatusError).ErrStatus.Reason == metav1.StatusReasonInvalid).To(BeTrue())
	}
	Context(".spec.dcs", func() {
		It("should be required", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.dcs", func() {
		It("should have at least one element", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.imagePullSecretName", func() {
		It("should be required", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context("with required fields specified", func() {
		It("should pass", func() {
			initializeReadyCluster()
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pullSecretName",
				},
			}
			Expect(k8sClient.Create(ctx, cc)).To(Succeed())

		})
	})
	Context(".spec.dc[].replicas", func() {
		It("can't be negative number", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(-3),
						},
					},
					ImagePullSecretName: "pullSecretName",
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.dc[].name", func() {
		It("can't be empty", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pullSecretName",
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.dc[].name", func() {
		It("can't be more than 63 chars", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "morethan63charsmorethan63charsmorethan63charsmorethan63charsmorethan63",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pullSecretName",
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.dc[].name", func() {
		It("can't be contain invalid chars", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "&%(*^()",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pullSecretName",
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.cassandra.numSeeds", func() {
		It("can't be negative number", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pullSecretName",
					Cassandra: &v1alpha1.Cassandra{
						NumSeeds: -3,
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.cassandra.imagePullPolicy", func() {
		It("can only be one of (Always;Never;IfNotPresent)", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pullSecretName",
					Cassandra: &v1alpha1.Cassandra{
						NumSeeds:        3,
						ImagePullPolicy: "invalid",
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.prober.imagePullPolicy", func() {
		It("can only be one of (Always;Never;IfNotPresent)", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pullSecretName",
					Cassandra: &v1alpha1.Cassandra{
						NumSeeds:        3,
						ImagePullPolicy: v1.PullAlways,
					},
					Prober: v1alpha1.Prober{
						ImagePullPolicy: "invalid",
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.prober.jolokia.imagePullPolicy", func() {
		It("can only be one of (Always;Never;IfNotPresent)", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pullSecretName",
					Cassandra: &v1alpha1.Cassandra{
						NumSeeds:        3,
						ImagePullPolicy: v1.PullAlways,
					},
					Prober: v1alpha1.Prober{
						ImagePullPolicy: v1.PullIfNotPresent,
						Jolokia: v1alpha1.Jolokia{
							ImagePullPolicy: "invalid",
						},
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.systemKeyspaces.names", func() {
		It("element can't be empty", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pullSecretName",
					SystemKeyspaces: v1alpha1.SystemKeyspaces{
						Names: []v1alpha1.KeyspaceName{
							"",
						},
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.systemKeyspaces.names", func() {
		It("element can't be more than 48 chars (cassandra limitation)", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pullSecretName",
					SystemKeyspaces: v1alpha1.SystemKeyspaces{
						Names: []v1alpha1.KeyspaceName{
							"morethan48charsmorethan48charsmorethan48charsmorethan48chars",
						},
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.systemKeyspaces.names", func() {
		It("element can't have invalid chars", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pullSecretName",
					SystemKeyspaces: v1alpha1.SystemKeyspaces{
						Names: []v1alpha1.KeyspaceName{
							"%(&*",
						},
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.systemKeyspaces.dc[].name", func() {
		It("can't be empty", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pullSecretName",
					SystemKeyspaces: v1alpha1.SystemKeyspaces{
						Names: []v1alpha1.KeyspaceName{
							"system_auth",
						},
						DCs: []v1alpha1.SystemKeyspaceDC{
							{
								Name: "",
							},
						},
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.systemKeyspaces.dc[].name", func() {
		It("can't be more than 63 chars", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pullSecretName",
					SystemKeyspaces: v1alpha1.SystemKeyspaces{
						Names: []v1alpha1.KeyspaceName{
							"system_auth",
						},
						DCs: []v1alpha1.SystemKeyspaceDC{
							{
								Name: "morethan63charsmorethan63charsmorethan63charsmorethan63charsmore",
							},
						},
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.systemKeyspaces.dc[].name", func() {
		It("can't have invalid chars", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pullSecretName",
					SystemKeyspaces: v1alpha1.SystemKeyspaces{
						Names: []v1alpha1.KeyspaceName{
							"system_auth",
						},
						DCs: []v1alpha1.SystemKeyspaceDC{
							{
								Name: "^&%$^&*",
							},
						},
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.systemKeyspaces.dc[].rf", func() {
		It("can't be zero", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pullSecretName",
					SystemKeyspaces: v1alpha1.SystemKeyspaces{
						Names: []v1alpha1.KeyspaceName{
							"system_auth",
						},
						DCs: []v1alpha1.SystemKeyspaceDC{
							{
								Name: "dc1",
								RF:   0,
							},
						},
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})

	Context(".spec.reaper.datacenterAvailability", func() {
		It("can't be invalid", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pullSecretName",
					Reaper: &v1alpha1.Reaper{
						Keyspace:               "system_auth",
						DatacenterAvailability: "invalid",
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})

	Context("with all valid parameters", func() {
		It("should pass validation", func() {
			initializeReadyCluster()
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
					Cassandra: &v1alpha1.Cassandra{
						NumSeeds:        3,
						ImagePullPolicy: v1.PullAlways,
						Image:           "cassandra/image",
					},
					Prober: v1alpha1.Prober{
						ImagePullPolicy: v1.PullAlways,
						Image:           "prober/image",
						Debug:           false,
						Jolokia: v1alpha1.Jolokia{
							Image:           "jolokia/image",
							ImagePullPolicy: v1.PullAlways,
						},
					},
					Reaper: &v1alpha1.Reaper{
						Image:           "reaper/image",
						ImagePullPolicy: v1.PullAlways,
						Keyspace:        "reaper_db",
						DCs: []v1alpha1.DC{
							{
								Name:     "dc1",
								Replicas: proto.Int32(1),
							},
							{
								Name:     "dc2",
								Replicas: proto.Int32(2),
							},
						},
						DatacenterAvailability:                 "each",
						Tolerations:                            nil,
						NodeSelector:                           nil,
						IncrementalRepair:                      false,
						RepairIntensity:                        "1.0",
						RepairManagerSchedulingIntervalSeconds: 0,
						BlacklistTWCS:                          false,
					},
					CQLConfigMapLabelKey: "cql-label-key",
					SystemKeyspaces: v1alpha1.SystemKeyspaces{
						Names: []v1alpha1.KeyspaceName{
							"system_auth",
						},
						DCs: []v1alpha1.SystemKeyspaceDC{
							{
								Name: "dc1",
								RF:   1,
							},
						},
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
