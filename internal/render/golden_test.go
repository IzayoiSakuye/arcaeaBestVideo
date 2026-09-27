package render

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGoldenCommands 把 golden 文件里**真实捕获的 argv** 和 `*Args` 拼出来的逐字对比。
//
// 做法：从 golden 的 argv 里把输入/输出路径**原样取出来**再喂回 `*Args`。
// golden 里的路径已经是 `<ROOT>` / `<WORK>` token，所以不需要还原成真实路径 ——
// 只要两边用同一份 token 字符串，就能直接比。
//
// ⚠️ 这是"防翻译漂移"的唯一手段：TS spike 怎么拼的，Go 就得怎么拼。
func TestGoldenCommands(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "golden-commands-project.json"))
	if err != nil {
		t.Fatalf("读 golden 失败: %v", err)
	}
	var g struct {
		Cases []struct {
			Name string   `json:"name"`
			Argv []string `json:"argv"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatalf("解析 golden 失败: %v", err)
	}

	tested := 0
	for _, c := range g.Cases {
		ins := inputs(c.Argv)
		var got []string

		switch {
		case strings.HasPrefix(c.Name, "background"):
			if len(ins) != 1 {
				t.Fatalf("%s: 期望 1 个输入，实际 %d", c.Name, len(ins))
			}
			got = BackgroundArgs(ins[0], last(c.Argv))

		case strings.HasPrefix(c.Name, "composite"):
			if len(ins) != 5 {
				t.Fatalf("%s: 期望 5 个输入，实际 %d", c.Name, len(ins))
			}
			// 用哪种模板决定窗口坐标 —— 正好顺带验证 Templates 常量
			wide := strings.Contains(ins[3], "bg16-9")
			got = CompositeArgs(Layers{
				Clip: ins[0], BG: ins[1], Jacket: ins[2],
				Template: ins[3], Text: ins[4], Win: Templates[wide].Win,
			}, last(c.Argv))

		case strings.HasPrefix(c.Name, "render-text"):
			u, err := url.Parse(last(c.Argv))
			if err != nil {
				t.Fatalf("%s: URL 解析失败: %v", c.Name, err)
			}
			vals := map[string]string{}
			for k, v := range u.Query() {
				if len(v) > 0 {
					vals[k] = v[0]
				}
			}
			// golden 的 URL 用的是正斜杠，所以这里也传正斜杠形式
			got = TextArgs(strings.TrimPrefix(u.Path, "/"),
				argAfter(c.Argv, "--user-data-dir="), argAfter(c.Argv, "--screenshot="), vals)
			if len(got) != len(c.Argv) {
				t.Errorf("%s 长度不同: got %d / want %d", c.Name, len(got), len(c.Argv))
				continue
			}
			for i := range got {
				// ⚠️ 最后一项是带查询参数的 URL —— url.Values.Encode() 按 key 排序，
				//    而 spike 用的是插入顺序，所以必须比解析后的 map（规格 §3.7）
				if !sameArg(got[i], c.Argv[i]) {
					t.Errorf("%s 与 golden 不一致：\n  [%d] got : %s\n  [%d] want: %s",
						c.Name, i, got[i], i, c.Argv[i])
				}
			}
			tested++
			continue

		case c.Name == "concat":
			if len(ins) != 1 {
				t.Fatalf("%s: 期望 1 个输入，实际 %d", c.Name, len(ins))
			}
			got = ConcatArgs(ins[0], last(c.Argv))

		default:
			t.Logf("跳过 %s（没有对应的 *Args）", c.Name)
			continue
		}

		tested++
		if diff := argvDiff(got, c.Argv); diff != "" {
			t.Errorf("%s 与 golden 不一致：\n%s", c.Name, diff)
		}
	}
	t.Logf("对比了 %d/%d 条用例", tested, len(g.Cases))
}

// inputs 取所有 `-i` 后面的值（按出现顺序）
func inputs(a []string) []string {
	var out []string
	for i, v := range a {
		if v == "-i" && i+1 < len(a) {
			out = append(out, a[i+1])
		}
	}
	return out
}

// argAfter 取以 prefix 开头的那个参数，去掉 prefix
func argAfter(a []string, prefix string) string {
	for _, v := range a {
		if strings.HasPrefix(v, prefix) {
			return strings.TrimPrefix(v, prefix)
		}
	}
	return ""
}

func last(a []string) string { return a[len(a)-1] }

// sameArg 比较两个参数。file:// URL 只比 path + 查询参数【集合】。
func sameArg(a, b string) bool {
	if !strings.HasPrefix(a, "file://") || !strings.HasPrefix(b, "file://") {
		return a == b
	}
	ua, ea := url.Parse(a)
	ub, eb := url.Parse(b)
	if ea != nil || eb != nil || ua.Path != ub.Path {
		return false
	}
	qa, qb := ua.Query(), ub.Query()
	if len(qa) != len(qb) {
		return false
	}
	for k, va := range qa {
		vb, ok := qb[k]
		if !ok || len(va) != len(vb) {
			return false
		}
		for i := range va {
			if va[i] != vb[i] {
				return false
			}
		}
	}
	return true
}

// argvDiff 逐项对比，只打印不同的位置
func argvDiff(got, want []string) string {
	if len(got) != len(want) {
		return fmt.Sprintf("  长度不同: got %d / want %d\n  got : %q\n  want: %q",
			len(got), len(want), got, want)
	}
	var b strings.Builder
	for i := range got {
		if got[i] != want[i] {
			fmt.Fprintf(&b, "  [%d] got : %s\n  [%d] want: %s\n", i, got[i], i, want[i])
		}
	}
	return b.String()
}
