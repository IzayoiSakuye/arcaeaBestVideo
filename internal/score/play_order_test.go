package score

import "testing"

func TestEffectivePlayOrder(t *testing.T) {
	p := &Project{Songs: make([]Song, 2), Thoughts: []Thought{{ID: "memo"}}}
	order, err := p.EffectivePlayOrder()
	if err != nil || len(order) != 2 || order[0].SongIndex != 2 || order[1].SongIndex != 1 {
		t.Fatalf("默认顺序：%v, %v", order, err)
	}
	p.PlayOrder = []PlayItem{{Kind: "thought", ThoughtID: "memo"}, {Kind: "song", SongIndex: 2}}
	order, err = p.EffectivePlayOrder()
	if err != nil || len(order) != 2 {
		t.Fatalf("感想编排：%v, %v", order, err)
	}
	p.PlayOrder = append(p.PlayOrder, PlayItem{Kind: "thought", ThoughtID: "memo"})
	if _, err := p.EffectivePlayOrder(); err == nil {
		t.Fatal("重复感想应被拒绝")
	}
	p.PlayOrder = p.PlayOrder[:2]
	p.PlayOrder = append(p.PlayOrder, PlayItem{Kind: "song", SongIndex: 2})
	if _, err := p.EffectivePlayOrder(); err == nil {
		t.Fatal("重复歌曲应被拒绝")
	}
	p.PlayOrder = []PlayItem{}
	if _, err := p.EffectivePlayOrder(); err == nil {
		t.Fatal("空播放顺序应被拒绝")
	}
}
