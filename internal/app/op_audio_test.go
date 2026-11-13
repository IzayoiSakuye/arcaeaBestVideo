package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestOPAudioManifestAndInstall(t *testing.T) {
	wav := append([]byte("RIFF"), make([]byte, 40)...)
	copy(wav[8:], "WAVE")
	hash := sha256.Sum256(wav)
	var manifest opManifest
	manifest.SchemaVersion, manifest.Game, manifest.Resource, manifest.AudioVersion = 1, "arcaea", "op-audio", "7.1.0"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/op-latest.json" {
			_ = json.NewEncoder(w).Encode(manifest)
			return
		}
		_, _ = w.Write(wav)
	}))
	defer server.Close()
	for i := 1; i <= 7; i++ {
		kind := "op" + string(rune('0'+i))
		manifest.Files = append(manifest.Files, opFile{Path: "templates/audio/" + kind + ".wav", Size: int64(len(wav)), SHA256: hex.EncodeToString(hash[:]), URL: server.URL + "/op/" + kind + ".wav"})
	}
	m, err := readOPManifest(context.Background(), server.Client(), server.URL+"/op-latest.json")
	if err != nil {
		t.Fatal(err)
	}
	manifest.Files[0].URL = server.URL + "/op/OP1.wav"
	if _, err := readOPManifest(context.Background(), server.Client(), server.URL+"/op-latest.json"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := installOPAudio(context.Background(), server.Client(), dir, m.Files[0], nil); err != nil {
		t.Fatal(err)
	}
	final := filepath.Join(dir, "op1-"+m.Files[0].SHA256+".wav")
	if got, err := os.ReadFile(final); err != nil || string(got) != string(wav) {
		t.Fatalf("安装内容不符：%v", err)
	}
	bad := m.Files[1]
	bad.SHA256 = hex.EncodeToString(make([]byte, 32))
	if err := installOPAudio(context.Background(), server.Client(), dir, bad, nil); err == nil {
		t.Fatal("错误校验值未被拒绝")
	}
	if _, err := os.Stat(filepath.Join(dir, "op2-"+bad.SHA256+".wav")); !os.IsNotExist(err) {
		t.Fatalf("校验失败留下了音频：%v", err)
	}
	manifest.Files[0].URL = "https://example.com/op1.wav"
	if _, err := readOPManifest(context.Background(), server.Client(), server.URL+"/op-latest.json"); err == nil {
		t.Fatal("跨来源下载地址未被拒绝")
	}
}

func TestOPAudioFileMatches(t *testing.T) {
	wav := append([]byte("RIFF"), make([]byte, 40)...)
	copy(wav[8:], "WAVE")
	hash := sha256.Sum256(wav)
	file := filepath.Join(t.TempDir(), "op1.wav")
	if err := os.WriteFile(file, wav, 0o644); err != nil {
		t.Fatal(err)
	}
	entry := opFile{Size: int64(len(wav)), SHA256: hex.EncodeToString(hash[:])}
	if !opAudioFileMatches(file, entry) {
		t.Fatal("打包音频应匹配清单")
	}
	entry.SHA256 = hex.EncodeToString(make([]byte, 32))
	if opAudioFileMatches(file, entry) {
		t.Fatal("哈希不符的打包音频不能视为已更新")
	}
}
