package score

import (
	"encoding/json"
	"testing"
)

func TestThoughtEffectivePages(t *testing.T) {
	legacy := Thought{Texts: []string{"第一段", "第二段"}, Image: &ThoughtImage{Path: "old.png", Width: 200}}
	pages := legacy.EffectivePages()
	if len(pages) != 2 || pages[0].Text != "第一段" || pages[1].Images[0].Path != "old.png" {
		t.Fatalf("旧感想图片未保留：%+v", pages)
	}
	modern := Thought{Pages: []ThoughtPage{{Text: "新段落", Images: []ThoughtImage{{Path: "a.png"}, {Path: "b.png"}}}}, Texts: legacy.Texts}
	pages = modern.EffectivePages()
	if len(pages) != 1 || len(pages[0].Images) != 2 || pages[0].Text != "新段落" {
		t.Fatalf("新段落图片未按段读取：%+v", pages)
	}
}

func TestProjectThoughtsRoundTrip(t *testing.T) {
	original := Project{SchemaVersion: SchemaVersion, Thoughts: []Thought{
		{ID: "intro", Name: "开场", DurationMs: 3000},
		{ID: "outro", Name: "结尾", DurationMs: 5000},
	}}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeProject(data, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Thoughts) != 2 || decoded.Thoughts[0].ID != "intro" || decoded.Thoughts[1].Name != "结尾" {
		t.Fatalf("多个感想片段未保留：%+v", decoded.Thoughts)
	}
}
