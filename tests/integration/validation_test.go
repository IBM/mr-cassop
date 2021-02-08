package integration

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

const (
	emptyChar           = ""
	invalidSpecialChars = "&%*^<>()"
	invalidPullPolicy   = v1.PullPolicy("invalid")
)

var (
	invalidNumCharsShort = strings.Repeat("A", 64)
	invalidNumCharsLong  = strings.Repeat("A", 254)
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
							Name:     emptyChar,
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
							Name:     invalidNumCharsShort,
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
							Name:     invalidSpecialChars,
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
	Context(".spec.cassandra.terminationGracePeriodSeconds", func() {
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
						TerminationGracePeriodSeconds: proto.Int64(-1),
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
						ImagePullPolicy: invalidPullPolicy,
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
						ImagePullPolicy: invalidPullPolicy,
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
							ImagePullPolicy: invalidPullPolicy,
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
							v1alpha1.KeyspaceName(emptyChar),
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
							v1alpha1.KeyspaceName(invalidNumCharsShort),
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
							v1alpha1.KeyspaceName(invalidSpecialChars),
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
								Name: emptyChar,
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
								Name: invalidNumCharsShort,
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
								Name: invalidSpecialChars,
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

	Context(".spec.maintenance[].dc", func() {
		It("should be required if maintenance request is specified", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					Maintenance: []v1alpha1.Maintenance{
						{
							Pods: []v1alpha1.PodName{},
						},
					},
					ImagePullSecretName: "pullSecretName",
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})

	Context(".spec.maintenance[].dc", func() {
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
					Maintenance: []v1alpha1.Maintenance{
						{
							DC: emptyChar,
						},
					},
					ImagePullSecretName: "pullSecretName",
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})

	Context(".spec.maintenance[].dc", func() {
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
					Maintenance: []v1alpha1.Maintenance{
						{
							DC: invalidNumCharsShort,
						},
					},
					ImagePullSecretName: "pullSecretName",
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})

	Context(".spec.maintenance[].dc", func() {
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
					Maintenance: []v1alpha1.Maintenance{
						{
							DC: invalidSpecialChars,
						},
					},
					ImagePullSecretName: "pullSecretName",
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})

	Context(".spec.maintenance[].pods", func() {
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
					Maintenance: []v1alpha1.Maintenance{
						{
							DC: "dc1",
							Pods: []v1alpha1.PodName{
								emptyChar,
							},
						},
					},
					ImagePullSecretName: "pullSecretName",
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})

	Context(".spec.maintenance[].pods", func() {
		It("can't be more than 253 chars", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					Maintenance: []v1alpha1.Maintenance{
						{
							DC: "dc1",
							Pods: []v1alpha1.PodName{
								v1alpha1.PodName(invalidNumCharsLong),
							},
						},
					},
					ImagePullSecretName: "pullSecretName",
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})

	Context(".spec.maintenance[].dc", func() {
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
					Maintenance: []v1alpha1.Maintenance{
						{
							DC: "dc1",
							Pods: []v1alpha1.PodName{
								invalidSpecialChars,
							},
						},
					},
					ImagePullSecretName: "pullSecretName",
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
						NumSeeds:                      3,
						TerminationGracePeriodSeconds: proto.Int64(60),
						ImagePullPolicy:               v1.PullAlways,
						Image:                         "cassandra/image",
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
					Maintenance: []v1alpha1.Maintenance{
						{
							DC: "dc1",
							Pods: []v1alpha1.PodName{
								"test-cluster-cassandra-dc1-0",
							},
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
