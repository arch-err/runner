//go:build ignore

// Publish release assets using the job's repository-scoped token.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var client = &http.Client{Timeout: 5 * time.Minute}

func request(method, endpoint, contentType string, body io.Reader) []byte {
	req, err := http.NewRequest(method, endpoint, body)
	must(err)
	req.Header.Set("Authorization", "token "+os.Getenv("FORGEJO_TOKEN"))
	req.Header.Set("Content-Type", contentType)
	res, err := client.Do(req)
	must(err)
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	must(err)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		panic(fmt.Sprintf("%s %s: HTTP %d: %s", method, endpoint, res.StatusCode, data))
	}
	return data
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	if os.Getenv("FORGEJO_TOKEN") == "" {
		panic("FORGEJO_TOKEN is required")
	}
	base := strings.TrimRight(os.Getenv("GITHUB_SERVER_URL"), "/") + "/api/v1/repos/" + os.Getenv("GITHUB_REPOSITORY") + "/releases"
	tag := os.Getenv("GITHUB_REF_NAME")
	metadata, err := json.Marshal(map[string]any{
		"tag_name": tag, "name": tag, "draft": true,
		"prerelease": strings.Contains(tag, "-"),
		"body":       "Standalone binaries for Linux, macOS and Windows. SHA256SUMS contains checksums. Windows binaries are cross-built; native Windows runtime verification is still needed.",
	})
	must(err)
	var release struct {
		ID int64 `json:"id"`
	}
	must(json.Unmarshal(request("POST", base, "application/json", bytes.NewReader(metadata)), &release))
	endpoint := fmt.Sprintf("%s/%d", base, release.ID)
	files, err := filepath.Glob("dist/release/*")
	must(err)
	if len(files) != 7 {
		panic("expected six binaries and SHA256SUMS")
	}
	for _, file := range files {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("attachment", filepath.Base(file))
		must(err)
		f, err := os.Open(file)
		must(err)
		_, err = io.Copy(part, f)
		f.Close()
		must(err)
		must(writer.Close())
		request("POST", endpoint+"/assets?name="+url.QueryEscape(filepath.Base(file)), writer.FormDataContentType(), &body)
	}
	request("PATCH", endpoint, "application/json", strings.NewReader(`{"draft":false}`))
	fmt.Println("Published", tag)
}
