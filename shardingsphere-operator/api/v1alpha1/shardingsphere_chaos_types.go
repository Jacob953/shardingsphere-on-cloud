/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:object:root=true
// ShardingSphereChaosList contains a list of ShardingSphereChaos
type ShardingSphereChaosList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ShardingSphereChaos `json:"items"`
}

// +kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",name=Age,type=date
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// ShardingSphereChaos defines a chaos test case for the ShardingSphere Proxy cluster
type ShardingSphereChaos struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ShardingSphereChaosSpec `json:"spec,omitempty"`
	// +optional
	Status ShardingSphereChaosStatus `json:"status,omitempty"`
}

// ShardingSphereChaosSpec defines the desired state of ShardingSphereChaos
type ShardingSphereChaosSpec struct{}

// ShardingSphereChaosStatus defines the actual state of ShardingSphereChaos
type ShardingSphereChaosStatus struct{}
