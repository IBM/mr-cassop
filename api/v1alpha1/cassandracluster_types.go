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

	CassandraClusterComponentProber    = "prober"
	CassandraClusterComponentKwatcher  = "kwatcher"
	CassandraClusterComponentCassandra = "cassandra"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CassandraClusterSpec defines the desired state of CassandraCluster
type CassandraClusterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ImagePullSecretName  string          `json:"imagePullSecretName"`
	CQLConfigMapLabelKey string          `json:"cqlConfigMapLabelKey,omitempty"`
	Cassandra            Cassandra       `json:"cassandra"`
	InternalAuth         bool            `json:"internalAuth"`
	HostPortEnabled      bool            `json:"hostPortEnabled"`
	SystemKeyspaces      SystemKeyspaces `json:"systemKeyspaces,omitempty"`
	DCs                  []DC            `json:"dcs"`
	Prober               Prober          `json:"prober,omitempty"`
	Kwatcher             Kwatcher        `json:"kwatcher"`
}

type Cassandra struct {
	NumSeeds        int32         `json:"numSeeds"`
	UsersDir        string        `json:"usersDir"`
	JMXPort         int32         `json:"jmxPort"`
	Auth            CassandraAuth `json:"auth"`
	Image           string        `json:"image"`
	ImagePullPolicy v1.PullPolicy `json:"imagePullPolicy"`
}

type CassandraAuth struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

type Prober struct {
	Image           string        `json:"image"`
	ImagePullPolicy v1.PullPolicy `json:"imagePullPolicy"`
	ServerPort      int32         `json:"serverPort"`
	Debug           bool          `json:"debug"`
	Jolokia         Jolokia       `json:"jolokia"`
}

type Kwatcher struct {
	Enabled         bool          `json:"enabled"`
	Image           string        `json:"image"`
	ImagePullPolicy v1.PullPolicy `json:"imagePullPolicy"`
}

type Jolokia struct {
	Image           string        `json:"image"`
	ImagePullPolicy v1.PullPolicy `json:"imagePullPolicy"`
}

type DC struct {
	Name     string `json:"name"`
	Replicas *int32 `json:"replicas"`
}

type SystemKeyspaces struct {
	Names []string           `json:"names"`
	DCs   []SystemKeyspaceDC `json:"dcs"`
}

type SystemKeyspaceDC struct {
	Name string `json:"name"`
	RF   int32  `json:"rf"`
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

	Spec   CassandraClusterSpec   `json:"spec,omitempty"`
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
