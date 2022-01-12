package names

import (
	"fmt"
	"os"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
)

const (
	cassandraOperator = "cassandra-operator"
)

func ProberService(clusterName string) string {
	return clusterName + "-cassandra-prober"
}

func ProberDeployment(clusterName string) string {
	return clusterName + "-cassandra-prober"
}

func ProberRole(clusterName string) string {
	return clusterName + "-cassandra-prober-role"
}

func ProberRoleBinding(clusterName string) string {
	return clusterName + "-cassandra-prober-rolebinding"
}

func ProberServiceAccount(clusterName string) string {
	return clusterName + "-cassandra-prober-serviceaccount"
}

func ProberIngress(clusterName string) string {
	return clusterName + "-cassandra-prober"
}

func proberIngressSubdomain(clusterName, namespace string) string {
	return fmt.Sprintf("%s-%s", namespace, ProberService(clusterName))
}

func ProberIngressHost(clusterName, namespace string, domain string) string {
	return fmt.Sprintf("%s.%s", proberIngressSubdomain(clusterName, namespace), domain)
}

func ProberIngressDomain(cc *dbv1alpha1.CassandraCluster, externalRegion dbv1alpha1.ExternalRegion) string {
	namespace := cc.Namespace
	if len(externalRegion.Namespace) != 0 {
		namespace = externalRegion.Namespace
	}
	return ProberIngressHost(cc.Name, namespace, externalRegion.Domain)
}

func ReaperDeployment(clusterName, dcName string) string {
	return DC(clusterName, dcName) + "-reaper"
}

func ReaperService(clusterName string) string {
	return clusterName + "-reaper"
}

func ShiroConfigMap(clusterName string) string {
	return clusterName + "-shiro-configmap"
}

func PrometheusConfigMap(clusterName string) string {
	return clusterName + "-prometheus-configmap"
}

func CollectdConfigMap(clusterName string) string {
	return clusterName + "-collectd-configmap"
}

func MaintenanceConfigMap(clusterName string) string {
	return clusterName + "-maintenance-configmap"
}

func DC(clusterName, dcName string) string {
	return clusterName + "-cassandra-" + dcName
}

func DCService(clusterName, dcName string) string {
	return DC(clusterName, dcName)
}

func ConfigMap(clusterName string) string {
	return clusterName + "-cassandra-config"
}

func PodsConfigConfigmap(clusterName string) string {
	return clusterName + "-pods-config"
}

func ActiveAdminSecret(clusterName string) string {
	return clusterName + "-auth-active-admin"
}

func AdminAuthConfigSecret(clusterName string) string {
	return clusterName + "-auth-config-admin"
}

func PodIPsConfigMap(clusterName string) string {
	return clusterName + "-pod-ips"
}

func OperatorCassandraCM() string {
	return cassandraOperator + "-cassandra-init-config"
}

func OperatorCollectdCM() string {
	return cassandraOperator + "-collectd-configmap"
}

func OperatorCassandraConfigCM() string {
	return cassandraOperator + "-cassandra-init-config"
}

func OperatorPrometheusCM() string {
	return cassandraOperator + "-prometheus-configmap"
}

func OperatorShiroCM() string {
	return cassandraOperator + "-shiro-configmap"
}

func OperatorClientTLSDir(cc *dbv1alpha1.CassandraCluster) string {
	return fmt.Sprintf("%s/%s-%s", os.Getenv("HOME"), cc.Namespace, cc.Name)
}

func OperatorWebhookTLSDir() string {
	return fmt.Sprintf("%s/%s", os.Getenv("HOME"), "k8s-webhook-server/serving-certs")
}

func ValidatingWebhookName() string {
	return cassandraOperator + "-cassandracluster-validation"
}

func WebhooksServiceName() string {
	return cassandraOperator + "-webhooks"
}
