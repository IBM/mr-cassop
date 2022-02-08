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
	CassandraClusterChecksum  = "cassandra-cluster-checksum"
	CassandraClusterSeed      = "cassandra-cluster-seed"

	CassandraClusterComponentProber    = "prober"
	CassandraClusterComponentReaper    = "reaper"
	CassandraClusterComponentCassandra = "cassandra"

	CassandraAgentTlp         = "tlp"
	CassandraAgentInstaclustr = "instaclustr"
	CassandraAgentDatastax    = "datastax"

	CassandraDefaultRole           = "cassandra"
	CassandraDefaultPassword       = "cassandra"
	CassandraOperatorAdminRole     = "admin-role"
	CassandraOperatorAdminPassword = "admin-password"
	CassandraOperatorJmxUsername   = "jmx-username"
	CassandraOperatorJmxPassword   = "jmx-password"

	ProberServicePort    = 80
	JolokiaContainerPort = 8080
	ProberContainerPort  = 8888

	ReaperAppPort   = 8080
	ReaperAdminPort = 8081

	IntraPort       = 7000
	TlsPort         = 7001
	JmxPort         = 7199
	TlpPort         = 8090
	CqlPort         = 9042
	DatastaxPort    = 9103
	ThriftPort      = 9160
	InstaclustrPort = 9500

	ReaperReplicasNumber     = 1
	reaperRepairIntensityMin = 0.1
	reaperRepairIntensityMax = 1.0

	InternodeEncryptionNone = "none"

	HMS       = "15:04:05"
	ISOFormat = "2006-01-02T" + HMS // YYYY-MM-DDThh:mm:ss format (reaper API dates do not include timezone)
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Run "make" to regenerate code after modifying this file.

// CassandraClusterSpec defines the desired state of CassandraCluster
type CassandraClusterSpec struct {
	// +kubebuilder:validation:MinItems:=1
	DCs []DC `json:"dcs"`
	// +kubebuilder:validation:MinLength:=1
	ImagePullSecretName  string     `json:"imagePullSecretName"`
	CQLConfigMapLabelKey string     `json:"cqlConfigMapLabelKey,omitempty"`
	Cassandra            *Cassandra `json:"cassandra,omitempty"`
	// +kubebuilder:validation:MinLength:=1
	AdminRoleSecretName  string          `json:"adminRoleSecretName"`
	RolesSecretName      string          `json:"rolesSecretName,omitempty"`
	TopologySpreadByZone *bool           `json:"topologySpreadByZone,omitempty"`
	Maintenance          []Maintenance   `json:"maintenance,omitempty" diff:"maintenance"`
	SystemKeyspaces      SystemKeyspaces `json:"systemKeyspaces,omitempty"`
	Ingress              Ingress         `json:"ingress,omitempty"`
	ExternalRegions      ExternalRegions `json:"externalRegions,omitempty"`
	Prober               Prober          `json:"prober,omitempty"`
	Reaper               *Reaper         `json:"reaper,omitempty"`
	HostPort             HostPort        `json:"hostPort,omitempty"`
	// Authentication is always enabled and by default is set to `internal`. Available options: `internal`, `local_files`.
	// +kubebuilder:validation:Enum:=local_files;internal
	JMXAuth    string     `json:"jmxAuth,omitempty"`
	Encryption Encryption `json:"encryption,omitempty"`
}

type ExternalRegions struct {
	Managed   []ManagedRegion   `json:"managed,omitempty"`
	Unmanaged []UnmanagedRegion `json:"unmanaged,omitempty"`
}

type ManagedRegion struct {
	Domain    string `json:"domain"`
	Namespace string `json:"namespace,omitempty"`
}

type UnmanagedRegion struct {
	Seeds []string           `json:"seeds"`
	DCs   []SystemKeyspaceDC `json:"dcs"`
}

type Reaper struct {
	Image string `json:"image,omitempty"`
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	ImagePullPolicy v1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// +kubebuilder:validation:MinLength=1
	Keyspace     string            `json:"keyspace,omitempty"`
	Tolerations  []v1.Toleration   `json:"tolerations,omitempty"`
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// +kubebuilder:validation:Minimum=1
	HangingRepairTimeoutMins               int32                   `json:"hangingRepairTimeoutMins,omitempty"`
	IncrementalRepair                      bool                    `json:"incrementalRepair,omitempty"`
	RepairIntensity                        string                  `json:"repairIntensity,omitempty"` // value between 0.0 and 1.0, but must never be 0.0.
	RepairManagerSchedulingIntervalSeconds int32                   `json:"repairManagerSchedulingIntervalSeconds,omitempty"`
	BlacklistTWCS                          bool                    `json:"blacklistTWCS,omitempty"`
	Resources                              v1.ResourceRequirements `json:"resources,omitempty"`
	ServiceMonitor                         ServiceMonitor          `json:"serviceMonitor,omitempty"`
	RepairSchedules                        RepairSchedules         `json:"repairSchedules,omitempty"`
	AutoScheduling                         AutoScheduling          `json:"autoScheduling,omitempty"`
	// +kubebuilder:validation:Enum=DATACENTER_AWARE;SEQUENTIAL;PARALLEL
	RepairParallelism string `json:"repairParallelism,omitempty"`
	// +kubebuilder:validation:Minimum=1
	RepairRunThreads int32 `json:"repairRunThreads,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4
	RepairThreadCount int32 `json:"repairThreadCount,omitempty"`
	// +kubebuilder:validation:Minimum=1
	SegmentCountPerNode int32 `json:"segmentCountPerNode,omitempty"`
	// +kubebuilder:validation:Minimum=1
	MaxParallelRepairs int32 `json:"maxParallelRepairs,omitempty"`
}

type HostPort struct {
	Enabled           bool     `json:"enabled,omitempty"`
	UseExternalHostIP bool     `json:"useExternalHostIP,omitempty"`
	Ports             []string `json:"ports,omitempty"`
}

type Monitoring struct {
	Enabled bool `json:"enabled,omitempty"`
	// +kubebuilder:validation:Enum=instaclustr;datastax;tlp
	Agent          string         `json:"agent,omitempty"`
	ServiceMonitor ServiceMonitor `json:"serviceMonitor,omitempty"`
}

type ServiceMonitor struct {
	Enabled        bool              `json:"enabled"`
	Namespace      string            `json:"namespace,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
	ScrapeInterval string            `json:"scrapeInterval,omitempty"`
}

type Encryption struct {
	Server ServerEncryption `json:"server,omitempty"`
	Client ClientEncryption `json:"client,omitempty"`
}

// ServerEncryption defines encryption between Cassandra nodes
type ServerEncryption struct {
	// InternodeEncryption enables server encryption and by default is set to `none`. Available options: `none`, `rack`, `dc`, `all`.
	// +kubebuilder:validation:Enum:=none;rack;dc;all
	InternodeEncryption string `json:"internodeEncryption,omitempty"`
	// TLS Secret fields configuration
	TLSSecret                   TLSSecret `json:"tlsSecret,omitempty"`
	RequireEndpointVerification bool      `json:"requireEndpointVerification,omitempty"`
	RequireClientAuth           *bool     `json:"requireClientAuth,omitempty"`
	Protocol                    string    `json:"protocol,omitempty"`
	Algorithm                   string    `json:"algorithm,omitempty"`
	StoreType                   string    `json:"storeType,omitempty"`
	CipherSuites                []string  `json:"cipherSuites,omitempty"`
}

// ClientEncryption defines encryption between Cassandra nodes and clients via CQL and JMX protocols for tools like reaper, cqlsh, nodetool and others.
type ClientEncryption struct {
	// ClientEncryption enables encryption between client and server via CQL and JXM protocols.
	Enabled bool `json:"enabled,omitempty"`
	// If enabled and optional is set to true both encrypted and unencrypted connections are handled.
	Optional bool `json:"optional,omitempty"`
	// TLS Secret fields configuration
	TLSSecret         ClientTLSSecret `json:"tlsSecret,omitempty"`
	RequireClientAuth *bool           `json:"requireClientAuth,omitempty"`
	Protocol          string          `json:"protocol,omitempty"`
	Algorithm         string          `json:"algorithm,omitempty"`
	StoreType         string          `json:"storeType,omitempty"`
	CipherSuites      []string        `json:"cipherSuites,omitempty"`
}

type TLSSecret struct {
	Name                  string `json:"name,omitempty"`
	KeystoreFileKey       string `json:"keystoreFileKey,omitempty"`
	KeystorePasswordKey   string `json:"keystorePasswordKey,omitempty"`
	TruststoreFileKey     string `json:"truststoreFileKey,omitempty"`
	TruststorePasswordKey string `json:"truststorePasswordKey,omitempty"`
}

type ClientTLSSecret struct {
	TLSSecret     `json:",inline"`
	CAFileKey     string `json:"caFileKey,omitempty"`
	TLSFileKey    string `json:"tlsFileKey,omitempty"`
	TLSCrtFileKey string `json:"tlsCrtFileKey,omitempty"`
}

type RepairSchedules struct {
	Enabled bool             `json:"enabled,omitempty"`
	Repairs []RepairSchedule `json:"repairs,omitempty"`
}

type RepairSchedule struct {
	Keyspace            string   `json:"keyspace" url:"keyspace"`
	Tables              []string `json:"tables,omitempty" url:"tables,comma,omitempty"`
	SegmentCountPerNode int32    `json:"segmentCountPerNode,omitempty" url:"segmentCountPerNode,omitempty"`
	// +kubebuilder:validation:Enum:=SEQUENTIAL;PARALLEL;DATACENTER_AWARE
	RepairParallelism   string   `json:"repairParallelism,omitempty" url:"repairParallelism,omitempty"`
	Intensity           string   `json:"intensity,omitempty" url:"intensity,omitempty"` // value between 0.0 and 1.0, but must never be 0.0.
	IncrementalRepair   bool     `json:"incrementalRepair,omitempty" url:"incrementalRepair,omitempty"`
	ScheduleDaysBetween int32    `json:"scheduleDaysBetween,omitempty" url:"scheduleDaysBetween"`
	ScheduleTriggerTime string   `json:"scheduleTriggerTime,omitempty" url:"scheduleTriggerTime,omitempty"`
	Nodes               []string `json:"nodes,omitempty" url:"nodes,comma,omitempty"`
	Datacenters         []string `json:"datacenters,omitempty" url:"datacenters,comma,omitempty"`
	BlacklistedTables   []string `json:"blacklistedTables,omitempty" url:"blacklistedTables,comma,omitempty"`
	// +kubebuilder:validation:Minimum:=1
	// +kubebuilder:validation:Maximum:=4
	RepairThreadCount int32 `json:"repairThreadCount,omitempty" url:"repairThreadCount,omitempty"`
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
	Replicas    *int32          `json:"replicas"`
	Affinity    *v1.Affinity    `json:"affinity,omitempty"`
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`
}

type Cassandra struct {
	Image string `json:"image,omitempty"`
	// +kubebuilder:validation:Enum:=Always;Never;IfNotPresent
	ImagePullPolicy v1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	Resources       v1.ResourceRequirements `json:"resources,omitempty"`
	// +kubebuilder:validation:Enum:=info;debug;trace
	LogLevel string `json:"logLevel,omitempty"`
	// +kubebuilder:validation:Minimum:=1
	NumSeeds int32 `json:"numSeeds,omitempty"`
	// +kubebuilder:validation:Minimum:=0
	TerminationGracePeriodSeconds *int64            `json:"terminationGracePeriodSeconds,omitempty"`
	PurgeGossip                   *bool             `json:"purgeGossip,omitempty"`
	Persistence                   Persistence       `json:"persistence,omitempty"`
	ZonesAsRacks                  bool              `json:"zonesAsRacks,omitempty"`
	JVMOptions                    []string          `json:"jvmOptions,omitempty"`
	Sysctls                       map[string]string `json:"sysctls,omitempty"`
	Monitoring                    Monitoring        `json:"monitoring,omitempty"`
	ConfigOverrides               string            `json:"configOverrides,omitempty"`
}

type Persistence struct {
	Enabled                  bool                         `json:"enabled,omitempty"`
	CommitLogVolume          bool                         `json:"commitLogVolume,omitempty"`
	Labels                   map[string]string            `json:"labels,omitempty"`
	Annotations              map[string]string            `json:"annotation,omitempty"`
	DataVolumeClaimSpec      v1.PersistentVolumeClaimSpec `json:"dataVolumeClaimSpec,omitempty"`
	CommitLogVolumeClaimSpec v1.PersistentVolumeClaimSpec `json:"commitLogVolumeClaimSpec,omitempty"`
}

type Prober struct {
	Image string `json:"image,omitempty"`
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	ImagePullPolicy v1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	Resources       v1.ResourceRequirements `json:"resources,omitempty"`
	// +kubebuilder:validation:Enum:=info;debug;trace
	LogLevel string `json:"logLevel,omitempty"`
	// +kubebuilder:validation:Enum:=console;json
	LogFormat      string         `json:"logFormat,omitempty"`
	Jolokia        Jolokia        `json:"jolokia,omitempty"`
	ServiceMonitor ServiceMonitor `json:"serviceMonitor,omitempty"`
}

type Jolokia struct {
	Image string `json:"image,omitempty"`
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	ImagePullPolicy v1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	Resources       v1.ResourceRequirements `json:"resources,omitempty"`
}

// PodName is the name of a Pod. Used to define CRD validation
// +kubebuilder:validation:MinLength:=1
// +kubebuilder:validation:MaxLength:=253
// +kubebuilder:validation:Pattern:=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
type PodName string

type Maintenance struct {
	// Maintenance object temporarily disables C* pods for debugging purposes.
	// +kubebuilder:validation:MinLength:=1
	// +kubebuilder:validation:MaxLength:=63
	// +kubebuilder:validation:Pattern:=^[a-z0-9][a-z0-9\-]*$
	DC   string    `json:"dc"`
	Pods []PodName `json:"pods,omitempty"`
}

// KeyspaceName is the name of a Cassandra keyspace
// +kubebuilder:validation:MinLength:=1
// +kubebuilder:validation:MaxLength:=48
// +kubebuilder:validation:Pattern:=^[a-zA-Z]\w+$
type KeyspaceName string

type SystemKeyspaces struct {
	Keyspaces []KeyspaceName     `json:"keyspaces,omitempty"`
	DCs       []SystemKeyspaceDC `json:"dcs,omitempty"`
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
	MaintenanceState []Maintenance `json:"maintenanceState,omitempty"`
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

type Ingress struct {
	Domain           string            `json:"domain,omitempty"`
	Secret           string            `json:"secret,omitempty"`
	Annotations      map[string]string `json:"annotations,omitempty"`
	IngressClassName *string           `json:"ingressClassName,omitempty"`
}

func init() {
	SchemeBuilder.Register(&CassandraCluster{}, &CassandraClusterList{})
}
