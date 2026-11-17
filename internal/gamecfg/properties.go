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

// Package gamecfg maps BedrockServer typed settings to itzg image env vars.
package gamecfg

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	bedrockv1alpha1 "github.com/DataKnifeAI/bedrock-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// EnvFromSpec builds the allow-listed env vars for the itzg bedrock image.
// Unknown keys are never admitted — only fields on ServerSettings are mapped.
func EnvFromSpec(spec bedrockv1alpha1.BedrockServerSpec, fallbackLevelName string) ([]corev1.EnvVar, error) {
	if !spec.EULA.Accepted {
		return nil, fmt.Errorf("spec.eula.accepted must be true (Minecraft EULA)")
	}

	s := spec.Settings
	levelName := strings.TrimSpace(s.LevelName)
	if levelName == "" {
		levelName = fallbackLevelName
	}
	if levelName == "" {
		levelName = "Bedrock level"
	}

	levelType := s.LevelType
	if levelType == "" {
		levelType = "DEFAULT"
	}
	gameMode := s.GameMode
	if gameMode == "" {
		gameMode = "survival"
	}
	difficulty := s.Difficulty
	if difficulty == "" {
		difficulty = "normal"
	}
	perm := s.DefaultPlayerPermissionLevel
	if perm == "" {
		perm = "member"
	}
	maxPlayers := s.MaxPlayers
	if maxPlayers == 0 {
		maxPlayers = bedrockv1alpha1.DefaultMaxPlayers
	}
	transport := Transport(spec)
	port := ServerPort(spec)
	portV6 := ServerPortV6(spec)

	env := []corev1.EnvVar{
		{Name: "EULA", Value: "TRUE"},
		{Name: "LEVEL_NAME", Value: levelName},
		{Name: "LEVEL_TYPE", Value: levelType},
		{Name: "GAMEMODE", Value: gameMode},
		{Name: "DIFFICULTY", Value: difficulty},
		{Name: "MAX_PLAYERS", Value: strconv.FormatInt(int64(maxPlayers), 10)},
		{Name: "ALLOW_CHEATS", Value: boolStr(s.AllowCheats)},
		{Name: "DEFAULT_PLAYER_PERMISSION_LEVEL", Value: perm},
		// Bedrock 1.26+ uses allow-list / ALLOW_LIST; WHITE_LIST remains for older images.
		{Name: "ALLOW_LIST", Value: boolStr(s.WhiteList)},
		{Name: "WHITE_LIST", Value: boolStr(s.WhiteList)},
		{Name: "ONLINE_MODE", Value: boolStr(onlineMode(s))},
		// TRANSPORT: nethernet required on BDS ≥1.26.60 for player joins.
		{Name: "TRANSPORT", Value: transport},
		{Name: "SERVER_PORT", Value: strconv.FormatInt(int64(port), 10)},
		{Name: "SERVER_PORT_V6", Value: strconv.FormatInt(int64(portV6), 10)},
	}
	if seed := strings.TrimSpace(s.LevelSeed); seed != "" {
		env = append(env, corev1.EnvVar{Name: "LEVEL_SEED", Value: seed})
	}
	if transport == bedrockv1alpha1.TransportNetherNet {
		if udp := ServerUdpPorts(spec); udp != "" {
			env = append(env, corev1.EnvVar{Name: "SERVER_UDP_PORTS", Value: udp})
		}
	}

	versionEnv, err := VersionEnv(spec.Version)
	if err != nil {
		return nil, err
	}
	env = append(env, versionEnv...)

	return env, nil
}

// VersionEnv maps VersionConfig to VERSION / DIRECT_DOWNLOAD_URL.
func VersionEnv(v bedrockv1alpha1.VersionConfig) ([]corev1.EnvVar, error) {
	mode := strings.ToUpper(strings.TrimSpace(v.Mode))
	if mode == "" {
		mode = bedrockv1alpha1.VersionModeLatest
	}

	var env []corev1.EnvVar
	if url := strings.TrimSpace(v.DirectDownloadUrl); url != "" {
		env = append(env, corev1.EnvVar{Name: "DIRECT_DOWNLOAD_URL", Value: url})
	}

	switch mode {
	case bedrockv1alpha1.VersionModeLatest, bedrockv1alpha1.VersionModePreview, bedrockv1alpha1.VersionModePrevious:
		env = append(env, corev1.EnvVar{Name: "VERSION", Value: mode})
	case bedrockv1alpha1.VersionModePinned:
		pinned := strings.TrimSpace(v.Pinned)
		if pinned == "" && strings.TrimSpace(v.DirectDownloadUrl) == "" {
			return nil, fmt.Errorf("spec.version.pinned is required when mode=PINNED (or set directDownloadUrl)")
		}
		if pinned != "" {
			env = append(env, corev1.EnvVar{Name: "VERSION", Value: pinned})
		}
	default:
		return nil, fmt.Errorf("spec.version.mode %q is not supported", mode)
	}
	return env, nil
}

// Transport returns the effective TRANSPORT value (default nethernet).
func Transport(spec bedrockv1alpha1.BedrockServerSpec) string {
	t := strings.ToLower(strings.TrimSpace(spec.Settings.Transport))
	if t == "" {
		return bedrockv1alpha1.DefaultTransport
	}
	return t
}

// MaxPlayers returns the effective player cap.
func MaxPlayers(spec bedrockv1alpha1.BedrockServerSpec) int32 {
	if spec.Settings.MaxPlayers > 0 {
		return spec.Settings.MaxPlayers
	}
	return bedrockv1alpha1.DefaultMaxPlayers
}

// UdpGameplayPortStart returns the first NetherNet gameplay UDP port.
func UdpGameplayPortStart(spec bedrockv1alpha1.BedrockServerSpec) int32 {
	return bedrockv1alpha1.DefaultUdpPortStart
}

// UdpGameplayPortEnd returns the last NetherNet gameplay UDP port (inclusive).
func UdpGameplayPortEnd(spec bedrockv1alpha1.BedrockServerSpec) int32 {
	n := MaxPlayers(spec)
	if n < 1 {
		n = 1
	}
	return UdpGameplayPortStart(spec) + n - 1
}

// ServerUdpPorts returns SERVER_UDP_PORTS for NetherNet.
// Prefer explicit spec.settings.serverUdpPorts; otherwise derive
// "<publicIP>:<start>-<end>:<start>-<end>" from gateway.address so clients
// behind kube-vip reach the VIP (itzg NAT form).
func ServerUdpPorts(spec bedrockv1alpha1.BedrockServerSpec) string {
	if v := strings.TrimSpace(spec.Settings.ServerUdpPorts); v != "" {
		return v
	}
	start := UdpGameplayPortStart(spec)
	end := UdpGameplayPortEnd(spec)
	rangeStr := fmt.Sprintf("%d-%d", start, end)
	publicIP := strings.TrimSpace(spec.Gateway.Address)
	if publicIP == "" {
		return rangeStr
	}
	// publicIP:publicRange:localRange — advertise VIP to clients.
	return fmt.Sprintf("%s:%s:%s", publicIP, rangeStr, rangeStr)
}

// ServerPort returns the effective IPv4 game port.
func ServerPort(spec bedrockv1alpha1.BedrockServerSpec) int32 {
	if spec.Settings.ServerPort > 0 {
		return spec.Settings.ServerPort
	}
	return bedrockv1alpha1.DefaultGamePort
}

// ServerPortV6 returns the effective IPv6 game port.
func ServerPortV6(spec bedrockv1alpha1.BedrockServerSpec) int32 {
	if spec.Settings.ServerPortV6 > 0 {
		return spec.Settings.ServerPortV6
	}
	return bedrockv1alpha1.DefaultGamePortV6
}

// ImageRef returns the container image reference.
func ImageRef(spec bedrockv1alpha1.BedrockServerSpec) string {
	if strings.TrimSpace(spec.ServerImage.Image) != "" {
		return spec.ServerImage.Image
	}
	return bedrockv1alpha1.DefaultImage
}

// PersistenceSize returns the PVC size.
func PersistenceSize(spec bedrockv1alpha1.BedrockServerSpec) string {
	if strings.TrimSpace(spec.Persistence.Size) != "" {
		return spec.Persistence.Size
	}
	return bedrockv1alpha1.DefaultPersistenceSize
}

// GraceSeconds returns terminationGracePeriodSeconds.
func GraceSeconds(spec bedrockv1alpha1.BedrockServerSpec) int64 {
	if spec.TerminationGracePeriodSeconds != nil {
		return *spec.TerminationGracePeriodSeconds
	}
	return bedrockv1alpha1.DefaultGraceSeconds
}

// NotifyOnly reports whether update watcher should only flip status.
func NotifyOnly(spec bedrockv1alpha1.BedrockServerSpec) bool {
	if spec.Update.NotifyOnly == nil {
		return true
	}
	return *spec.Update.NotifyOnly
}

// GatewayEnabled reports whether Envoy Gateway resources should be reconciled.
func GatewayEnabled(spec bedrockv1alpha1.BedrockServerSpec) bool {
	if spec.Gateway.Enabled == nil {
		return true
	}
	return *spec.Gateway.Enabled
}

// ValidateSpec returns a user-facing error if the CR cannot be reconciled.
func ValidateSpec(spec bedrockv1alpha1.BedrockServerSpec) error {
	if !spec.EULA.Accepted {
		return fmt.Errorf("spec.eula.accepted must be true — Mojang requires EULA acceptance; this operator does not redistribute Bedrock binaries")
	}
	if _, err := VersionEnv(spec.Version); err != nil {
		return err
	}
	if GatewayEnabled(spec) && strings.TrimSpace(spec.Gateway.Address) == "" {
		return fmt.Errorf("spec.gateway.address is required when gateway is enabled")
	}
	if !GatewayEnabled(spec) && spec.Gateway.ServiceType == corev1.ServiceTypeLoadBalancer && strings.TrimSpace(spec.Gateway.Address) == "" {
		return fmt.Errorf("spec.gateway.address is required when serviceType is LoadBalancer")
	}
	if interval := strings.TrimSpace(spec.Update.CheckInterval); interval != "" {
		if _, err := time.ParseDuration(interval); err != nil {
			return fmt.Errorf("spec.update.checkInterval: %w", err)
		}
	}
	if t := strings.ToLower(strings.TrimSpace(spec.Settings.Transport)); t != "" &&
		t != bedrockv1alpha1.TransportRakNet && t != bedrockv1alpha1.TransportNetherNet {
		return fmt.Errorf("spec.settings.transport %q is not supported (raknet|nethernet)", spec.Settings.Transport)
	}
	return nil
}

func onlineMode(s bedrockv1alpha1.ServerSettings) bool {
	if s.OnlineMode == nil {
		return true
	}
	return *s.OnlineMode
}

func boolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
