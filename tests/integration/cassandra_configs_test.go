package integration

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/names"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

var _ = Describe("cassandra cluster configs", func() {
	cc := &v1alpha1.CassandraCluster{
		ObjectMeta: cassandraObjectMeta,
		Spec: v1alpha1.CassandraClusterSpec{
			DCs: []v1alpha1.DC{
				{
					Name:     "dc1",
					Replicas: proto.Int32(6),
				},
				{
					Name:     "dc2",
					Replicas: proto.Int32(6),
				},
			},
			Cassandra: &v1alpha1.Cassandra{
				ConfigOverrides: `concurrent_reads: 40
concurrent_writes: 45
non_existing_config_in_default_file: something
back_pressure_strategy:
- class_name: org.apache.cassandra.net.RateBasedBackPressure
  parameters:
  - factor: 4
    flow: FAST
    high_ratio: 0.8
`,
			},
			AdminRoleSecretName: "admin-role",
			ImagePullSecretName: "pullSecretName",
		},
	}

	It("should be overriden", func() {
		createReadyCluster(cc)

		cm := &v1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.ConfigMap(cc.Name), Namespace: cc.Namespace}, cm))
		config, exists := cm.Data["cassandra.yaml"]
		Expect(exists).To(BeTrue())
		cassandraYaml := make(map[string]interface{})
		Expect(yaml.Unmarshal([]byte(config), &cassandraYaml)).To(Succeed())

		Expect(cassandraYaml).To(HaveKeyWithValue("concurrent_reads", float64(40)))
		Expect(cassandraYaml).To(HaveKeyWithValue("concurrent_writes", float64(45)))
		Expect(cassandraYaml).To(HaveKeyWithValue("non_existing_config_in_default_file", "something"))
		Expect(cassandraYaml).To(HaveKeyWithValue("back_pressure_strategy", []interface{}{
			map[string]interface{}{
				"class_name": "org.apache.cassandra.net.RateBasedBackPressure",
				"parameters": []interface{}{
					map[string]interface{}{
						"factor":     float64(4),
						"flow":       "FAST",
						"high_ratio": 0.8,
					},
				},
			},
		}))
	})
})
