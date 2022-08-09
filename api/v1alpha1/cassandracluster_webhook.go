/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/ibm/cassandra-operator/controllers/util"
	"time"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/yaml"
	"strconv"
)

var webhookLogger = zap.NewNop().Sugar()

func SetWebhookLogger(l *zap.SugaredLogger) {
	webhookLogger = l
}

func (cc *CassandraCluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(cc).
		Complete()
}

var _ webhook.Validator = &CassandraCluster{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (cc *CassandraCluster) ValidateCreate() error {
	webhookLogger.Infof("Validating webhook has been called on create request for cluster: %s", cc.Name)

	return kerrors.NewAggregate(validateCreateUpdate(cc, nil))
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (cc *CassandraCluster) ValidateUpdate(old runtime.Object) error {
	webhookLogger.Infof("Validating webhook has been called on update request for cluster: %s", cc.Name)

	ccOld, ok := old.(*CassandraCluster)
	if !ok {
		return fmt.Errorf("old casandra cluster object: (%s) is not of type CassandraCluster", ccOld.Name)
	}

	return kerrors.NewAggregate(validateCreateUpdate(cc, ccOld))
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (cc *CassandraCluster) ValidateDelete() error {
	webhookLogger.Infof("Validating webhook has been called on delete request for cluster: %s", cc.Name)
	return nil
}

func validateCreateUpdate(cc *CassandraCluster, ccOld *CassandraCluster) (errors []error) {
	err := validateImmutableFields(cc, ccOld)
	if err != nil {
		errors = append(errors, err...)
	}

	err = generalValidation(cc)
	if err != nil {
		errors = append(errors, err...)
	}

	if cc.Spec.Cassandra != nil {
		if err = validateCassandra(cc); err != nil {
			errors = append(errors, err...)
		}
	}

	if cc.Spec.Reaper != nil {
		if err = validateReaper(cc); err != nil {
			errors = append(errors, err...)
		}
	}

	if err = validateProber(cc); err != nil {
		errors = append(errors, err...)
	}

	if err = validateIngress(cc); err != nil {
		errors = append(errors, err...)
	}

	if err = validateNetworkPolicies(cc); err != nil {
		errors = append(errors, err...)
	}

	return
}

func validateImmutableFields(cc *CassandraCluster, ccOld *CassandraCluster) (errors []error) {
	if ccOld != nil && ccOld.Spec.Cassandra != nil && cc.Spec.Cassandra != nil {
		if ccOld.Spec.Cassandra.Persistence.Enabled != cc.Spec.Cassandra.Persistence.Enabled {
			errors = append(errors, fmt.Errorf("once the persistence is set, you can't change it; you need to recreate your cluster to apply new value"))
		}

		if cc.Spec.Cassandra.Persistence.Enabled {
			if !cmp.Equal(ccOld.Spec.Cassandra.Persistence.DataVolumeClaimSpec.StorageClassName, cc.Spec.Cassandra.Persistence.DataVolumeClaimSpec.StorageClassName) {
				errors = append(errors, fmt.Errorf("once the storage class is set, you can't change it; you need to recreate your cluster to apply new value; previous `persistence.dataVolumeClaimSpec.storageClassName: (%s)` doesn't match current value: (%s)", *ccOld.Spec.Cassandra.Persistence.DataVolumeClaimSpec.StorageClassName, *cc.Spec.Cassandra.Persistence.DataVolumeClaimSpec.StorageClassName))
			}

			if ccOld.Spec.Cassandra.Persistence.DataVolumeClaimSpec.StorageClassName != nil && cc.Spec.Cassandra.Persistence.DataVolumeClaimSpec.StorageClassName != nil {
				if *ccOld.Spec.Cassandra.Persistence.DataVolumeClaimSpec.StorageClassName != *cc.Spec.Cassandra.Persistence.DataVolumeClaimSpec.StorageClassName {
					errors = append(errors, fmt.Errorf("once the storage class is set, you can't change it; you need to recreate your cluster to apply new value; previous `persistence.dataVolumeClaimSpec.storageClassName: (%s)` doesn't match current value: (%s)", *ccOld.Spec.Cassandra.Persistence.DataVolumeClaimSpec.StorageClassName, *cc.Spec.Cassandra.Persistence.DataVolumeClaimSpec.StorageClassName))
				}
			}
		}
	}

	return
}

func generalValidation(cc *CassandraCluster) (errors []error) {
	if cc.Spec.Cassandra != nil && cc.Spec.DCs != nil && cc.Spec.Cassandra.NumSeeds > 0 {
		for _, dc := range cc.Spec.DCs {
			if *dc.Replicas == 0 {
				errors = append(errors, fmt.Errorf("number of replicas can't be 0 for dc (%s)", dc.Name))
			}
			if cc.Spec.Cassandra.NumSeeds == 1 && *dc.Replicas == 1 {
				continue
			}
			if cc.Spec.Cassandra.NumSeeds > *dc.Replicas {
				errors = append(errors, fmt.Errorf("number of seeds (%d) is greater than number of replicas (%d) for dc %s", cc.Spec.Cassandra.NumSeeds, *dc.Replicas, dc.Name))
			}
		}
	}

	if cc.Spec.SystemKeyspaces.DCs != nil && cc.Spec.DCs != nil {
		for _, dc := range cc.Spec.DCs {
			dcName := getSystemKeyspaceDCByName(cc.Spec.SystemKeyspaces.DCs, dc.Name)
			if dcName.RF > *dc.Replicas {
				errors = append(errors, fmt.Errorf("replication factor (%d) is greater than number of replicas (%d) for dc %s", dcName.RF, *dc.Replicas, dc.Name))
			}
		}
	}

	if cc.Spec.Encryption.Server.NodeTLSSecret.Name != "" && cc.Spec.Encryption.Server.CATLSSecret.Name != "" {
		errors = append(errors, fmt.Errorf("either `encryption.server.caTLSSecret.name` or `encryption.server.nodeTLSSecret.name` should be set"))
	}

	if cc.Spec.Encryption.Client.CATLSSecret.Name != "" && cc.Spec.Encryption.Client.NodeTLSSecret.Name != "" {
		errors = append(errors, fmt.Errorf("either `encryption.client.caTLSSecret.name` or `encryption.client.nodeTLSSecret.name` should be set"))
	}

	return
}

func validateCassandra(cc *CassandraCluster) (errors []error) {
	if len(cc.Spec.Cassandra.ConfigOverrides) > 0 {
		err := yaml.Unmarshal([]byte(cc.Spec.Cassandra.ConfigOverrides), map[string]interface{}{})
		if err != nil {
			errors = append(errors, fmt.Errorf("cassandra config override should be a string with valid YAML: %s", err.Error()))
		}
	}

	if cc.Spec.Cassandra.Monitoring.ServiceMonitor.ScrapeInterval != "" {
		if _, err := time.ParseDuration(cc.Spec.Cassandra.Monitoring.ServiceMonitor.ScrapeInterval); err != nil {
			errors = append(errors, fmt.Errorf("service monitor scrapeInterval must be a valid time duration"))
		}
	}

	return
}

func validateReaper(cc *CassandraCluster) (errors []error) {
	if cc.Spec.Reaper.IncrementalRepair && cc.Spec.Reaper.RepairParallelism != "PARALLEL" {
		errors = append(errors, fmt.Errorf("repairParallelism must be only `PARALLEL` if incrementalRepair is true"))
	}

	if cc.Spec.Reaper.ServiceMonitor.ScrapeInterval != "" {
		if _, err := time.ParseDuration(cc.Spec.Reaper.ServiceMonitor.ScrapeInterval); err != nil {
			errors = append(errors, fmt.Errorf("service monitor scrapeInterval must be a valid time duration"))
		}
	}

	if len(cc.Spec.Reaper.RepairIntensity) > 0 {
		err := checkRepairIntensity(cc.Spec.Reaper.RepairIntensity)
		if err != nil {
			errors = append(errors, err)
		}
	}

	if cc.Spec.Reaper.RepairSchedules.Enabled && cc.Spec.Reaper.RepairSchedules.Repairs == nil {
		errors = append(errors, fmt.Errorf("repairs can't be nil when repairSchedules are enabled"))
	}

	if cc.Spec.Reaper.RepairSchedules.Enabled {
		for _, repair := range cc.Spec.Reaper.RepairSchedules.Repairs {
			if repair.IncrementalRepair && repair.RepairParallelism != "PARALLEL" {
				errors = append(errors, fmt.Errorf("repairParallelism must be only `PARALLEL` if incrementalRepair is true for Keyspace: '%s' and Tables: '%s'", repair.Keyspace, repair.Tables))
			}

			if len(repair.Intensity) > 0 {
				err := checkRepairIntensity(repair.Intensity)
				if err != nil {
					errors = append(errors, err)
				}
			}

			_, err := time.Parse(ISOFormat, repair.ScheduleTriggerTime)

			if err != nil {
				errors = append(errors, fmt.Errorf("reaper repair schedule `%s` has invalid format, should be `2000-01-31T00:00:00`", repair.ScheduleTriggerTime))
			}
		}
	}

	return
}

func validateProber(cc *CassandraCluster) (errors []error) {
	if cc.Spec.Prober.ServiceMonitor.ScrapeInterval != "" {
		if _, err := time.ParseDuration(cc.Spec.Prober.ServiceMonitor.ScrapeInterval); err != nil {
			errors = append(errors, fmt.Errorf("service monitor scrapeInterval must be a valid time duration"))
		}
	}

	return
}

func validateIngress(cc *CassandraCluster) (errors []error) {
	if len(cc.Spec.Ingress.Domain) > 0 {
		if len(cc.Spec.Ingress.Secret) == 0 {
			errors = append(errors, fmt.Errorf("tls secret must be set if an ingress domain is specified"))
		}

		if !cc.Spec.HostPort.Enabled {
			errors = append(errors, fmt.Errorf("hostPort must be enabled if an ingress domain is specified or external regions are in use"))
		}

		if !cc.Spec.NetworkPolicies.Enabled {
			webhookLogger.Warnf("Network policies are not enabled but the cluster `%s` is exposed via ingress", cc.Name)
		}
	}

	if cc.Spec.ExternalRegions.Managed != nil {
		if len(cc.Spec.Ingress.Domain) == 0 {
			errors = append(errors, fmt.Errorf("an ingress domain must be set if external regions are specified"))
		}
	}

	for _, extRegion := range cc.Spec.ExternalRegions.Managed {
		if extRegion.Domain == cc.Spec.Ingress.Domain && extRegion.Namespace == "" {
			errors = append(errors, fmt.Errorf("namespace for external region must be set if ingress and managed domain names match"))
		}
	}

	return
}

func getSystemKeyspaceDCByName(dcs []SystemKeyspaceDC, dcName string) *SystemKeyspaceDC {
	for _, systemKeyspaceDC := range dcs {
		if systemKeyspaceDC.Name == dcName {
			return &systemKeyspaceDC
		}
	}

	return nil
}

func checkRepairIntensity(intensity string) error {
	repairInt, err := strconv.ParseFloat(intensity, 32)
	if err != nil {
		return fmt.Errorf("can't parse (%s) to float", intensity)
	}
	if (repairInt < reaperRepairIntensityMin) || (repairInt > reaperRepairIntensityMax) {
		return fmt.Errorf("reaper repairIntensity value %.1f must be between 0.1 and 1.0", repairInt)
	}

	return nil
}

func validateNetworkPolicies(cc *CassandraCluster) (errors []error) {
	if !cc.Spec.NetworkPolicies.Enabled {
		return nil
	}

	cassandraAllowedPorts := []string{
		strconv.Itoa(IntraPort),
		strconv.Itoa(TlsPort),
		strconv.Itoa(JmxPort),
		strconv.Itoa(CqlPort),
		strconv.Itoa(ThriftPort),
	}

	for _, rule := range cc.Spec.NetworkPolicies.ExtraCassandraRules {
		for _, port := range rule.Ports {
			if !util.Contains(cassandraAllowedPorts, strconv.Itoa(int(port))) {
				errors = append(errors, fmt.Errorf("provided port `%v` doesn't allowed. Allowed ports: %s", port, cassandraAllowedPorts))
			}
		}
	}

	if cc.Spec.NetworkPolicies.ExtraIngressRules == nil {
		webhookLogger.Warnf("Prober is not secured by network policies. You may setup `networkPolicies.extraIngressRules` to enable network policies for prober component")
	}

	return errors
}
