package names

import "github.com/ibm/cassandra-operator/api/v1alpha1"

const (
	cassandraOperator = "cassandra-operator"
)

func ProberService(cc *v1alpha1.CassandraCluster) string {
	return cc.Name + "-cassandra-prober"
}

func ProberDeployment(cc *v1alpha1.CassandraCluster) string {
	return cc.Name + "-cassandra-prober"
}

func ProberRole(cc *v1alpha1.CassandraCluster) string {
	return cc.Name + "-cassandra-prober-role"
}

func ProberRoleBinding(cc *v1alpha1.CassandraCluster) string {
	return cc.Name + "-cassandra-prober-rolebinding"
}

func ProberServiceAccount(cc *v1alpha1.CassandraCluster) string {
	return cc.Name + "-cassandra-prober-serviceaccount"
}

func ProberSources(cc *v1alpha1.CassandraCluster) string {
	return cc.Name + "-cassandra-prober-sources"
}

func ReaperDeployment(cc *v1alpha1.CassandraCluster, dcName string) string {
	return DC(cc, dcName) + "-reaper"
}

func ReaperService(cc *v1alpha1.CassandraCluster) string {
	return cc.Name + "-reaper"
}

func ReaperCqlConfigMap(cc *v1alpha1.CassandraCluster) string {
	return ReaperService(cc) + "-cql-configmap"
}

func RepairsConfigMap(cc *v1alpha1.CassandraCluster) string {
	return cc.Name + "-repairs-configmap"
}

func ShiroConfigMap(cc *v1alpha1.CassandraCluster) string {
	return cc.Name + "-shiro-configmap"
}

func MaintenanceConfigMap(cc *v1alpha1.CassandraCluster) string {
	return cc.Name + "-maintenance-configmap"
}

func RolesSecret(cc *v1alpha1.CassandraCluster) string {
	return cc.Name + "-roles-secret"
}

func ScriptsConfigMap(cc *v1alpha1.CassandraCluster) string {
	return cc.Name + "-scripts-configmap"
}

func JMXRemoteSecret(cc *v1alpha1.CassandraCluster) string {
	return cc.Name + "-jmxremote-secret"
}

func DC(cc *v1alpha1.CassandraCluster, dcName string) string {
	return cc.Name + "-cassandra-" + dcName
}

func DCService(cc *v1alpha1.CassandraCluster, dcName string) string {
	return DC(cc, dcName)
}

func ConfigMap(cc *v1alpha1.CassandraCluster, dcName string) string {
	return DC(cc, dcName) + "-configmap"
}

func OperatorScriptsCM() string {
	return cassandraOperator + "-scripts-configmap"
}

func OperatorProberSourcesCM() string {
	return cassandraOperator + "-prober-sources-configmap"
}

func OperatorCassandraConfigCM() string {
	return cassandraOperator + "-cassandra-config-configmap"
}

func OperatorShiroCM() string {
	return cassandraOperator + "-shiro-configmap"
}

func OperatorMaintenanceCM() string {
	return cassandraOperator + "-maintenance-configmap"
}
