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

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func (cr *CassandraRestore) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(cr).
		Complete()
}

var _ webhook.Validator = &CassandraRestore{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (cr *CassandraRestore) ValidateCreate() error {
	webhookLogger.Debugf("Validating webhook has been called on create request for restore: %s", cr.Name)

	return kerrors.NewAggregate(validateRestoreCreateUpdate(cr))
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (cr *CassandraRestore) ValidateUpdate(old runtime.Object) error {
	webhookLogger.Debugf("Validating webhook has been called on update request for restore: %s", cr.Name)

	cbOld, ok := old.(*CassandraRestore)
	if !ok {
		return fmt.Errorf("old cassandra cluster object: (%s) is not of type CassandraRestore", cbOld.Name)
	}

	return kerrors.NewAggregate(validateRestoreCreateUpdate(cr))
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (cr *CassandraRestore) ValidateDelete() error {
	webhookLogger.Debugf("Validating webhook has been called on delete request for restore: %s", cr.Name)
	return nil
}

func validateRestoreCreateUpdate(cr *CassandraRestore) (verrors []error) {
	if len(cr.Spec.CassandraBackup) == 0 {
		if len(cr.Spec.StorageLocation) == 0 || len(cr.Spec.SnapshotTag) == 0 || len(cr.Spec.SecretName) == 0 {
			verrors = append(verrors, errors.New(".spec.storageLocation, .spec.snapshotTag and .spec.secretName should be set if .spec.cassandraBackup is not set"))
		} else {
			if err := validateStorageLocation(cr.Spec.StorageLocation); err != nil {
				verrors = append(verrors, err)
			}
		}
	}

	return verrors
}
