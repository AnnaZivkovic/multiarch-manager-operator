/*
Copyright 2023 Red Hat, Inc.

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

	"github.com/openshift/multiarch-tuning-operator/apis/multiarch/common"
)

// ClusterPodPlacementConfigSpec defines the desired state of ClusterPodPlacementConfig
type ClusterPodPlacementConfigSpec struct {
	// LogVerbosity is the log level for the pod placement components.
	// Valid values are: "Normal", "Debug", "Trace", "TraceAll".
	// Defaults to "Normal".
	// +optional
	// +kubebuilder:default=Normal
	LogVerbosity common.LogVerbosityLevel `json:"logVerbosity,omitempty"`

	// NamespaceSelector selects the namespaces in which the pod placement operand must process the nodeAffinity
	// of the pods. All namespaces are considered by default.
	//
	//
	// Example:
	// {"namespaceSelector":{"matchExpressions":[{"key":"multiarch.openshift.io/exclude-pod-placement","operator":"DoesNotExist"}]}}
	//
	// the "operator" field value is set to "DoesNotExist". Therefore, if the key field value
	// "multiarch.openshift.io/exclude-pod-placement" is set as a label in a namespace, the operand does not process
	// the nodeAffinity of the pods in that namespace. Instead, the operand processes the nodeAffinity of the pods in
	// namespaces that do not contain the label.
	//
	// Users that want the operand to process the nodeAffinity of the pods only in specific namespaces, can configure
	// the namespaceSelector as follows:
	//
	// {"namespaceSelector":{"matchExpressions":[{"key":"multiarch.openshift.io/include-pod-placement","operator":"Exists"}]}}
	//
	// The operator field value is set to "Exists". Therefore, the operand processes the nodeAffinity of the pods only
	// in namespaces that contain the multiarch.openshift.io/include-pod-placement label.
	//
	// See
	// https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
	// for more examples of label selectors.
	//
	// Default to the empty LabelSelector, which matches everything. Selectors are ANDed.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
}

// ClusterPodPlacementConfigStatus defines the observed state of ClusterPodPlacementConfig
type ClusterPodPlacementConfigStatus struct {
	// Conditions represents the latest available observations of a ClusterPodPlacementConfig's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ClusterPodPlacementConfig defines the configuration for the PodPlacement operand.
// It is a singleton resource. Users can deploy one only object named "cluster".
// Creating this object will trigger the deployment of the architecture aware pod placement operand.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=clusterpodplacementconfigs,scope=Cluster
type ClusterPodPlacementConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterPodPlacementConfigSpec   `json:"spec,omitempty"`
	Status ClusterPodPlacementConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ClusterPodPlacementConfigList contains a list of ClusterPodPlacementConfig
type ClusterPodPlacementConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterPodPlacementConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterPodPlacementConfig{}, &ClusterPodPlacementConfigList{})
}
