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
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ibm/cassandra-operator/controllers/util"

	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func (cb *CassandraBackup) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(cb).
		Complete()
}

var _ webhook.Validator = &CassandraBackup{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (cb *CassandraBackup) ValidateCreate() error {
	webhookLogger.Debugf("Validating webhook has been called on create request for backup: %s", cb.Name)

	return kerrors.NewAggregate(validateBackupCreateUpdate(cb))
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (cb *CassandraBackup) ValidateUpdate(old runtime.Object) error {
	webhookLogger.Debugf("Validating webhook has been called on update request for backup: %s", cb.Name)

	cbOld, ok := old.(*CassandraBackup)
	if !ok {
		return fmt.Errorf("old casandra cluster object: (%s) is not of type CassandraBackup", cbOld.Name)
	}

	return kerrors.NewAggregate(validateBackupCreateUpdate(cb))
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (cb *CassandraBackup) ValidateDelete() error {
	webhookLogger.Debugf("Validating webhook has been called on delete request for backup: %s", cb.Name)
	return nil
}

func validateBackupCreateUpdate(cb *CassandraBackup) (verrors []error) {
	if err := validateStorageLocation(cb.Spec.StorageLocation); err != nil {
		verrors = append(verrors, err)
	}

	if err := validateDuration(cb.Spec.Duration); err != nil {
		verrors = append(verrors, err)
	}

	return verrors
}

func validateDuration(durationStr string) error {
	if len(durationStr) == 0 {
		return nil
	}

	allowedDurationUnits := []string{"days", "hours", "microseconds", "milliseconds", "minutes", "nanoseconds", "seconds"}
	duration := strings.Split(durationStr, " ")
	validationErr := fmt.Errorf(
		"duration should be in format \"amount unit\", where amount is an integer value and unit is one of the following values: %v",
		allowedDurationUnits,
	)
	if len(duration) != 2 {
		return validationErr
	}

	if _, err := strconv.ParseInt(duration[0], 10, 64); err != nil {
		return validationErr
	}

	if !util.Contains(allowedDurationUnits, strings.TrimSpace(strings.ToLower(duration[1]))) {
		return validationErr
	}

	return nil
}

func validateStorageLocation(location string) error {
	index := 0
	if index = strings.Index(location, "://"); index < 0 {
		return errors.New("storage location should be in format 'protocol://backup/location'")
	}

	supportedProtocols := []string{
		string(StorageProviderS3),
		string(StorageProviderMinio),
		string(StorageProviderOracle),
		string(StorageProviderCeph),
		string(StorageProviderGCP),
		string(StorageProviderAzure),
	}
	requestedProtocol := location[:index]
	if !util.Contains(supportedProtocols, requestedProtocol) {
		return fmt.Errorf("protocol %s is not supported. Should be one of the following: %v", requestedProtocol, supportedProtocols)
	}

	return nil
}
