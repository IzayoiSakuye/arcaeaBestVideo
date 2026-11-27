package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"rhythm-game-bvideo/internal/resource"
	"rhythm-game-bvideo/internal/score"
)

func TestQALocalIdentityChangeRequiresConfirmation(t *testing.T) {
	for _, change := range []string{"size", "mtime", "path", "unsaved-range"} {
		t.Run(change, func(t *testing.T) {
			a, song, probe := qaVideoApp(t)
			selection, err := a.ConfirmVideoRange(song, score.ClipRange{StartMs: 500, DurationMs: 1000})
			if err != nil {
				t.Fatal(err)
			}
			song = selection.Song
			info, err := os.Stat(song.Video.LocalPath)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "size":
				data, err := os.ReadFile(song.Video.LocalPath)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(song.Video.LocalPath, append(data, 0), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(song.Video.LocalPath, info.ModTime(), info.ModTime()); err != nil {
					t.Fatal(err)
				}
			case "mtime":
				if err := os.Chtimes(song.Video.LocalPath, info.ModTime(), info.ModTime().Add(time.Second)); err != nil {
					t.Fatal(err)
				}
			case "path":
				data, err := os.ReadFile(song.Video.LocalPath)
				if err != nil {
					t.Fatal(err)
				}
				video := *song.Video
				video.LocalPath = filepath.Join(t.TempDir(), "other.mp4")
				if err := os.WriteFile(video.LocalPath, data, 0600); err != nil {
					t.Fatal(err)
				}
				song.Video = &video
			case "unsaved-range":
				clip := *song.Clip
				clip.StartMs++
				song.Clip = &clip
			}
			states, err := a.VideoInfos(&score.Project{Songs: []score.Song{song}})
			if err != nil || states[0].Confirmed || states[0].MaterialReady {
				t.Fatalf("身份或范围变化仍被确认为就绪：%+v %v", states, err)
			}
			if _, _, err := resource.VideoMaterial(context.Background(), probe, filepath.Join(a.cacheDir(), "clips"), song); err == nil {
				t.Fatal("身份或范围变化未经重新确认即可取材")
			}
			prepared, err := a.PrepareVideo(song)
			if err != nil || !prepared.PreviewReady || prepared.Confirmed || prepared.MaterialReady || prepared.RequestID == selection.State.RequestID {
				t.Fatalf("准备新身份应要求重确认：%+v %v", prepared, err)
			}
		})
	}
}

func TestQAFailedRetryTaskAndGenerationIsolation(t *testing.T) {
	for _, source := range []string{"youtube", "bilibili", "local"} {
		t.Run(source, func(t *testing.T) {
			a, song, probe := qaVideoApp(t)
			fixture, err := os.ReadFile(song.Video.LocalPath)
			if err != nil {
				t.Fatal(err)
			}
			qaSidecar(t, a, probe)
			t.Setenv("QA_FIXTURE", song.Video.LocalPath)
			if source == "youtube" {
				song.Video = &score.VideoRef{URL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ"}
			} else if source == "bilibili" {
				song.Video = &score.VideoRef{URL: "https://www.bilibili.com/video/BV1op4y1W7M2?p=1"}
			}
			if _, err := a.PrepareVideo(song); err != nil {
				t.Fatal(err)
			}
			qaWait(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return !a.videoWorker })
			selection, err := a.ConfirmVideoRange(song, score.ClipRange{StartMs: 500, DurationMs: 1000})
			if err != nil {
				t.Fatal(err)
			}
			song = selection.Song
			qaWait(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return !a.videoWorker })
			path := a.videoMaterialPath(song)
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			states, err := a.VideoInfos(&score.Project{Songs: []score.Song{song}})
			if err != nil || states[0].Status != "failed" || states[0].Error == "" || states[0].RequestID != selection.State.RequestID {
				t.Fatalf("失败须保留诊断和原请求：%+v %v", states, err)
			}
			calls, _ := os.ReadFile(os.Getenv("QA_SIDECAR_LOG"))
			if _, _, err := resource.VideoMaterial(context.Background(), probe, filepath.Join(a.cacheDir(), "clips"), song); err == nil {
				t.Fatal("缺失素材仍可取材")
			}
			afterCalls, _ := os.ReadFile(os.Getenv("QA_SIDECAR_LOG"))
			if string(calls) != string(afterCalls) {
				t.Fatal("生成取材触发下载")
			}
			if source == "local" {
				if err := os.WriteFile(path, fixture, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
					t.Fatal(err)
				}
			}
			key := videoSourceKey(song.SongID, song.Difficulty.Name)
			a.mu.Lock()
			old := a.videoTasks[key]
			a.mu.Unlock()
			retried, err := a.RetryVideoPreparation(song.SongID, song.Difficulty.Name, states[0].RequestID)
			if err != nil || retried.RequestID == states[0].RequestID {
				t.Fatalf("原请求恢复失败：%+v %v", retried, err)
			}
			a.mu.Lock()
			isolated := a.videoTasks[key] != old && old.State.RequestID == states[0].RequestID && old.State.Status == "failed"
			a.mu.Unlock()
			if !isolated {
				t.Error("重试复用了旧任务对象或改写了旧请求")
			}
			qaWait(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return !a.videoWorker })
			states, err = a.VideoInfos(&score.Project{Songs: []score.Song{song}})
			if err != nil || !states[0].Confirmed || !states[0].MaterialReady || states[0].RequestID != retried.RequestID {
				t.Fatalf("恢复后确认或取材未就绪：%+v %v", states, err)
			}
		})
	}
}
