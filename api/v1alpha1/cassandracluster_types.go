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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CassandraClusterSpec defines the desired state of CassandraCluster
type CassandraClusterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	CQLConfigMapLabelKey string          `json:"cqlConfigMapLabelKey,omitempty"`
	ProberHost           string          `json:"proberHost,omitempty"`
	CassandraUser        string          `json:"cassandraUser"`
	CassandraPassword    string          `json:"cassandraPassword"`
	InternalAuth         bool            `json:"internalAuth"`
	KwatcherEnabled      bool            `json:"kwatcherEnabled"`
	SystemKeyspaces      SystemKeyspaces `json:"systemKeyspaces,omitempty"`
	DCs                  []DC            `json:"dcs"`
}

type DC struct {
	Name     string `json:"name"`
	Replicas int32  `json:"replicas"`
}

type SystemKeyspaces struct {
	DCs []SystemKeyspaceDC `json:"dcs"`
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
