package events

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

const (
	EventRecorderNameCassandraCluster = "cassandra-cluster" // appears in the 'From' column of the events list

	EventAdminRoleSecretNotFound          = "AdminRoleSecretNotFound"
	EventAdminRoleSecretInvalid           = "AdminRoleSecretInvalid"
	EventAdminRoleUpdateFailed            = "AdminRoleUpdateFailed"
	EventAdminRoleCreated                 = "AdminRoleCreated"
	EventDefaultAdminRoleDropped          = "DefaultAdminRoleRemoved"
	EventRoleSecretNotFound               = "RolesSecretNotFound"
	EventTLSSecretNotFound                = "TLSSecretNotFound"
	EventTLSSecretInvalid                 = "TLSSecretInvalid"
	EventInsecureSetup                    = "InsecureSetup"
	EventInvalidRole                      = "InvalidRole"
	CassandraConfigInvalid                = "InvalidCassandraConfig"
	EventDCDecommissionBlocked            = "DCDecommissionBlocked"
	EventCassandraClusterNotFound         = "CassandraClusterNotFound"
	EventCassandraBackupNotFound          = "CassandraBackupNotFound"
	EventStorageCredentialsSecretNotFound = "StorageCredentialsSecretNotFound"
	EventStorageCredentialsSecretInvalid  = "StorageCredentialsSecretInvalid"

	EventAdminRoleChanged = "AdminRoleChanged"
	EventRegionInit       = "RegionInit"
	EventDCInit           = "DCInit"
	EventCQLScriptSuccess = "CQLScriptSuccess"
	EventCQLScriptFailed  = "CQLScriptFailed"
)

// EventReason is the reason why the event was created. The value appears in the 'Reason' tab of the events list
type EventReason string

// NewEventRecorder wraps the provided event recorder
func NewEventRecorder(recorder record.EventRecorder) *EventRecorder {
	return &EventRecorder{
		Recorder: recorder,
	}
}

// EventRecorder handles the operations for events
type EventRecorder struct {
	Recorder record.EventRecorder
}

// Warning creates a 'warning' type event
func (e *EventRecorder) Warning(object runtime.Object, reason EventReason, message string) {
	e.Recorder.Event(object, v1.EventTypeWarning, string(reason), message)
}

// Normal creates a 'normal' type event
func (e *EventRecorder) Normal(object runtime.Object, reason EventReason, message string) {
	e.Recorder.Event(object, v1.EventTypeNormal, string(reason), message)
}
