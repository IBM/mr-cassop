package integration

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/reaper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("reaper deployment", func() {
	type test struct {
		name string
		cc   *v1alpha1.CassandraCluster
	}

	tests := []test{
		{
			name: "should deploy with defaulted values",
			cc: &v1alpha1.CassandraCluster{
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
			},
		},
		{
			name: "should schedule all repair jobs",
			cc: &v1alpha1.CassandraCluster{
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
					AdminRoleSecretName: "admin-role",
					Reaper: &v1alpha1.Reaper{
						RepairSchedules: v1alpha1.RepairSchedules{
							Enabled: true,
							Repairs: []v1alpha1.RepairSchedule{
								{
									Keyspace:            "system_traces",
									Tables:              []string{"events"},
									ScheduleDaysBetween: 7,
									ScheduleTriggerTime: "2020-11-15T14:00:00",
									Datacenters:         []string{"dc1, dc2"},
									IncrementalRepair:   false,
									RepairThreadCount:   2,
									Intensity:           "1.0",
									RepairParallelism:   "DATACENTER_AWARE",
								},
								{
									Keyspace:            "system_auth",
									ScheduleDaysBetween: 7,
									ScheduleTriggerTime: "2021-01-06T04:00:00",
									IncrementalRepair:   false,
									RepairThreadCount:   4,
									RepairParallelism:   "PARALLEL",
								},
							},
						},
					},
					ImagePullSecretName: "pullSecretName",
				},
			},
		},
	}

	Context("when cassandracluster is created", func() {
		for _, tc := range tests {
			It(tc.name, func() {
				cc := tc.cc.DeepCopy()
				createAdminSecret(cc)
				Expect(k8sClient.Create(ctx, cc)).To(Succeed())
				markMocksAsReady(cc)
				waitForDCsToBeCreated(cc)
				markAllDCsReady(cc)
				createCassandraPods(cc)
				for index, dc := range cc.Spec.DCs {
					reaperLabels := map[string]string{
						"cassandra-cluster-component": "reaper",
						"cassandra-cluster-instance":  "test-cassandra-cluster",
						"cassandra-cluster-dc":        dc.Name,
					}

					// Check if first reaper deployment has been created
					if index == 0 {
						// Wait for the operator to create the first reaper deployment
						validateNumberOfDeployments(cc.Namespace, reaperDeploymentLabels, 1)
					}

					reaperDeployment := markDeploymentAsReady(types.NamespacedName{Name: names.ReaperDeployment(cc.Name, dc.Name), Namespace: cc.Namespace})

					Expect(reaperDeployment.Labels).To(BeEquivalentTo(reaperLabels))
					Expect(reaperDeployment.Spec.Replicas).To(Equal(proto.Int32(v1alpha1.ReaperReplicasNumber)))
					Expect(reaperDeployment.Spec.Selector.MatchLabels).To(BeEquivalentTo(reaperLabels))
					Expect(reaperDeployment.Spec.Template.Labels).To(Equal(reaperLabels))
					Expect(reaperDeployment.OwnerReferences[0].Controller).To(Equal(proto.Bool(true)))
					Expect(reaperDeployment.OwnerReferences[0].Kind).To(Equal("CassandraCluster"))
					Expect(reaperDeployment.OwnerReferences[0].APIVersion).To(Equal("db.ibm.com/v1alpha1"))
					Expect(reaperDeployment.OwnerReferences[0].Name).To(Equal(cc.Name))
					Expect(reaperDeployment.OwnerReferences[0].BlockOwnerDeletion).To(Equal(proto.Bool(true)))

					reaperContainer, found := getContainerByName(reaperDeployment.Spec.Template.Spec, "reaper")
					Expect(found).To(BeTrue())
					Expect(reaperContainer.Image).To(Equal(operatorConfig.DefaultReaperImage), "default values")
					Expect(reaperContainer.ImagePullPolicy).To(Equal(v1.PullIfNotPresent), "default values")
				}

				By("reaper client should add C* cluster to reaper")
				mockReaperClient.isRunning = true
				mockReaperClient.err = nil
				Eventually(mockReaperClient.clusters).Should(BeEquivalentTo([]string{cc.Name}))
				By("reaper client should schedule all repair jobs")
				Eventually(mockReaperClient.repairSchedules).Should(HaveLen(len(cc.Spec.Reaper.RepairSchedules.Repairs)))
			})
		}
	})
})

var _ = Describe("repair schedules in reaper", func() {
	Context("when changes to CR are made", func() {
		It("should be reconciled to match the CR spec", func() {
			externalRepairSchedule := reaper.RepairSchedule{
				ID:                  "id-external",
				Owner:               "manual-creation",
				State:               "ACTIVE",
				Intensity:           1,
				KeyspaceName:        "system_traces",
				ColumnFamilies:      nil,
				SegmentCount:        0,
				RepairParallelism:   "PARALLEL",
				ScheduleDaysBetween: 4,
				Datacenters:         nil,
				IncrementalRepair:   false,
				Tables:              []string{"table2", "table3"},
				RepairThreadCount:   0,
			}

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
				},
			}

			createReadyCluster(cc)

			By("Empty repair schedules in CR should result in no reaper repair schedules")
			repairSchedules, err := mockReaperClient.RepairSchedules(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(repairSchedules).To(BeEmpty())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cc.Name, Namespace: cc.Namespace}, cc))
			cc.Spec.Reaper = &v1alpha1.Reaper{
				RepairSchedules: v1alpha1.RepairSchedules{
					Enabled: true,
					Repairs: []v1alpha1.RepairSchedule{
						{
							Keyspace:            "system_traces",
							Tables:              []string{"events"},
							ScheduleDaysBetween: 7,
							ScheduleTriggerTime: "2020-11-15T14:00:00",
							Datacenters:         []string{"dc1, dc2"},
							IncrementalRepair:   false,
							RepairThreadCount:   2,
							Intensity:           "1.0",
							RepairParallelism:   "DATACENTER_AWARE",
						},
						{
							Keyspace:            "system_auth",
							ScheduleDaysBetween: 7,
							ScheduleTriggerTime: "2021-01-06T04:00:00",
							IncrementalRepair:   false,
							RepairThreadCount:   4,
							RepairParallelism:   "PARALLEL",
						},
					},
				},
			}

			// add an externally created repair schedule to ensure that the operator doesn't delete/update not owned schedules
			mockReaperClient.repairSchedules = append(mockReaperClient.repairSchedules, externalRepairSchedule)

			Expect(k8sClient.Update(ctx, cc)).To(Succeed())
			By("CR updated with repair schedules should create them in reaper")
			Eventually(func() []reaper.RepairSchedule {
				repairSchedules, err := mockReaperClient.RepairSchedules(ctx)
				Expect(err).ToNot(HaveOccurred())
				return repairSchedules
			}).Should(Equal([]reaper.RepairSchedule{
				externalRepairSchedule,
				{
					ID:                  "id-1",
					Owner:               "cassandra-operator",
					State:               "ACTIVE",
					Intensity:           1,
					KeyspaceName:        "system_traces",
					ColumnFamilies:      nil,
					SegmentCount:        0,
					RepairParallelism:   "DATACENTER_AWARE",
					ScheduleDaysBetween: 7,
					Datacenters:         []string{"dc1, dc2"},
					IncrementalRepair:   false,
					Tables:              []string{"events"},
					RepairThreadCount:   2,
				},
				{
					ID:                  "id-2",
					Owner:               "cassandra-operator",
					State:               "ACTIVE",
					Intensity:           1,
					KeyspaceName:        "system_auth",
					ColumnFamilies:      nil,
					SegmentCount:        0,
					RepairParallelism:   "PARALLEL",
					ScheduleDaysBetween: 7,
					Datacenters:         []string{"dc1"},
					IncrementalRepair:   false,
					Tables:              nil,
					RepairThreadCount:   4,
				}}))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cc.Name, Namespace: cc.Namespace}, cc))
			cc.Spec.Reaper.RepairSchedules.Repairs = []v1alpha1.RepairSchedule{
				{
					Keyspace:            "system_traces",
					Tables:              []string{"events"},
					ScheduleDaysBetween: 7,
					ScheduleTriggerTime: "2020-11-15T14:00:00",
					Datacenters:         []string{"dc2"},
					IncrementalRepair:   false,
					RepairThreadCount:   2,
					Intensity:           "1.0",
					RepairParallelism:   "PARALLEL",
				},
			}
			Expect(k8sClient.Update(ctx, cc))

			By("A deleted or updated repair schedule in the CR should be reflected in reaper")
			Eventually(func() []reaper.RepairSchedule {
				repairSchedules, err := mockReaperClient.RepairSchedules(ctx)
				Expect(err).ToNot(HaveOccurred())
				return repairSchedules
			}).Should(Equal([]reaper.RepairSchedule{
				externalRepairSchedule,
				{
					ID:                  "id-1",
					Owner:               "cassandra-operator",
					State:               "ACTIVE",
					Intensity:           1,
					KeyspaceName:        "system_traces",
					ColumnFamilies:      nil,
					SegmentCount:        0,
					RepairParallelism:   "PARALLEL",
					ScheduleDaysBetween: 7,
					Datacenters:         []string{"dc2"},
					IncrementalRepair:   false,
					Tables:              []string{"events"},
					RepairThreadCount:   2,
				}}))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cc.Name, Namespace: cc.Namespace}, cc))
			cc.Spec.Reaper.RepairSchedules.Enabled = false
			Expect(k8sClient.Update(ctx, cc))

			By("Disabled repair schedules should result in all operator created schedules removed from Reaper")
			Eventually(func() []reaper.RepairSchedule {
				repairSchedules, err := mockReaperClient.RepairSchedules(ctx)
				Expect(err).ToNot(HaveOccurred())
				return repairSchedules
			}).Should(Equal([]reaper.RepairSchedule{externalRepairSchedule}))
		})
	})
})
