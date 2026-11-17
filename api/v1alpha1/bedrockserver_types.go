/*
Copyright 2026 DataKnifeAI.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	PhasePending   = "Pending"
	PhaseDeploying = "Deploying"
	PhaseRunning   = "Running"
	PhaseFailed    = "Failed"

	ConditionReady             = "Ready"
	ConditionEULAAccepted      = "EULAAccepted"
	ConditionGatewayConfigured = "GatewayConfigured"
	ConditionUpdateAvailable   = "UpdateAvailable"

	DefaultImage           = "itzg/minecraft-bedrock-server:latest"
	DefaultGamePort        = int32(19132)
	DefaultGamePortV6      = int32(19133)
	DefaultMaxPlayers      = int32(10)
	DefaultPersistenceSize = "8Gi"
	DefaultGraceSeconds    = int64(60)
	DefaultUpdateInterval  = "6h"

	VersionModeLatest   = "LATEST"
	VersionModePreview  = "PREVIEW"
	VersionModePrevious = "PREVIOUS"
	VersionModePinned   = "PINNED"

	ContainerName = "bedrock"
)

// EULAConfig is the Mojang EULA acceptance gate. The itzg image exits unless
// EULA=TRUE; the operator mirrors that hard requirement before creating children.
type EULAConfig struct {
	// Accepted must be true or reconcile fails and creates no child resources.
	// +kubebuilder:validation:Required
	Accepted bool `json:"accepted"`
}

// ServerImageConfig selects the community runtime-download image.
// Binaries are never baked into an operator-published image.
type ServerImageConfig struct {
	// Image is the container image. Default: itzg/minecraft-bedrock-server:latest
	// +kubebuilder:default="itzg/minecraft-bedrock-server:latest"
	// +optional
	Image string `json:"image,omitempty"`

	// PullPolicy for the game server container.
	// +kubebuilder:default=IfNotPresent
	// +optional
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
}

// VersionConfig controls which BDS build the itzg entrypoint downloads.
type VersionConfig struct {
	// Mode selects VERSION env semantics: LATEST, PREVIEW, PREVIOUS, or PINNED.
	// +kubebuilder:default=LATEST
	// +kubebuilder:validation:Enum=LATEST;PREVIEW;PREVIOUS;PINNED
	// +optional
	Mode string `json:"mode,omitempty"`

	// Pinned is the exact BDS version when mode=PINNED (e.g. "1.21.93.05").
	// +optional
	Pinned string `json:"pinned,omitempty"`

	// DirectDownloadUrl bypasses Mojang version lookup (air-gapped / pinned zip).
	// Maps to DIRECT_DOWNLOAD_URL on the itzg image.
	// +optional
	DirectDownloadUrl string `json:"directDownloadUrl,omitempty"`
}

// ServerSettings is a typed allow-list of server.properties keys mapped to env vars.
type ServerSettings struct {
	// LevelName maps to LEVEL_NAME.
	// +optional
	LevelName string `json:"levelName,omitempty"`

	// LevelSeed maps to LEVEL_SEED.
	// +optional
	LevelSeed string `json:"levelSeed,omitempty"`

	// LevelType maps to LEVEL_TYPE: DEFAULT | FLAT | LEGACY.
	// +kubebuilder:default=DEFAULT
	// +kubebuilder:validation:Enum=DEFAULT;FLAT;LEGACY
	// +optional
	LevelType string `json:"levelType,omitempty"`

	// GameMode maps to GAMEMODE: survival | creative | adventure.
	// +kubebuilder:default=survival
	// +kubebuilder:validation:Enum=survival;creative;adventure
	// +optional
	GameMode string `json:"gameMode,omitempty"`

	// Difficulty maps to DIFFICULTY: peaceful | easy | normal | hard.
	// +kubebuilder:default=normal
	// +kubebuilder:validation:Enum=peaceful;easy;normal;hard
	// +optional
	Difficulty string `json:"difficulty,omitempty"`

	// MaxPlayers maps to MAX_PLAYERS.
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=30
	// +optional
	MaxPlayers int32 `json:"maxPlayers,omitempty"`

	// AllowCheats maps to ALLOW_CHEATS.
	// +optional
	AllowCheats bool `json:"allowCheats,omitempty"`

	// DefaultPlayerPermissionLevel maps to DEFAULT_PLAYER_PERMISSION_LEVEL.
	// +kubebuilder:default=member
	// +kubebuilder:validation:Enum=visitor;member;operator
	// +optional
	DefaultPlayerPermissionLevel string `json:"defaultPlayerPermissionLevel,omitempty"`

	// WhiteList maps to WHITE_LIST.
	// +optional
	WhiteList bool `json:"whiteList,omitempty"`

	// IPv6Enabled controls SERVER_PORT_V6 and a second Gateway UDP listener (19133).
	// +optional
	IPv6Enabled bool `json:"ipv6Enabled,omitempty"`

	// ServerPort overrides SERVER_PORT (default 19132).
	// +kubebuilder:validation:Minimum=1024
	// +kubebuilder:validation:Maximum=65535
	// +optional
	ServerPort int32 `json:"serverPort,omitempty"`

	// ServerPortV6 overrides SERVER_PORT_V6 (default 19133). Used when ipv6Enabled.
	// +kubebuilder:validation:Minimum=1024
	// +kubebuilder:validation:Maximum=65535
	// +optional
	ServerPortV6 int32 `json:"serverPortV6,omitempty"`
}

// SecretsConfig controls optional operator/whitelist Secret material.
type SecretsConfig struct {
	// GenerateSecrets creates an empty-owned Secret shell for operators/whitelist
	// when true. Bring-your-own refs take precedence when set.
	// +optional
	GenerateSecrets bool `json:"generateSecrets,omitempty"`

	// OperatorList refs a Secret with key "operators.json" mounted into /data permissions.
	// +optional
	OperatorList *corev1.LocalObjectReference `json:"operatorList,omitempty"`

	// AllowList refs a Secret with key "allowlist.json" (Bedrock whitelist file).
	// +optional
	AllowList *corev1.LocalObjectReference `json:"allowList,omitempty"`
}

// PersistenceConfig configures the /data PVC (worlds, package, properties, packs).
type PersistenceConfig struct {
	// Size is the PVC capacity. Default 8Gi.
	// +kubebuilder:default="8Gi"
	// +optional
	Size string `json:"size,omitempty"`

	// StorageClassName selects the StorageClass. Empty uses the cluster default.
	// +optional
	StorageClassName string `json:"storageClassName,omitempty"`

	// ExistingClaim adopts an existing PVC by name (DR / restore).
	// +optional
	ExistingClaim string `json:"existingClaim,omitempty"`
}

// UpdateConfig controls the Mojang version-feed watcher.
type UpdateConfig struct {
	// CheckInterval is how often the reconciler polls the Mojang download links API.
	// +kubebuilder:default="6h"
	// +optional
	CheckInterval string `json:"checkInterval,omitempty"`

	// NotifyOnly when true (default) only flips status.updateAvailable.
	// When false, the reconciler may patch version fields so the next pod
	// restart re-downloads (runtime-download analogue of autoUpdateImage).
	// +kubebuilder:default=true
	// +optional
	NotifyOnly *bool `json:"notifyOnly,omitempty"`
}

// GatewayConfig configures Envoy Gateway exposure (or Service fallback).
type GatewayConfig struct {
	// Enabled creates Gateway / UDPRoute / TCPRoute resources. Default true.
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Address is the dedicated external IP (Kube-VIP / MetalLB).
	// Required when enabled is true; also used as LoadBalancerIP when
	// enabled is false and serviceType is LoadBalancer.
	// +optional
	// +kubebuilder:validation:Pattern=`^$|^([0-9]{1,3}\.){3}[0-9]{1,3}$`
	Address string `json:"address,omitempty"`

	// ClassName is the GatewayClass name.
	// +kubebuilder:default="envoy"
	// +optional
	ClassName string `json:"className,omitempty"`

	// GatewayName overrides the Gateway resource name.
	// +optional
	GatewayName string `json:"gatewayName,omitempty"`

	// EnvoyProxyName overrides the EnvoyProxy resource name.
	// +optional
	EnvoyProxyName string `json:"envoyProxyName,omitempty"`

	// ServiceType overrides the backing Service type when gateway is disabled.
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
	// +kubebuilder:default=ClusterIP
	// +optional
	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`
}

// ModsConfig is a Phase 2 stub for pack/resource mounting.
type ModsConfig struct {
	// Enabled mounts packs into /data (Phase 2).
	// +optional
	Enabled bool `json:"enabled,omitempty"`
}

// AdminUIConfig is a Phase 3 stub for Server Manager.
type AdminUIConfig struct {
	// Enabled starts the Server Manager UI (Phase 3).
	// +optional
	Enabled bool `json:"enabled,omitempty"`
}

// BedrockServerSpec defines the desired state of a Minecraft Bedrock dedicated server.
type BedrockServerSpec struct {
	// EULA must be accepted (accepted: true) or reconcile fails hard.
	// +kubebuilder:validation:Required
	EULA EULAConfig `json:"eula"`

	// ServerImage selects the runtime-download community image.
	// +optional
	ServerImage ServerImageConfig `json:"serverImage,omitempty"`

	// Version controls BDS download selection (VERSION / DIRECT_DOWNLOAD_URL).
	// +optional
	Version VersionConfig `json:"version,omitempty"`

	// Settings is a typed allow-list mapped to itzg env vars / server.properties.
	// +optional
	Settings ServerSettings `json:"settings,omitempty"`

	// Secrets configures operator/whitelist Secret material.
	// +optional
	Secrets SecretsConfig `json:"secrets,omitempty"`

	// Persistence configures the /data PVC.
	// +optional
	Persistence PersistenceConfig `json:"persistence,omitempty"`

	// Update configures the Mojang version-feed watcher.
	// +optional
	Update UpdateConfig `json:"update,omitempty"`

	// Gateway configures Envoy Gateway exposure (UDP + NetherNet TCP).
	// +optional
	Gateway GatewayConfig `json:"gateway,omitempty"`

	// Mods is Phase 2 — pack/resource mounting (stub).
	// +optional
	Mods ModsConfig `json:"mods,omitempty"`

	// AdminUI is Phase 3 — Server Manager UI (stub).
	// +optional
	AdminUI AdminUIConfig `json:"adminUI,omitempty"`

	// HostNetworkDiscovery opt-in sets hostNetwork for LAN discovery.
	// Document trade-offs: UDP broadcasts are not forwarded through pod networking.
	// +optional
	HostNetworkDiscovery bool `json:"hostNetworkDiscovery,omitempty"`

	// ImagePullSecrets for private registries.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Resources overrides default CPU/memory.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// TerminationGracePeriodSeconds for clean world flush on stop.
	// +kubebuilder:default=60
	// +kubebuilder:validation:Minimum=30
	// +optional
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty"`

	// NodeSelector pins the game server pod to specific nodes.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// BedrockServerStatus defines the observed state of BedrockServer.
type BedrockServerStatus struct {
	// Phase is Pending, Deploying, Running, or Failed.
	// +optional
	Phase string `json:"phase,omitempty"`

	// Ready is true when the StatefulSet has a ready replica.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// AvailableVersion is the latest BDS version from the Mojang feed.
	// +optional
	AvailableVersion string `json:"availableVersion,omitempty"`

	// InstalledVersion is the version observed from the running server when known.
	// +optional
	InstalledVersion string `json:"installedVersion,omitempty"`

	// UpdateAvailable is true when availableVersion differs from installedVersion.
	// +optional
	UpdateAvailable bool `json:"updateAvailable,omitempty"`

	// ConnectionAddress is the IP clients should use.
	// +optional
	ConnectionAddress string `json:"connectionAddress,omitempty"`

	// ConnectionPort is the UDP game port (default 19132).
	// +optional
	ConnectionPort int32 `json:"connectionPort,omitempty"`

	// Message is a human-readable status detail.
	// +optional
	Message string `json:"message,omitempty"`

	// ObservedGeneration is the last reconciled generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastUpdateCheck is when the Mojang feed was last polled.
	// +optional
	LastUpdateCheck *metav1.Time `json:"lastUpdateCheck,omitempty"`

	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=bds;bedrock
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Address",type=string,JSONPath=`.status.connectionAddress`
// +kubebuilder:printcolumn:name="Port",type=integer,JSONPath=`.status.connectionPort`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.installedVersion`
// +kubebuilder:printcolumn:name="Update",type=boolean,JSONPath=`.status.updateAvailable`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// BedrockServer is the Schema for the bedrockservers API.
type BedrockServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BedrockServerSpec   `json:"spec,omitempty"`
	Status BedrockServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BedrockServerList contains a list of BedrockServer.
type BedrockServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BedrockServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BedrockServer{}, &BedrockServerList{})
}
