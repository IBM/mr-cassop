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
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var webhookLogger = zap.NewNop().Sugar()

func SetWebhookLogger(l *zap.SugaredLogger) {
	webhookLogger = l
}

func (r *CassandraCluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

var _ webhook.Validator = &CassandraCluster{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *CassandraCluster) ValidateCreate() error {
	webhookLogger.Infof("Validating webhook has been called on create request for cluster: %s", r.Name)
	return validateCreateUpdate(r)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *CassandraCluster) ValidateUpdate(old runtime.Object) error {
	webhookLogger.Infof("Validating webhook has been called on update request for cluster: %s", r.Name)
	return validateCreateUpdate(r)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *CassandraCluster) ValidateDelete() error {
	webhookLogger.Infof("Validating webhook has been called on delete request for cluster: %s", r.Name)
	return nil
}

func validateCreateUpdate(r *CassandraCluster) error {
	var errors []error

	if r.Spec.Reaper != nil {
		if err := validateReaper(r); err != nil {
			errors = append(errors, err...)
		}
	}

	// Todo: implement other checks discussed in https://github.com/TheWeatherCompany/cassandra-operator/issues/55

	// If `errors` is empty this returns nil
	return kerrors.NewAggregate(errors)
}

func validateReaper(r *CassandraCluster) (errors []error) {
	if r.Spec.Reaper.IncrementalRepair && r.Spec.Reaper.RepairParallelism != "PARALLEL" {
		errors = append(errors, fmt.Errorf("repairParallelism must be only `PARALLEL` if incrementalRepair is true"))
	}

	if r.Spec.Reaper.RepairSchedules.Enabled {
		for _, repair := range r.Spec.Reaper.RepairSchedules.Repairs {
			if repair.IncrementalRepair && repair.RepairParallelism != "PARALLEL" {
				errors = append(errors, fmt.Errorf("repairParallelism must be only `PARALLEL` if incrementalRepair is true for Keyspace: '%s' and Tables: '%s'", repair.Keyspace, repair.Tables))
			}
		}
	}

	return
}
