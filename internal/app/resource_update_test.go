package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallResourceAndKeepPreviousOnFailure(t *testing.T) {
	files := map[string][]byte{
		"meta.json":                    []byte(`{}`),
		"data/arcaea-catalog.json":     []byte(`{"schema_version":1,"songs":[{"song_id":"test","title_en":"Test","title_norm":"test","jacket":{},"charts":[{"difficulty":"FTR"}]}]}`),
		"data/arcaea-backgrounds.json": []byte(`{"songs":{"test":{"bg":"test"}},"backgrounds":{}}`),
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	m := resourceManifest{SchemaVersion: 1, Game: "arcaea", DataVersion: "2026.11.01"}
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(content)
		m.Files = append(m.Files, resourceFile{Path: name, Size: int64(len(content)), SHA256: hex.EncodeToString(h[:])})
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	archiveHash := sha256.Sum256(buf.Bytes())
	m.Archive.Path = "data-2026.11.01.zip"
	m.Archive.Size = int64(buf.Len())
	m.Archive.SHA256 = hex.EncodeToString(archiveHash[:])
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(buf.Bytes()) }))
	defer server.Close()
	m.Archive.URL = server.URL + "/" + m.Archive.Path
	if err := m.validate(); err != nil {
		t.Fatal(err)
	}
	store := t.TempDir()
	root, err := installResource(context.Background(), server.Client(), store, m, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, version, digest := installedResource(store); got != root || version != m.DataVersion || digest != m.Archive.SHA256 {
		t.Fatalf("installedResource = %q, %q, %q", got, version, digest)
	}
	if _, err := os.Stat(filepath.Join(root, "data", "arcaea-catalog.json")); err != nil {
		t.Fatal(err)
	}
	bad := m
	bad.DataVersion = "2026.11.02"
	bad.Archive.SHA256 = hex.EncodeToString(make([]byte, 32))
	if _, err := installResource(context.Background(), server.Client(), store, bad, nil); err == nil {
		t.Fatal("expected archive hash failure")
	}
	if got, _, _ := installedResource(store); got != root {
		t.Fatalf("old resource lost: %q", got)
	}
}

func TestManifestRejectsUnsafePath(t *testing.T) {
	m := resourceManifest{SchemaVersion: 1, Game: "arcaea", DataVersion: "2026.11.01"}
	m.Archive.URL = "https://example.com/data.zip"
	m.Archive.Path = "data.zip"
	m.Archive.Size = 1
	m.Archive.SHA256 = hex.EncodeToString(make([]byte, 32))
	m.Files = []resourceFile{{Path: "data/../outside", SHA256: m.Archive.SHA256}}
	if err := m.validate(); err == nil {
		t.Fatal("unsafe path accepted")
	}
}

func TestReleaseAPIManifestAndProxyArchiveURLs(t *testing.T) {
	hash := hex.EncodeToString(make([]byte, 32))
	m := resourceManifest{SchemaVersion: 1, Game: "arcaea", DataVersion: "7.1.0"}
	m.Archive.URL = "https://github.com/owner/repo/releases/download/tag/data.zip"
	m.Archive.Path = "data.zip"
	m.Archive.Size = 10
	m.Archive.SHA256 = hash
	for _, name := range []string{"meta.json", "data/arcaea-catalog.json", "data/arcaea-backgrounds.json"} {
		m.Files = append(m.Files, resourceFile{Path: name, SHA256: hash})
	}
	manifest, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(manifest)
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release":
			_ = json.NewEncoder(w).Encode(map[string]any{"assets": []map[string]string{{"name": "latest.json", "url": server.URL + "/asset", "digest": "sha256:" + hex.EncodeToString(sum[:])}}})
		case "/asset":
			if r.Header.Get("Accept") != "application/octet-stream" {
				t.Error("asset Accept header missing")
			}
			_, _ = w.Write(manifest)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	got, err := readManifestFromReleaseAPI(context.Background(), server.Client(), server.URL+"/release")
	if err != nil {
		t.Fatal(err)
	}
	if got.DataVersion != m.DataVersion {
		t.Fatalf("version = %s", got.DataVersion)
	}
	urls := resourceArchiveURLs(got)
	if len(urls) != 3 || !strings.HasPrefix(urls[0], resourceZipProxies[0]) || !strings.HasPrefix(urls[1], resourceZipProxies[1]) || urls[2] != m.Archive.URL {
		t.Fatalf("archive sources = %v", urls)
	}
}

func TestR2ResourceSource(t *testing.T) {
	t.Setenv("ARCAEA_RESOURCE_MIRRORS", "")
	if got := resourceManifestURLs()[0]; got != resourceR2ManifestURL {
		t.Fatalf("默认清单地址 = %s", got)
	}
	m := resourceManifest{sourceURL: resourceR2ManifestURL}
	m.Archive.Path = "data-v7.1.0.zip"
	m.Archive.URL = strings.TrimSuffix(resourceR2ManifestURL, "latest.json") + m.Archive.Path
	if got := resourceArchiveURLs(m); len(got) != 1 || got[0] != m.Archive.URL {
		t.Fatalf("R2 归档地址 = %v", got)
	}
}

func TestResourceGameVersionUpgrade(t *testing.T) {
	for _, tc := range []struct {
		old, next string
	}{
		{"2026.11.01", "7.1.0"},
		{"7.1.0", "7.1.0.1"},
		{"7.1.0.1", "7.1.1"},
	} {
		if compareResourceVersion(tc.old, tc.next) >= 0 || compareResourceVersion(tc.next, tc.old) <= 0 {
			t.Fatalf("版本顺序错误：%s → %s", tc.old, tc.next)
		}
	}
	if !resourceVersionPattern.MatchString("7.1.0") || !resourceVersionPattern.MatchString("7.1.0.1") || resourceVersionPattern.MatchString("7.01.0") {
		t.Fatal("游戏版本格式校验错误")
	}
	m := resourceManifest{DataVersion: "7.1.0"}
	if !resourceAvailable(m, "2026.11.01", "") {
		t.Fatal("旧日期版本未检测到更新")
	}
}
