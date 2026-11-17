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

package controller

import (
	"context"
	"strings"
	"time"

	bedrockv1alpha1 "github.com/DataKnifeAI/bedrock-operator/api/v1alpha1"
	"github.com/DataKnifeAI/bedrock-operator/internal/gamecfg"
	"github.com/DataKnifeAI/bedrock-operator/internal/version"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type updateResult struct {
	AvailableVersion string
	InstalledVersion string
	UpdateAvailable  bool
}

// checkUpdates polls Mojang's download links API and optionally applies version bumps.
// Default notifyOnly=true: status only. When notifyOnly=false and an update is available
// while mode=LATEST, set directDownloadUrl so the next pod restart re-downloads.
func (r *BedrockServerReconciler) checkUpdates(ctx context.Context, server *bedrockv1alpha1.BedrockServer) (updateResult, time.Duration) {
	log := logf.FromContext(ctx)
	interval := CheckInterval(server.Spec)

	if server.Status.LastUpdateCheck != nil {
		elapsed := time.Since(server.Status.LastUpdateCheck.Time)
		if elapsed < interval && server.Status.AvailableVersion != "" {
			return updateResult{
				AvailableVersion: server.Status.AvailableVersion,
				InstalledVersion: server.Status.InstalledVersion,
				UpdateAvailable:  server.Status.UpdateAvailable,
			}, interval - elapsed
		}
	}

	client := r.Versions
	if client == nil {
		client = version.DefaultClient()
	}

	mode := strings.ToUpper(strings.TrimSpace(server.Spec.Version.Mode))
	if mode == "" {
		mode = bedrockv1alpha1.VersionModeLatest
	}

	var (
		avail string
		url   string
		err   error
	)
	switch mode {
	case bedrockv1alpha1.VersionModePreview:
		avail, url, err = client.LatestPreview(ctx)
	default:
		avail, url, err = client.LatestStable(ctx)
	}
	if err != nil {
		log.Info("mojang version check failed", "error", err)
		return updateResult{
			AvailableVersion: server.Status.AvailableVersion,
			InstalledVersion: installedHint(server),
			UpdateAvailable:  server.Status.UpdateAvailable,
		}, interval
	}

	installed := installedHint(server)
	updateAvail := installed != "" && avail != "" && installed != avail
	// When running LATEST/PREVIEW with no recorded install yet, still surface available.
	if installed == "" && avail != "" && (mode == bedrockv1alpha1.VersionModeLatest || mode == bedrockv1alpha1.VersionModePreview) {
		// Treat as "feed known"; updateAvailable stays false until we know installed.
		updateAvail = false
	}
	if mode == bedrockv1alpha1.VersionModePinned && server.Spec.Version.Pinned != "" {
		installed = server.Spec.Version.Pinned
		updateAvail = avail != installed
	}

	res := updateResult{
		AvailableVersion: avail,
		InstalledVersion: installed,
		UpdateAvailable:  updateAvail,
	}

	if updateAvail && !gamecfg.NotifyOnly(server.Spec) && url != "" {
		// Opt-in apply: pin DIRECT_DOWNLOAD_URL to the latest zip so restart re-downloads.
		if server.Spec.Version.DirectDownloadUrl != url {
			patched := server.DeepCopy()
			patched.Spec.Version.DirectDownloadUrl = url
			if mode == bedrockv1alpha1.VersionModePinned {
				patched.Spec.Version.Pinned = avail
			}
			if err := r.Update(ctx, patched); err != nil {
				log.Info("failed to apply version bump", "error", err)
			} else {
				log.Info("applied version bump via directDownloadUrl", "version", avail)
			}
		}
	}

	return res, interval
}

func installedHint(server *bedrockv1alpha1.BedrockServer) string {
	if server.Status.InstalledVersion != "" {
		return server.Status.InstalledVersion
	}
	mode := strings.ToUpper(strings.TrimSpace(server.Spec.Version.Mode))
	if mode == bedrockv1alpha1.VersionModePinned {
		return strings.TrimSpace(server.Spec.Version.Pinned)
	}
	return ""
}
