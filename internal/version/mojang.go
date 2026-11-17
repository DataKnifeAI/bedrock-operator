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

// Package version polls Mojang's public Bedrock download links API
// (same endpoint the itzg image uses).
package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

const (
	// DownloadLinksURL is Mojang's public download-links API used by itzg.
	DownloadLinksURL = "https://net.web.minecraft-services.net/api/v1.0/download/links"

	downloadTypeLinux        = "serverBedrockLinux"
	downloadTypePreviewLinux = "serverBedrockPreviewLinux"
)

var versionFromURL = regexp.MustCompile(`bedrock-server-([0-9.]+)\.zip`)

// Client fetches available BDS versions from Mojang.
type Client struct {
	HTTP    *http.Client
	BaseURL string
}

// DefaultClient returns a client with sensible timeouts.
func DefaultClient() *Client {
	return &Client{
		HTTP: &http.Client{
			Timeout: 15 * time.Second,
		},
		BaseURL: DownloadLinksURL,
	}
}

type linksResponse struct {
	Result struct {
		Links []struct {
			DownloadType string `json:"downloadType"`
			DownloadURL  string `json:"downloadUrl"`
		} `json:"links"`
	} `json:"result"`
}

// LatestStable returns the current non-preview Bedrock dedicated server version.
func (c *Client) LatestStable(ctx context.Context) (version string, downloadURL string, err error) {
	return c.lookup(ctx, downloadTypeLinux)
}

// LatestPreview returns the current preview Bedrock dedicated server version.
func (c *Client) LatestPreview(ctx context.Context) (version string, downloadURL string, err error) {
	return c.lookup(ctx, downloadTypePreviewLinux)
}

func (c *Client) lookup(ctx context.Context, downloadType string) (string, string, error) {
	base := c.BaseURL
	if base == "" {
		base = DownloadLinksURL
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "DataKnifeAI/bedrock-operator")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("mojang download links: HTTP %d", resp.StatusCode)
	}

	var body linksResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", "", fmt.Errorf("decode mojang download links: %w", err)
	}
	for _, link := range body.Result.Links {
		if link.DownloadType != downloadType {
			continue
		}
		ver := ExtractVersionFromURL(link.DownloadURL)
		if ver == "" {
			return "", "", fmt.Errorf("could not parse version from %q", link.DownloadURL)
		}
		return ver, link.DownloadURL, nil
	}
	return "", "", fmt.Errorf("download type %q not found in Mojang feed", downloadType)
}

// ExtractVersionFromURL parses bedrock-server-<ver>.zip from a download URL.
func ExtractVersionFromURL(url string) string {
	m := versionFromURL.FindStringSubmatch(url)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}
