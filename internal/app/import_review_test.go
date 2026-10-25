package app

import (
	"os"
	"path/filepath"
	"testing"

	"rhythm-game-bvideo/internal/score"
)

func testApp(t *testing.T) *App {
	t.Helper()
	root := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("APPDATA", configDir)
	t.Setenv("XDG_CONFIG_HOME", configDir)
	if err := os.Mkdir(filepath.Join(root, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join("..", "catalog", "testdata", "catalog-mini.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "data", "arcaea-catalog.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return &App{root: root}
}

func testProject() *score.Project {
	return &score.Project{SchemaVersion: score.SchemaVersion, Game: "arcaea", Songs: []score.Song{{
		Index: 1, SongID: "complete", Difficulty: score.DiffRef{Constant: 10.5},
		Score: score.ScoreRef{Achievement: 99.68409, ClearType: "HC"},
	}}}
}

func TestP2ImportReviewSearch(t *testing.T) {
	a := testApp(t)
	draft, err := a.Import(score.KindJSON, "", `{"schema_version":1,"game":"arcaea","songs":[{"index":1,"song_id":"complete","difficulty":{"constant":10.5},"score":{"achievement":99.68409,"clear_type":"HC"}}]}`)
	if err != nil {
		t.Fatal(err)
	}
	review, err := a.Review(draft.Project)
	if err != nil {
		t.Fatal(err)
	}
	if !review.Renderable || len(review.Rows) != 1 || review.Rows[0].Title != "Complete Song" || review.Project.Songs[0].Difficulty.Name != "FTR" {
		t.Fatalf("校对结果不完整：%+v", review)
	}
	if review.Rows[0].PTT == nil {
		t.Fatal("单曲 PTT 未计算")
	}
	if len(review.Rows[0].Difficulties) != 1 || review.Rows[0].Difficulties[0] != "FTR" {
		t.Fatalf("难度候选与曲库不一致：%v", review.Rows[0].Difficulties)
	}
	choices, err := a.SearchSongs("complete", 5)
	if err != nil || len(choices) != 1 || choices[0].SongID != "complete" {
		t.Fatalf("搜曲结果：%+v, %v", choices, err)
	}
	bad := testProject()
	bad.Songs[0].SongID = "missing"
	checked, err := a.Review(bad)
	if err != nil {
		t.Fatal(err)
	}
	if checked.Renderable {
		t.Fatal("无法定位的曲目不应允许渲染")
	}
}
