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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	CassandraClusterInstance  = "cassandra-cluster-instance"
	CassandraClusterComponent = "cassandra-cluster-component"
	CassandraClusterDC        = "cassandra-cluster-dc"

	CassandraClusterComponentProber    = "prober"
	CassandraClusterComponentKwatcher  = "kwatcher"
	CassandraClusterComponentReaper    = "reaper"
	CassandraClusterComponentCassandra = "cassandra"
)

var (
	CassandraUsername = "cassandra"
	CassandraPassword = "cassandra"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CassandraClusterSpec defines the desired state of CassandraCluster
type CassandraClusterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// +kubebuilder:validation:MinItems:=1
	DCs []DC `json:"dcs"`
	// +kubebuilder:validation:MinLength:=1
	ImagePullSecretName  string          `json:"imagePullSecretName"`
	CQLConfigMapLabelKey string          `json:"cqlConfigMapLabelKey,omitempty"`
	Cassandra            *Cassandra      `json:"cassandra,omitempty"`
	SystemKeyspaces      SystemKeyspaces `json:"systemKeyspaces,omitempty"`
	Prober               Prober          `json:"prober,omitempty"`
	Kwatcher             Kwatcher        `json:"kwatcher,omitempty"`
	Reaper               *Reaper         `json:"reaper,omitempty"`
	//JMX                  JMX             `json:"jmx,omitempty"` //TODO part of auth  implementation
	//NodetoolUser         string          `json:"nodetoolUser,omitempty"` //TODO part of auth implementation
	//HostPort             HostPort        `json:"hostPort,omitempty"` //TODO part of hostport implementation
	//Monitoring           Monitoring      `json:"monitoring,omitempty"` //TODO part of monitoring implementation
	//JVM                  JVM             `json:"jvm,omitempty"`
}

// TODO: choose which fields will go into the Reaper ConfigMap
type Reaper struct {
	Image string `json:"image,omitempty"`
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	ImagePullPolicy v1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// +kubebuilder:validation:MinLength=1
	Keyspace string `json:"keyspace"`
	DCs      []DC   `json:"dcs,omitempty"`
	// +kubebuilder:validation:Enum=each
	DatacenterAvailability                 string                  `json:"datacenterAvailability,omitempty"`
	Tolerations                            []v1.Toleration         `json:"tolerations,omitempty"`
	NodeSelector                           map[string]string       `json:"nodeSelector,omitempty"`
	IncrementalRepair                      bool                    `json:"incrementalRepair,omitempty"`
	RepairIntensity                        string                  `json:"repairIntensity,omitempty"` // value between 0.0 and 1.0, but must never be 0.0.
	RepairManagerSchedulingIntervalSeconds int32                   `json:"repairManagerSchedulingIntervalSeconds,omitempty"`
	BlacklistTWCS                          bool                    `json:"blacklistTWCS,omitempty"`
	Resources                              v1.ResourceRequirements `json:"resources,omitempty"`
	ScheduleRepairs                        ScheduleRepairs         `json:"scheduleRepairs,omitempty"`
	AutoScheduling                         AutoScheduling          `json:"autoScheduling,omitempty"`
}

//type HostPort struct {//TODO part of hostport implementation
//	Enabled   bool `json:"enabled"`
//	UseHostIP bool `json:"useHostIP,omitempty"`
//}

//type Monitoring struct { //TODO part of monitoring implementation
//	Enabled bool  `json:"enabled"`
//	Port    int32 `json:"port"`
//}

//type JVM struct { //TODO implement usage of those parameters
//MaxHeapSize                    string   `json:"maxHeapSize,omitempty"`
//HeapNewSize                    string   `json:"heapNewSize,omitempty"`
//MigrationTaskWaitSeconds       int32    `json:"migrationTaskWaitSeconds,omitempty"`
//RingDelayMS                    int32    `json:"ringDelayMS,omitempty"`
//MaxGCPauseMillis               int32    `json:"maxGCPauseMillis,omitempty"`
//G1RSetUpdatingPauseTimePercent int32    `json:"g1RSetUpdatingPauseTimePercent,omitempty"`
//InitiatingHeapOccupancyPercent int32    `json:"initiatingHeapOccupancyPercent,omitempty"`
//}

//type JMX struct { //TODO usage of those parrameters should be fully implemented during auth implementation
//	// Authentication available options: false, local_files, internal
//	// If internals is selected for C* <3.6, authentication will default to local_files
//	// +kubebuilder:validation:Enum:=false;local_files;internal
//	Authentication string `json:"authentication,omitempty"`
//	SSL            bool   `json:"ssl,omitempty"` //todo can be only false until auth implemented
//}

type ScheduleRepairs struct {
	Enabled        bool     `json:"enabled,omitempty"`
	StartRepairsIn string   `json:"startRepairsIn,omitempty"`
	Repairs        []Repair `json:"repairs,omitempty"`
}

type Repair struct {
	Keyspace            string   `json:"keyspace" url:"keyspace"`
	Owner               string   `json:"owner,omitempty" url:"owner"`
	Tables              []string `json:"tables,omitempty" url:"tables"`
	ScheduleDaysBetween int32    `json:"scheduleDaysBetween,omitempty" url:"scheduleDaysBetween"`
	ScheduleTriggerTime string   `json:"scheduleTriggerTime,omitempty" url:"scheduleTriggerTime"`
	Datacenters         []string `json:"datacenters,omitempty" url:"datacenters"`
	IncrementalRepair   bool     `json:"incrementalRepair,omitempty" url:"incrementalRepair"`
	// +kubebuilder:validation:Minimum:=1
	// +kubebuilder:validation:Maximum:=4
	RepairThreadCount int32  `json:"repairThreadCount" url:"repairThreadCount"`
	Intensity         string `json:"intensity" url:"intensity"` // value between 0.0 and 1.0, but must never be 0.0.
	// +kubebuilder:validation:Enum:=sequential;parallel;datacenter_aware
	RepairParallelism string `json:"repairParallelism" url:"repairParallelism"`
}

type AutoScheduling struct {
	Enabled                 bool     `json:"enabled,omitempty"`
	InitialDelayPeriod      string   `json:"initialDelayPeriod,omitempty"`
	PeriodBetweenPolls      string   `json:"periodBetweenPolls,omitempty"`
	TimeBeforeFirstSchedule string   `json:"timeBeforeFirstSchedule,omitempty"`
	ScheduleSpreadPeriod    string   `json:"scheduleSpreadPeriod,omitempty"`
	ExcludedKeyspaces       []string `json:"excludedKeyspaces,omitempty"`
}

type DC struct {
	// +kubebuilder:validation:MinLength:=1
	// +kubebuilder:validation:MaxLength:=63
	// +kubebuilder:validation:Pattern:=^[a-z0-9][a-z0-9\-]*$
	Name string `json:"name"`
	// +kubebuilder:validation:Minimum:=0
	Replicas *int32 `json:"replicas"`
}

type Cassandra struct {
	Image string `json:"image,omitempty"`
	// +kubebuilder:validation:Enum:=Always;Never;IfNotPresent
	ImagePullPolicy v1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	Resources       v1.ResourceRequirements `json:"resources,omitempty"`
	//LogLevel                       string   `json:"logLevel,omitempty"`
	//AdditionalSeeds                          []string `json:"additionalSeeds,omitempty"`
	//RackName                       string   `json:"rackName,omitempty"`
	//PreferLocal                    bool     `json:"preferLocal,omitempty"`
	// +kubebuilder:validation:Minimum:=1
	NumSeeds int32 `json:"numSeeds,omitempty"`
	// internalAuth: (true|false), configures Cassandra to use internal authentication
	// https://docs.datastax.com/en/archived/cassandra/2.1/cassandra/security/security_config_native_authenticate_t.html
	// https://docs.datastax.com/en/cassandra/3.0/cassandra/configuration/secureConfigNativeAuth.html
	//InternalAuth bool `json:"internalAuth,omitempty"` //TODO part of auth implementation
}

type Prober struct {
	Image string `json:"image,omitempty"`
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	ImagePullPolicy v1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	Resources       v1.ResourceRequirements `json:"resources,omitempty"`
	Debug           bool                    `json:"debug,omitempty"`
	Jolokia         Jolokia                 `json:"jolokia,omitempty"`
}

type Jolokia struct {
	Image string `json:"image,omitempty"`
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	ImagePullPolicy v1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	Resources       v1.ResourceRequirements `json:"resources,omitempty"`
}

type Kwatcher struct {
	Image string `json:"image,omitempty"`
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	ImagePullPolicy v1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	Resources       v1.ResourceRequirements `json:"resources,omitempty"`
}

// +kubebuilder:validation:MinLength:=1
// +kubebuilder:validation:MaxLength:=48
// +kubebuilder:validation:Pattern:=^[a-zA-Z]\w+$
type KeyspaceName string

type SystemKeyspaces struct {
	Names []KeyspaceName     `json:"names,omitempty"`
	DCs   []SystemKeyspaceDC `json:"dcs,omitempty"`
}

type SystemKeyspaceDC struct {
	// +kubebuilder:validation:MinLength:=1
	// +kubebuilder:validation:MaxLength:=63
	// +kubebuilder:validation:Pattern:=^[a-z0-9][a-z0-9\-]*$
	Name string `json:"name"`
	// +kubebuilder:validation:Minimum:=1
	RF int32 `json:"rf"`
}

// CassandraClusterStatus defines the observed state of CassandraCluster
type CassandraClusterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// CassandraCluster is the Schema for the cassandraclusters API
type CassandraCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CassandraClusterSpec   `json:"spec"`
	Status CassandraClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CassandraClusterList contains a list of CassandraCluster
type CassandraClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CassandraCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CassandraCluster{}, &CassandraClusterList{})
}
