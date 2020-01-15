/*
 * Copyright (c) 2019-Present Pivotal Software, Inc. All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type Maplet struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ImageMapSpec defines the desired state of ImageMap
type ImageMapSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Map []Maplet `json:"map"`
}

// ImageMapStatus defines the observed state of ImageMap
type ImageMapStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ObservedGeneration records the ImageMap generation that the status reflects
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions are observations about the state of the ImageMap
	Conditions []Condition `json:"conditions"`
}

// Condition defines an observation about an ImageMap
// See: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
type Condition struct {
	// Type is the type of condition
	Type ConditionType `json:"type"`

	// Status of the condition, one of True, False, Unknown
	// +required
	Status corev1.ConditionStatus `json:"status"`

	// ObservationTime records when the condition was observed
	ObservationTime metav1.Time `json:"observationTime,omitempty"`

	// Message is a human readable description of the condition
	// +optional
	Message string `json:"message,omitempty"`
}

// ConditionType is a camel-cased condition type
type ConditionType string

const (
	// ConditionReady specifies that the resource has been processed, regardless of the outcome
	ConditionReady ConditionType = "Ready"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ImageMap is the Schema for the imagemaps API
type ImageMap struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImageMapSpec   `json:"spec,omitempty"`
	Status ImageMapStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ImageMapList contains a list of ImageMap
type ImageMapList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ImageMap `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ImageMap{}, &ImageMapList{})
}
