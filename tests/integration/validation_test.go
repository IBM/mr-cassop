package integration

import (
	"github.com/gogo/protobuf/proto"
	"strings"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	var validCluster = &v1alpha1.CassandraCluster{
		ObjectMeta: cassandraObjectMeta,
		Spec: v1alpha1.CassandraClusterSpec{
			DCs: []v1alpha1.DC{
				{
					Name:     "dc1",
					Replicas: proto.Int32(3),
				},
			},
			ImagePullSecretName: "pullSecretName",
			AdminRoleSecretName: "admin-role",
		},
	}

	expectToBeInvalidError := func(err error) {
		Expect(err).NotTo(BeNil())
		Expect(err).To(BeAssignableToTypeOf(&errors.StatusError{}))
		Expect(err.(*errors.StatusError).ErrStatus.Reason == metav1.StatusReasonInvalid).To(BeTrue())
	}
	Context("with all valid parameters", func() {
		It("should pass validation", func() {
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
						NumSeeds:                      3,
						TerminationGracePeriodSeconds: proto.Int64(60),
						ImagePullPolicy:               v1.PullAlways,
						Image:                         "cassandra/image",
					},
					Prober: v1alpha1.Prober{
						ImagePullPolicy: v1.PullAlways,
						Image:           "prober/image",
						Jolokia: v1alpha1.Jolokia{
							Image:           "jolokia/image",
							ImagePullPolicy: v1.PullAlways,
						},
						Tolerations:  nil,
						NodeSelector: nil,
						Affinity:     nil,
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
						Image:                                  "reaper/image",
						ImagePullPolicy:                        v1.PullAlways,
						Keyspace:                               "reaper",
						Tolerations:                            nil,
						NodeSelector:                           nil,
						IncrementalRepair:                      false,
						RepairIntensity:                        "1.0",
						RepairManagerSchedulingIntervalSeconds: 0,
						BlacklistTWCS:                          false,
					},
					CQLConfigMapLabelKey: "cql-label-key",
					SystemKeyspaces: v1alpha1.SystemKeyspaces{
						Keyspaces: []v1alpha1.KeyspaceName{
							"system_auth",
						},
						DCs: []v1alpha1.SystemKeyspaceDC{
							{
								Name: "dc1",
								RF:   1,
							},
							{
								Name: "dc2",
								RF:   1,
							},
						},
					},
				},
			}
			markMocksAsReady(cc)
			err := k8sClient.Create(ctx, cc)
			Expect(err).ToNot(HaveOccurred())
		})
	})
	Context(".spec.dcs", func() {
		It("should be required", func() {
			cc := &v1alpha1.CassandraCluster{ObjectMeta: cassandraObjectMeta}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.dcs", func() {
		It("should have at least one element", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.DCs = []v1alpha1.DC{}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.imagePullSecretName", func() {
		It("should be required", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.ImagePullSecretName = ""
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context("with required fields specified", func() {
		It("should pass", func() {
			cc := validCluster.DeepCopy()
			createAdminSecret(cc)
			markMocksAsReady(cc)
			Expect(k8sClient.Create(ctx, cc)).To(Succeed())

		})
	})
	Context("with empty admin role secret name", func() {
		It("should not pass validation", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.AdminRoleSecretName = ""
			markMocksAsReady(cc)
			Expect(k8sClient.Create(ctx, cc)).ToNot(Succeed())

		})
	})
	Context(".spec.dc[].replicas", func() {
		It("can't be negative number", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.DCs = []v1alpha1.DC{
				{
					Name:     "dc1",
					Replicas: proto.Int32(-3),
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.dc[].name", func() {
		It("can't be empty", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.DCs = []v1alpha1.DC{
				{
					Name:     emptyChar,
					Replicas: proto.Int32(3),
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.dc[].name", func() {
		It("can't be more than 63 chars", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.DCs = []v1alpha1.DC{
				{
					Name:     invalidNumCharsShort,
					Replicas: proto.Int32(3),
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.dc[].name", func() {
		It("can't be contain invalid chars", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.DCs = []v1alpha1.DC{
				{
					Name:     invalidSpecialChars,
					Replicas: proto.Int32(3),
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.cassandra.numSeeds", func() {
		It("can't be negative number", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.Cassandra = &v1alpha1.Cassandra{
				NumSeeds: -3,
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.cassandra.terminationGracePeriodSeconds", func() {
		It("can't be negative number", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.Cassandra = &v1alpha1.Cassandra{
				TerminationGracePeriodSeconds: proto.Int64(-1),
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.cassandra.imagePullPolicy", func() {
		It("can only be one of (Always;Never;IfNotPresent)", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.Cassandra = &v1alpha1.Cassandra{
				NumSeeds:        3,
				ImagePullPolicy: invalidPullPolicy,
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.prober.imagePullPolicy", func() {
		It("can only be one of (Always;Never;IfNotPresent)", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.Cassandra = &v1alpha1.Cassandra{
				NumSeeds:        3,
				ImagePullPolicy: v1.PullAlways,
			}
			cc.Spec.Prober = v1alpha1.Prober{
				ImagePullPolicy: invalidPullPolicy,
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.prober.jolokia.imagePullPolicy", func() {
		It("can only be one of (Always;Never;IfNotPresent)", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.Cassandra = &v1alpha1.Cassandra{
				NumSeeds:        3,
				ImagePullPolicy: v1.PullAlways,
			}
			cc.Spec.Prober = v1alpha1.Prober{
				ImagePullPolicy: v1.PullIfNotPresent,
				Jolokia: v1alpha1.Jolokia{
					ImagePullPolicy: invalidPullPolicy,
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.systemKeyspaces.names", func() {
		It("element can't be empty", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.SystemKeyspaces = v1alpha1.SystemKeyspaces{
				Keyspaces: []v1alpha1.KeyspaceName{
					emptyChar,
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.systemKeyspaces.names", func() {
		It("element can't be more than 48 chars (cassandra limitation)", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.SystemKeyspaces.Keyspaces = []v1alpha1.KeyspaceName{
				v1alpha1.KeyspaceName(invalidNumCharsShort),
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.systemKeyspaces.names", func() {
		It("element can't have invalid chars", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.SystemKeyspaces.Keyspaces = []v1alpha1.KeyspaceName{
				invalidSpecialChars,
			}

			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.systemKeyspaces.dc[].name", func() {
		It("can't be empty", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.SystemKeyspaces = v1alpha1.SystemKeyspaces{
				Keyspaces: []v1alpha1.KeyspaceName{
					"system_auth",
				},
				DCs: []v1alpha1.SystemKeyspaceDC{
					{
						Name: emptyChar,
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.systemKeyspaces.dc[].name", func() {
		It("can't be more than 63 chars", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.SystemKeyspaces = v1alpha1.SystemKeyspaces{
				Keyspaces: []v1alpha1.KeyspaceName{
					"system_auth",
				},
				DCs: []v1alpha1.SystemKeyspaceDC{
					{
						Name: invalidNumCharsShort,
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.systemKeyspaces.dc[].name", func() {
		It("can't have invalid chars", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.SystemKeyspaces = v1alpha1.SystemKeyspaces{
				Keyspaces: []v1alpha1.KeyspaceName{
					"system_auth",
				},
				DCs: []v1alpha1.SystemKeyspaceDC{
					{
						Name: invalidSpecialChars,
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.systemKeyspaces.dc[].rf", func() {
		It("can't be zero", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.SystemKeyspaces = v1alpha1.SystemKeyspaces{
				Keyspaces: []v1alpha1.KeyspaceName{
					"system_auth",
				},
				DCs: []v1alpha1.SystemKeyspaceDC{
					{
						Name: "dc1",
						RF:   0,
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context("with invalid reaper parameters", func() {
		It("should fail the validation", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.Reaper = &v1alpha1.Reaper{
				IncrementalRepair: true,
				RepairParallelism: "DATACENTER_AWARE",
			}
			markMocksAsReady(cc)
			err := k8sClient.Create(ctx, cc)
			Expect(err).To(BeAssignableToTypeOf(&errors.StatusError{}))
			Expect(err.(*errors.StatusError).ErrStatus.Reason == "repairParallelism must be only `PARALLEL` if incrementalRepair is true").To(BeTrue())
		})
	})
	Context("with invalid repair intensity values", func() {
		It("should fail the validation", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.Reaper = &v1alpha1.Reaper{
				RepairIntensity: "0.0",
				RepairSchedules: v1alpha1.RepairSchedules{
					Enabled: true,
					Repairs: []v1alpha1.RepairSchedule{
						{
							Keyspace:            "usage",
							Tables:              []string{"usagebymonth"},
							SegmentCountPerNode: 0,
							RepairParallelism:   "SEQUENTIAL",
							Intensity:           "1.1",
							IncrementalRepair:   false,
							ScheduleDaysBetween: 7,
							ScheduleTriggerTime: "2020-03-27T04:00:00",
							RepairThreadCount:   4,
						},
					},
				},
			}
			markMocksAsReady(cc)
			err := k8sClient.Create(ctx, cc)
			Expect(err).To(BeAssignableToTypeOf(&errors.StatusError{}))
			Expect(err.(*errors.StatusError).ErrStatus.Reason).To(BeEquivalentTo("[reaper repairIntensity value 0.0 must be between 0.1 and 1.0, reaper repairIntensity value 1.1 must be between 0.1 and 1.0]"))
		})
	})
	Context("with invalid repair schedule time", func() {
		It("should fail the validation", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.Reaper = &v1alpha1.Reaper{
				RepairSchedules: v1alpha1.RepairSchedules{
					Enabled: true,
					Repairs: []v1alpha1.RepairSchedule{
						{
							Keyspace:            "usage",
							Tables:              []string{"usagebymonth"},
							SegmentCountPerNode: 0,
							RepairParallelism:   "SEQUENTIAL",
							Intensity:           "1.0",
							IncrementalRepair:   false,
							ScheduleDaysBetween: 7,
							ScheduleTriggerTime: "20200327T04:00:00",
							RepairThreadCount:   4,
						},
						{
							Keyspace:            "usage",
							Tables:              []string{"usagebymonth"},
							SegmentCountPerNode: 0,
							RepairParallelism:   "SEQUENTIAL",
							Intensity:           "1.0",
							IncrementalRepair:   false,
							ScheduleDaysBetween: 7,
							ScheduleTriggerTime: "2020-03-27T04:00",
							RepairThreadCount:   4,
						},
					},
				},
			}
			markMocksAsReady(cc)
			err := k8sClient.Create(ctx, cc)
			Expect(err).To(BeAssignableToTypeOf(&errors.StatusError{}))
			Expect(err.(*errors.StatusError).ErrStatus.Reason).To(BeEquivalentTo("[reaper repair schedule `20200327T04:00:00` has invalid format, should be `2000-01-31T00:00:00`, reaper repair schedule `2020-03-27T04:00` has invalid format, should be `2000-01-31T00:00:00`]"))
		})
	})
	Context(".spec.maintenance[].dc", func() {
		It("should be required if maintenance request is specified", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.Maintenance = []v1alpha1.Maintenance{
				{
					Pods: []v1alpha1.PodName{},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.maintenance[].dc", func() {
		It("can't be empty", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.Maintenance = []v1alpha1.Maintenance{
				{
					DC: emptyChar,
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.maintenance[].dc", func() {
		It("can't be more than 63 chars", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.Maintenance = []v1alpha1.Maintenance{
				{
					DC: invalidNumCharsShort,
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.maintenance[].dc", func() {
		It("can't have invalid chars", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.Maintenance = []v1alpha1.Maintenance{
				{
					DC: invalidSpecialChars,
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.maintenance[].pods", func() {
		It("can't be empty", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.Maintenance = []v1alpha1.Maintenance{
				{
					DC: "dc1",
					Pods: []v1alpha1.PodName{
						emptyChar,
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.maintenance[].pods", func() {
		It("can't be more than 253 chars", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.Maintenance = []v1alpha1.Maintenance{
				{
					DC: "dc1",
					Pods: []v1alpha1.PodName{
						v1alpha1.PodName(invalidNumCharsLong),
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.maintenance[].dc", func() {
		It("can't have invalid chars", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.Maintenance = []v1alpha1.Maintenance{
				{
					DC: "dc1",
					Pods: []v1alpha1.PodName{
						invalidSpecialChars,
					},
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context(".spec.monitoring.agent", func() {
		It("can only be one of (instaclustr;datastax;tlp)", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.Cassandra = &v1alpha1.Cassandra{
				NumSeeds:        3,
				ImagePullPolicy: v1.PullAlways,
				Monitoring: v1alpha1.Monitoring{
					Enabled: true,
					Agent:   "agent",
				},
			}
			err := k8sClient.Create(ctx, cc)
			expectToBeInvalidError(err)
		})
	})
	Context("with invalid cassandra config overrides", func() {
		It("should fail the validation", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.Cassandra = &v1alpha1.Cassandra{
				ConfigOverrides: `invalid yaml&*^&&(*) config{}:::- sd:
:
:sd;`,
			}
			markMocksAsReady(cc)
			err := k8sClient.Create(ctx, cc)
			Expect(err).To(BeAssignableToTypeOf(&errors.StatusError{}))
			Expect(err.(*errors.StatusError).ErrStatus.Reason).To(BeEquivalentTo("cassandra config override should be a string with valid YAML: error converting YAML to JSON: yaml: line 1: did not find expected key"))
		})
	})
	Context("with invalid seeds config", func() {
		It("should fail the validation", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.Cassandra = &v1alpha1.Cassandra{
				NumSeeds: 4,
			}
			markMocksAsReady(cc)
			err := k8sClient.Create(ctx, cc)
			Expect(err).To(BeAssignableToTypeOf(&errors.StatusError{}))
			Expect(err.(*errors.StatusError).ErrStatus.Reason).To(BeEquivalentTo("number of seeds (4) is greater than number of replicas (3) for dc dc1"))
		})
	})
	Context("with invalid rf config", func() {
		It("should fail the validation", func() {
			cc := validCluster.DeepCopy()
			cc.Spec.SystemKeyspaces.DCs = []v1alpha1.SystemKeyspaceDC{
				{
					Name: "dc1",
					RF:   4,
				},
			}
			markMocksAsReady(cc)
			err := k8sClient.Create(ctx, cc)
			Expect(err).To(BeAssignableToTypeOf(&errors.StatusError{}))
			Expect(err.(*errors.StatusError).ErrStatus.Reason).To(BeEquivalentTo("replication factor (4) is greater than number of replicas (3) for dc dc1"))
		})
	})
})
