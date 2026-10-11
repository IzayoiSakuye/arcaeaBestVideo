package job

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rhythm-game-bvideo/internal/score"
)

func validProject() *score.Project {
	return &score.Project{
		SchemaVersion: 1,
		Game:          "arcaea",
		Songs: []score.Song{
			{Index: 1, SongID: "egoeimi", Difficulty: score.DiffRef{Name: "FTR"}},
		},
	}
}

// Project 非空时不许碰 ProjectPath —— 这是"校对表改完直接渲染"的根
func TestProjectPrefersInMemory(t *testing.T) {
	inMem := validProject()
	missing := filepath.Join(t.TempDir(), "不存在.json")
	j := NewJob(Deps{}, Config{Project: inMem, ProjectPath: missing}, nil)

	got, err := j.project()
	if err != nil {
		t.Fatalf("内存里有 Project，不该因为路径不存在而失败：%v", err)
	}
	if got != inMem {
		t.Error("该原样返回内存里那份（不是重新解一份）")
	}
}

func TestProjectFallsBackToPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.json")
	body := `{"schema_version":1,"game":"arcaea","songs":[
		{"index":1,"song_id":"egoeimi","difficulty":{"name":"FTR"}}]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := NewJob(Deps{}, Config{ProjectPath: path}, nil).project()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Songs) != 1 || got.Songs[0].SongID != "egoeimi" {
		t.Errorf("没读对文件：%+v", got.Songs)
	}
}

func TestProjectNeedsOneOfTheTwo(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "不存在.json")
	if _, err := NewJob(Deps{}, Config{ProjectPath: missing}, nil).project(); err == nil {
		t.Error("两条路都不通时必须报错")
	}
}

// 绕开 score.ValidateBytes 直接塞进来的 Project 必须当场拦住，
// 否则 renderOne 会拿一句"no video sources"来误导人
func TestProjectRejectsUnresolvedSongs(t *testing.T) {
	cases := []struct {
		name string
		song score.Song
	}{
		{"缺 song_id", score.Song{Difficulty: score.DiffRef{Name: "FTR"}}},
		{"缺难度代号", score.Song{SongID: "egoeimi"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proj := &score.Project{SchemaVersion: 1, Songs: []score.Song{tc.song}}
			if _, err := NewJob(Deps{}, Config{Project: proj}, nil).project(); err == nil {
				t.Error("该报错")
			}
		})
	}
}

// 定位信息必须来自行号：缺 song_id 时字段本身是空的，只有行号能指路
func TestProjectReportsRowIndex(t *testing.T) {
	proj := validProject()
	proj.Songs = append(proj.Songs,
		score.Song{Index: 2, SongID: "pupa", Difficulty: score.DiffRef{Name: "FTR"}},
		score.Song{Index: 3, Difficulty: score.DiffRef{Name: "FTR"}},
	)

	_, err := NewJob(Deps{}, Config{Project: proj}, nil).project()
	if err == nil {
		t.Fatal("第 3 首缺 song_id，该报错")
	}
	if !strings.Contains(err.Error(), "第 3 首") {
		t.Errorf("消息要指到第 3 首，实得 %q", err.Error())
	}
}

// Abs("") 返回 cwd 而不是报错 —— 空路径必须原样留着
func TestNormalizeSkipsEmptyPaths(t *testing.T) {
	j := NewJob(Deps{}, Config{Root: ".", ProjectPath: "", Output: "out.mp4"}, nil)
	if err := j.normalize(); err != nil {
		t.Fatal(err)
	}
	if j.cfg.ProjectPath != "" {
		t.Errorf("空路径被改成了 %q", j.cfg.ProjectPath)
	}
	if !filepath.IsAbs(j.cfg.Root) || !filepath.IsAbs(j.cfg.Output) {
		t.Error("非空路径应该被绝对化")
	}
}
