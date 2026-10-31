package score

import "fmt"

// EffectivePlayOrder 保留未编排项目的原始成绩顺序
func (p *Project) EffectivePlayOrder() ([]PlayItem, error) {
	if p.PlayOrder == nil {
		order := make([]PlayItem, len(p.Songs))
		for i := range p.Songs {
			order[i] = PlayItem{Kind: "song", SongIndex: i + 1}
		}
		return order, nil
	}
	if len(p.PlayOrder) == 0 {
		return nil, fmt.Errorf("播放顺序不能为空")
	}
	seen := make(map[int]bool)
	seenThoughts := make(map[string]bool)
	thoughts := make(map[string]bool)
	for _, thought := range p.Thoughts {
		thoughts[thought.ID] = true
	}
	for i, item := range p.PlayOrder {
		switch item.Kind {
		case "song":
			if item.SongIndex < 1 || item.SongIndex > len(p.Songs) || seen[item.SongIndex] {
				return nil, fmt.Errorf("播放顺序第 %d 项的歌曲编号无效或重复", i+1)
			}
			seen[item.SongIndex] = true
		case "thought":
			if item.ThoughtID == "" || !thoughts[item.ThoughtID] || seenThoughts[item.ThoughtID] {
				return nil, fmt.Errorf("播放顺序第 %d 项的感想片段不存在或重复", i+1)
			}
			seenThoughts[item.ThoughtID] = true
		default:
			return nil, fmt.Errorf("播放顺序第 %d 项的类型无效", i+1)
		}
	}
	return p.PlayOrder, nil
}
