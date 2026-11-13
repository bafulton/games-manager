// Copyright 2026 games-manager contributors.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// GameVersionSpec is an immutable release of an administrator-approved Game.
// Publish another GameVersion to change its image or deployment recipe.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="GameVersion specs are immutable; publish a new version"
type GameVersionSpec struct {
	// Game is the catalog entry this release belongs to.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Game string `json:"game"`

	Template GameTemplateSpec `json:"template"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Game",type=string,JSONPath=`.spec.game`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.template.image`

// GameVersion is an immutable deployment recipe. GameServers pin it by name.
type GameVersion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              GameVersionSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// GameVersionList contains GameVersion resources.
type GameVersionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GameVersion `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GameVersion{}, &GameVersionList{})
}
