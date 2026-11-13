package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConnectionType is how players reach a server of a Game.
// +kubebuilder:validation:Enum=Hostname;PortPool
type ConnectionType string

const (
	// ConnectionHostname gives each server its own hostname under a shared
	// domain, routed by a proxy that reads the hostname from the game's
	// protocol (mc-router for Minecraft Java). All servers share one port.
	ConnectionHostname ConnectionType = "Hostname"

	// ConnectionPortPool gives each server a numbered slot. The slot picks
	// the server's public port and the name of its Tailscale Service, which
	// the edge forwarder has a fixed route to.
	ConnectionPortPool ConnectionType = "PortPool"
)

// GamePort is a port the game server listens on.
type GamePort struct {
	// Name of the port, unique within the Game.
	// +kubebuilder:validation:MaxLength=15
	// +kubebuilder:validation:Pattern=`^[a-z]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`

	// ContainerPort is the port the game listens on inside the container.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	ContainerPort int32 `json:"containerPort"`

	// Protocol is TCP or UDP.
	// +kubebuilder:validation:Enum=TCP;UDP
	// +kubebuilder:default=TCP
	// +optional
	Protocol corev1.Protocol `json:"protocol,omitempty"`
}

// HostnameConnection configures ConnectionHostname.
type HostnameConnection struct {
	// Domain each server's hostname is created under: server "alice" of a
	// Game with domain "mc.example.com" is "alice.mc.example.com". The
	// namespace isn't part of it, so a second "alice" in another namespace
	// reports HostnameConflict until the first is deleted.
	// +kubebuilder:validation:MinLength=1
	Domain string `json:"domain"`
}

// PortPoolConnection configures ConnectionPortPool.
// +kubebuilder:validation:XValidation:rule="self.basePort + self.slots * self.stride - 1 <= 65535",message="port pool exceeds port 65535"
type PortPoolConnection struct {
	// Slots is how many servers of this Game can exist at once. The edge
	// forwarder must have a route for every slot.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	Slots int32 `json:"slots"`

	// PublicHost is the hostname players connect to, eg
	// "valheim.example.com".
	// +kubebuilder:validation:MinLength=1
	PublicHost string `json:"publicHost"`

	// BasePort is slot 0's public port. Slot n's is BasePort + n*Stride.
	// The edge forwards each slot's public ports to the game's own ports.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	BasePort int32 `json:"basePort"`

	// Stride is how many public ports each slot takes, for games that use
	// consecutive ports (Valheim uses 2: game and query).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default=1
	// +optional
	Stride int32 `json:"stride,omitempty"`

	// TailscaleTags for each slot's Tailscale Service. The edge forwarder's
	// grant targets these.
	// +kubebuilder:default="tag:k8s-games"
	// +optional
	TailscaleTags string `json:"tailscaleTags,omitempty"`
}

// Connection is how players reach a server of a Game.
// +kubebuilder:validation:XValidation:rule="self.type != 'Hostname' || has(self.hostname)",message="type Hostname needs hostname"
// +kubebuilder:validation:XValidation:rule="self.type != 'PortPool' || has(self.portPool)",message="type PortPool needs portPool"
type Connection struct {
	Type ConnectionType `json:"type"`

	// +optional
	Hostname *HostnameConnection `json:"hostname,omitempty"`

	// +optional
	PortPool *PortPoolConnection `json:"portPool,omitempty"`
}

// SizePreset is a named set of resources a server can choose.
type SizePreset struct {
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`

	// Resources for the game container. Set memory requests and limits but
	// no CPU limit: a game that throttles lags.
	Resources corev1.ResourceRequirements `json:"resources"`
}

// GameStorage is the volume each server's world lives on.
type GameStorage struct {
	// Size of each server's volume.
	Size resource.Quantity `json:"size"`

	// MountPath of the volume in the game container.
	// +kubebuilder:validation:MinLength=1
	MountPath string `json:"mountPath"`

	// StorageClassName for each server's PVC. Omit to use the cluster default;
	// an explicit empty string disables dynamic provisioning.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`

	// Labels and Annotations customize new PVCs, for example to opt into a
	// storage provider's backup jobs. Existing world volumes are not changed.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// GameSpec defines how to run servers of one game.
// +kubebuilder:validation:XValidation:rule="self.sizes.exists(s, s.name == self.defaultSize)",message="defaultSize must name one of sizes"
// +kubebuilder:validation:XValidation:rule="!has(self.arch) || size(self.arch) == 0 || !has(self.nodeSelector) || !('kubernetes.io/arch' in self.nodeSelector)",message="set the architecture with arch, not nodeSelector"
type GameSpec struct {
	// Image of the game server, pinned to a tag or digest.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// +optional
	Command []string `json:"command,omitempty"`

	// +optional
	Args []string `json:"args,omitempty"`

	// Env for every server of this game. A GameServer's own env comes after
	// it, so a server can override these.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Ports the game listens on.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=20
	// +listType=map
	// +listMapKey=name
	Ports []GamePort `json:"ports"`

	Connection Connection `json:"connection"`

	// Sizes a server can choose from.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	// +listType=map
	// +listMapKey=name
	Sizes []SizePreset `json:"sizes"`

	// DefaultSize is the size a server gets when it names none.
	// +kubebuilder:validation:MaxLength=63
	DefaultSize string `json:"defaultSize"`

	Storage GameStorage `json:"storage"`

	// Arch the image is built for, as a kubernetes.io/arch node selector.
	// Empty for a multi-arch image.
	// +kubebuilder:validation:Enum="";amd64;arm64
	// +optional
	Arch string `json:"arch,omitempty"`

	// NodeSelector for every server's pod, for a hard placement constraint.
	// arch is added to it.
	// +kubebuilder:validation:MaxProperties=20
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Affinity for every server's pod. A preferred node affinity steers
	// servers toward some nodes without ruling the others out.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Tolerations for every server's pod.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// ReadinessProbe for the game container. A server reports Ready from
	// it, so it should mean "players can join".
	// +optional
	ReadinessProbe *corev1.Probe `json:"readinessProbe,omitempty"`

	// Lifecycle for the game container. Put the save-and-stop command in
	// preStop.
	// +optional
	Lifecycle *corev1.Lifecycle `json:"lifecycle,omitempty"`

	// TerminationGracePeriodSeconds must cover the preStop save.
	// +kubebuilder:default=120
	// +optional
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty"`

	// PodSecurityContext for every server's pod. Defaults to non-root
	// 1000:1000 with fsGroup 1000.
	// +optional
	PodSecurityContext *corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`

	// SecurityContext for the game container. Defaults to no privilege
	// escalation and no capabilities. Images that start as root and drop
	// privileges themselves need some back.
	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`
}

// GameStatus defines the observed state of Game.
type GameStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Connection",type=string,JSONPath=`.spec.connection.type`

// Game describes how to run servers of one game.
type Game struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GameSpec   `json:"spec,omitempty"`
	Status GameStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GameList contains a list of Game.
type GameList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Game `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Game{}, &GameList{})
}
