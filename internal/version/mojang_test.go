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

package version

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractVersionFromURL(t *testing.T) {
	got := ExtractVersionFromURL("https://www.minecraft.net/bedrockdedicatedserver/bin-linux/bedrock-server-1.21.100.05.zip")
	if got != "1.21.100.05" {
		t.Fatalf("got %q", got)
	}
}

func TestClientLatestStable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":{"links":[{"downloadType":"serverBedrockLinux","downloadUrl":"https://example.com/bedrock-server-1.21.93.05.zip"},{"downloadType":"serverBedrockPreviewLinux","downloadUrl":"https://example.com/bedrock-server-1.22.0.01.zip"}]}}`))
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL}
	ver, url, err := c.LatestStable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ver != "1.21.93.05" || url == "" {
		t.Fatalf("got ver=%q url=%q", ver, url)
	}
}
