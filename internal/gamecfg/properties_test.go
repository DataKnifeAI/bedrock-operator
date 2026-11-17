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

package gamecfg

import (
	"testing"

	bedrockv1alpha1 "github.com/DataKnifeAI/bedrock-operator/api/v1alpha1"
)

func TestEnvFromSpec_EULARequired(t *testing.T) {
	_, err := EnvFromSpec(bedrockv1alpha1.BedrockServerSpec{}, "world")
	if err == nil {
		t.Fatal("expected EULA error")
	}
}

func TestEnvFromSpec_SettingsMapping(t *testing.T) {
	spec := bedrockv1alpha1.BedrockServerSpec{
		EULA: bedrockv1alpha1.EULAConfig{Accepted: true},
		Settings: bedrockv1alpha1.ServerSettings{
			LevelName:                    "survival-1",
			LevelSeed:                    "42",
			LevelType:                    "FLAT",
			GameMode:                     "creative",
			Difficulty:                   "hard",
			MaxPlayers:                   8,
			AllowCheats:                  true,
			DefaultPlayerPermissionLevel: "operator",
			WhiteList:                    true,
			IPv6Enabled:                  true,
		},
		Version: bedrockv1alpha1.VersionConfig{Mode: "LATEST"},
	}
	env, err := EnvFromSpec(spec, "fallback")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range env {
		got[e.Name] = e.Value
	}
	assertEq(t, got["EULA"], "TRUE")
	assertEq(t, got["LEVEL_NAME"], "survival-1")
	assertEq(t, got["LEVEL_SEED"], "42")
	assertEq(t, got["LEVEL_TYPE"], "FLAT")
	assertEq(t, got["GAMEMODE"], "creative")
	assertEq(t, got["DIFFICULTY"], "hard")
	assertEq(t, got["MAX_PLAYERS"], "8")
	assertEq(t, got["ALLOW_CHEATS"], "true")
	assertEq(t, got["DEFAULT_PLAYER_PERMISSION_LEVEL"], "operator")
	assertEq(t, got["ALLOW_LIST"], "true")
	assertEq(t, got["WHITE_LIST"], "true")
	assertEq(t, got["VERSION"], "LATEST")
	assertEq(t, got["SERVER_PORT"], "19132")
	assertEq(t, got["SERVER_PORT_V6"], "19133")
}

func TestVersionEnv_PinnedRequiresValue(t *testing.T) {
	_, err := VersionEnv(bedrockv1alpha1.VersionConfig{Mode: "PINNED"})
	if err == nil {
		t.Fatal("expected pinned error")
	}
	env, err := VersionEnv(bedrockv1alpha1.VersionConfig{Mode: "PINNED", Pinned: "1.21.93.05"})
	if err != nil {
		t.Fatal(err)
	}
	if env[0].Value != "1.21.93.05" {
		t.Fatalf("got %v", env)
	}
}

func TestValidateSpec_GatewayAddress(t *testing.T) {
	spec := bedrockv1alpha1.BedrockServerSpec{
		EULA: bedrockv1alpha1.EULAConfig{Accepted: true},
	}
	if err := ValidateSpec(spec); err == nil {
		t.Fatal("expected gateway.address required")
	}
	enabled := false
	spec.Gateway.Enabled = &enabled
	if err := ValidateSpec(spec); err != nil {
		t.Fatal(err)
	}
}

func assertEq(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
