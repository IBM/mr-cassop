package integration

import (
	"bufio"
	"fmt"
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/names"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
	"strings"
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

var _ = Describe("cassandra jvm configs", func() {
	cc := &v1alpha1.CassandraCluster{
		ObjectMeta: cassandraObjectMeta,
		Spec: v1alpha1.CassandraClusterSpec{
			DCs: []v1alpha1.DC{
				{
					Name:     "dc1",
					Replicas: proto.Int32(6),
				},
			},
			Cassandra: &v1alpha1.Cassandra{
				JVMOptions: []string{
					"-Xmx1024M",
					"-Xms512M",
				},
			},
			AdminRoleSecretName: "admin-role",
			ImagePullSecretName: "pullSecretName",
		},
	}

	It("should be overriden", func() {
		createReadyCluster(cc)

		cm := &v1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.ConfigMap(cc.Name), Namespace: cc.Namespace}, cm))
		jvmOptions, exists := cm.Data["jvm.options"]
		Expect(exists).To(BeTrue())

		for _, option := range cc.Spec.Cassandra.JVMOptions {
			scanner := bufio.NewScanner(strings.NewReader(jvmOptions))
			found := false
			for scanner.Scan() {
				if option == scanner.Text() {
					found = true
					break
				}
			}

			if !found {
				Fail(fmt.Sprintf("JVM option override was not found in the jvm.options file. File content: \n%s\n\n. Searched option: %s.", jvmOptions, option))
			}
		}
	})
})
