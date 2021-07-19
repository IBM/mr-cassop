package integration

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/names"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type test struct {
	name string
	cc   *v1alpha1.CassandraCluster
}

var _ = Describe("reaper deployment", func() {
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
					Reaper: &v1alpha1.Reaper{
						DCs: []v1alpha1.DC{
							{
								Name:     "dc1",
								Replicas: proto.Int32(1),
							},
							{
								Name:     "dc2",
								Replicas: proto.Int32(1),
							},
						},
						ScheduleRepairs: v1alpha1.ScheduleRepairs{
							Enabled: true,
							Repairs: []v1alpha1.Repair{
								{
									Keyspace:            "system_traces",
									Tables:              []string{"events"},
									ScheduleDaysBetween: 7,
									ScheduleTriggerTime: "2020-11-15T14:00:00",
									Datacenters:         []string{"dc1, dc2"},
									IncrementalRepair:   false,
									RepairThreadCount:   2,
									Intensity:           "1.0",
									RepairParallelism:   "datacenter_aware",
								},
								{
									Keyspace:            "system_auth",
									ScheduleDaysBetween: 7,
									ScheduleTriggerTime: "2021-01-06T04:00:00",
									IncrementalRepair:   false,
									RepairThreadCount:   4,
									RepairParallelism:   "parallel",
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
				createReadyCluster(cc)
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
				_ = mockReaperClient.AddCluster(ctx, cc.Name, "seed")
				Expect(mockReaperClient.clusterExists).To(BeTrue())

				By("reaper client should schedule all repair jobs")
				for _, repair := range cc.Spec.Reaper.ScheduleRepairs.Repairs {
					_ = mockReaperClient.ScheduleRepair(ctx, cc.Name, repair)
				}
				Expect(mockReaperClient.repairs).To(Equal(cc.Spec.Reaper.ScheduleRepairs.Repairs))
			})
		}
	})
})
