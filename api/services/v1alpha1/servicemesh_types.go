/*
Copyright 2023.

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
	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	infrav1 "github.com/opendatahub-io/opendatahub-operator/v2/api/infrastructure/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ServiceMeshServiceName = "servicemesh"
	// ServiceMeshInstanceName the name of the ServiceMesh instance singleton.
	ServiceMeshInstanceName = "default-servicemesh"
	ServiceMeshKind         = "ServiceMesh"
)

// Check that the component implements common.PlatformObject.
var _ common.PlatformObject = (*ServiceMesh)(nil)

// ServiceMeshSpec defines the desired state of ServiceMesh
type ServiceMeshSpec struct {
	// ServiceMesh configuration from infrastructure API
	infrav1.ServiceMeshSpec `json:",inline"`
}

// ServiceMeshStatus defines the observed state of ServiceMesh
type ServiceMeshStatus struct {
	common.Status `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Management State",type="string",JSONPath=".spec.managementState"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].reason"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ServiceMesh is the Schema for the servicemesh API
type ServiceMesh struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceMeshSpec   `json:"spec,omitempty"`
	Status ServiceMeshStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceMeshList contains a list of ServiceMesh
type ServiceMeshList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceMesh `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServiceMesh{}, &ServiceMeshList{})
}

// GetManagementState returns the management state of the ServiceMesh
func (s *ServiceMesh) GetManagementState() string {
	return string(s.Spec.ManagementState)
}

// GetName returns the name of the ServiceMesh
func (s *ServiceMesh) GetName() string {
	return s.Name
}

// GetNamespace returns the namespace of the ServiceMesh
func (s *ServiceMesh) GetNamespace() string {
	return s.Namespace
}

// GetConditions returns the conditions of the ServiceMesh
func (s *ServiceMesh) GetConditions() []common.Condition {
	return s.Status.GetConditions()
}

// SetConditions sets the conditions of the ServiceMesh
func (s *ServiceMesh) SetConditions(conditions []common.Condition) {
	s.Status.SetConditions(conditions)
}

// GetStatus returns the status of the ServiceMesh
func (s *ServiceMesh) GetStatus() *common.Status {
	return &s.Status.Status
}
