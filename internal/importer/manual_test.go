package importer

import (
	"context"
	"os"
	"strings"
	"testing"

	"rhythm-game-bvideo/internal/score"
)

const projectFixture = "../score/testdata/good-minimal.json"

// 手抄 JSON 是成绩的常规来路：读进来就得是个能校对的 Project
func TestManualJSONProviderReadsProject(t *testing.T) {
	data, err := os.ReadFile(projectFixture)
	if err != nil {
		t.Skipf("没有夹具：%v", err)
	}
	p := &ManualJSONProvider{}

	draft, err := p.Import(context.Background(), score.Input{Kind: score.KindJSON, Path: projectFixture})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(draft.Project.Songs); n == 0 {
		t.Fatal("一首都没读出来")
	}
	if draft.Notes != nil {
		t.Errorf("手抄路径不该有 Notes：%v", draft.Notes)
	}

	// 同一份内容贴进来也得能用
	pasted, err := p.Import(context.Background(), score.Input{Kind: score.KindJSON, Text: string(data)})
	if err != nil {
		t.Fatal(err)
	}
	if len(pasted.Project.Songs) != len(draft.Project.Songs) {
		t.Errorf("粘贴读出 %d 首，文件读出 %d 首", len(pasted.Project.Songs), len(draft.Project.Songs))
	}
	t.Logf("读出 %d 首，玩家 PTT %v", len(draft.Project.Songs), draft.Project.Player.Rating)
}

// 报错必须说清是哪一行、哪个字段（规格 §3.4）—— 只写一句 invalid project file 等于没报
func TestManualJSONProviderErrorMessages(t *testing.T) {
	cases := []struct {
		name string
		in   score.Input
		want []string
	}{
		{"文件不存在", score.Input{Path: "../score/testdata/nope.json"}, []string{"nope.json", "请检查文件"}},
		{"语法错", score.Input{Path: "../score/testdata/bad-syntax.json"}, []string{"语法错误", "4 行", "4 列", "songs"}},
		{"版本不对", score.Input{Path: "../score/testdata/bad-schema.json"}, []string{"99", "预期 1"}},
		{"类型错", score.Input{Path: "../score/testdata/bad-type.json"}, []string{"score", "类型错误"}},
		{"什么都没给", score.Input{}, []string{"路径"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := (&ManualJSONProvider{}).Import(context.Background(), c.in)
			if err == nil {
				t.Fatal("该报错的情况没报错")
			}
			for _, want := range c.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("报错里没有 %q：%v", want, err)
				}
			}
			t.Logf("%s → %v", c.name, err)
		})
	}
}
