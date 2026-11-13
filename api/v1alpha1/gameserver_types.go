package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GameServerPhase is a coarse summary of a server's state, for display.
type GameServerPhase string

const (
	PhasePending  GameServerPhase = "Pending"
	PhaseStarting GameServerPhase = "Starting"
	PhaseRunning  GameServerPhase = "Running"
	PhaseStopped  GameServerPhase = "Stopped"
	PhaseError    GameServerPhase = "Error"
)

const (
	// PurgeAnnotation, set to "true" before deleting a GameServer, deletes
	// its PVC too. Without it the world is kept.
	PurgeAnnotation = "games.homelab.io/purge"

	// ConditionReady is true when the server's pod is ready for players.
	ConditionReady = "Ready"
)

// GameServerSpec defines the desired state of GameServer.
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.version) || has(self.version)",message="version cannot be removed once set; name the version to change to"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.slot) || (has(self.slot) && self.slot == oldSelf.slot)",message="slot cannot change once assigned: it is the server's public address"
type GameServerSpec struct {
	// Game is the name of the Game this server runs.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="game is immutable"
	Game string `json:"game"`

	// Version names one of the Game's versions. The controller sets the
	// Game's defaultVersion when it's left empty, and then the server keeps
	// it: changing it upgrades or downgrades this server alone, on the same
	// world. Back the world up first; a downgrade may not read a world a
	// newer version has saved.
	// +kubebuilder:validation:MaxLength=63
	// +optional
	Version string `json:"version,omitempty"`

	// Owner is who the server belongs to, as their identity-provider
	// username. The UI lets only the owner change it.
	// +optional
	Owner string `json:"owner,omitempty"`

	// Running is whether the server should be up. false stops its pod and
	// frees its memory; the world stays on its volume.
	// +kubebuilder:default=true
	// +optional
	Running *bool `json:"running,omitempty"`

	// Size names one of the Game's sizes. Defaults to the Game's defaultSize.
	// +optional
	Size string `json:"size,omitempty"`

	// Env for this server, after the Game's env, so it can override it.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// RestartedAt restarts the server when it changes.
	// +optional
	RestartedAt *metav1.Time `json:"restartedAt,omitempty"`

	// Slot in the Game's port pool. Assigned by the controller for a
	// PortPool game, then fixed, so a restore keeps the server's address.
	// +kubebuilder:validation:Minimum=0
	// +optional
	Slot *int32 `json:"slot,omitempty"`
}

// GameServerStatus defines the observed state of GameServer.
type GameServerStatus struct {
	// +optional
	Phase GameServerPhase `json:"phase,omitempty"`

	// Address players connect to.
	// +optional
	Address string `json:"address,omitempty"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Game",type=string,JSONPath=`.spec.game`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="Owner",type=string,JSONPath=`.spec.owner`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Address",type=string,JSONPath=`.status.address`

// GameServer is one server of one Game.
type GameServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GameServerSpec   `json:"spec,omitempty"`
	Status GameServerStatus `json:"status,omitempty"`
}

// IsRunning reports whether the server should be up. Running defaults to
// true when unset.
func (s *GameServer) IsRunning() bool {
	return s.Spec.Running == nil || *s.Spec.Running
}

// +kubebuilder:object:root=true

// GameServerList contains a list of GameServer.
type GameServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GameServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GameServer{}, &GameServerList{})
}
