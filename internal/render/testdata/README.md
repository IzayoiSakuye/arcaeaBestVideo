# testdata/（render 包）

**从真实运行里捕获的 argv**，给 `internal/render` 的 golden test 当期望值。**有两份**：

| 文件 | 用例数 | 说明 |
|---|---|---|
| `golden-commands.json` | 7 | spike 的 `--songs` 模式（自己挑谱面）—— 历史行为基线 |
| **`golden-commands-project.json`** | **10** | ⭐ spike 的 `--project` 模式（按 `samples/sample-project.json` 的契约）—— **应该对齐这个** |

两份的区别与各自覆盖什么，见下面「两份 golden，别搞混」一节。

> 同目录下的 `golden-raw.jsonl` / `golden-project-raw.jsonl` 是**捕获回来的原料**
> （一行一条 argv，没做 token 替换，含本机绝对路径）。两份 golden 是从它整理出来的，
> **别删** —— 派生文件对不上时，只能靠它回溯。

## 为什么是"捕获"而不是"手抄"

Go 侧要把 spike 的命令**逐字翻译**过去，而防翻译漂移的唯一可靠手段是 golden test。
但期望值如果靠人肉抄，**抄错就等于没测** —— 这个项目真抄错过两次
（把「键缺席」写成「显式 null」、把 `file:///` 三斜杠说成"两斜杠会失败"）。

所以做法是：给 spike 加一个 `--log-commands <file>`（**只记录、不改变行为**），
跑一次真实管线，让它**自己把 argv 吐出来**。

## 怎么重新捕获

⚠️ **捕获用的 spike 已于 2026-10 删除**（`b49a19c`），现在只存在于 git 历史里。
`cmd/pipeline` 也还没有 `--log-commands`，所以捕获路径目前是**断的**。要重新捕获：

```bash
# 从历史里把 spike 捞出来（752 行）
git show b49a19c^:scripts/spike-sample-video.ts > /tmp/spike.ts

node /tmp/spike.ts --project samples/sample-project.json \
  --log-commands /tmp/cmds.jsonl --quiet
```

> 长期做法是给 `cmd/pipeline` 补一个**只记录、不改变行为**的 `--log-commands`。
> **改 `templates/` 版式或洞口坐标时你就需要它** —— 那正是这两份 golden 唯一测不住的地方。

然后把 `/tmp/cmds.jsonl` 里的机器相关路径换成下面三个 token。

> ⚠️ 重新捕获后**要重新核对** —— 尤其你如果改了 `templates/` 的版式或洞口坐标。

## 路径 token 约定

| token | 代表 | 例子 |
|---|---|---|
| `<ROOT>` | 仓库根目录 | `<ROOT>\templates\bg4-3.png` |
| `<WORK>` | 该次渲染的工作目录（每次重建） | `<WORK>\clip.mp4` |
| `<PROFILE>` | Edge 的 user-data-dir | `<PROFILE>` |

**测试里要做的**：把本地路径前缀换成同样的 token，再比对。

```go
// 示意
func tok(s, root string) string {
    s = strings.ReplaceAll(s, filepath.FromSlash(root), "<ROOT>")
    s = strings.ReplaceAll(s, filepath.ToSlash(root), "<ROOT>")
    // <WORK> / <PROFILE> 同理
    return s
}
```

> ⚠️ 注意**两种斜杠形式都要换** —— 普通参数用的是 Windows 反斜杠，
> 而 Edge 的 URL 用的是正斜杠（spike 里有个 `toPosix()`）。

## ⚠️ 捕获时发现的两个 spike 行为（翻译前必读）

### ① `diff` 传的是**定数**，不是等级

```
spike 实际传的：  diff=Future+10.9        ← DIFF_NAME[难度] + chart.constant
规格 §3.3 写的是：diff=Future 10+         ← 应该是 level
```

`spike-sample-video.ts:582` 那行是 `` diff: `${DIFF_NAME[...]} ${r.chart.constant}` ``。

**Go 侧要自己决定用哪个**，不能默认"照抄 spike"：

- 用 **`level`**（`"10+"`）→ 和游戏内显示一致，但部分曲目区分不出 10.7 / 10.8 / 10.9
- 用 **`constant`**（`"10.9"`）→ 信息更精确，但和游戏内写法不一致

**改了就要同步改这份 golden**，否则测试会红在一个"其实是你故意改的"地方。

### ② spike 默认不读 `samples/sample-project.json` —— **已加 `--project` 修掉**

默认模式（`--songs` / 自动挑）自己选谱面（`spike-sample-video.ts:308`、`529`）：

```ts
if (!['FTR', 'ETR', 'BYD'].includes(ch.difficulty)) continue;   // 先滤掉 PST/PRS
const chart = song.charts.find(c => pickSource(c) && c.constant != null && c.note)
           ?? song.charts.find(c => pickSource(c));              // 取第一个有录像的
```

后果：样例文件里 `testify` 写的是 **BYD 12**，这个模式实际渲染的是 **FTR 10.9**。

**现在加了 `--project` 模式**，按输入契约走（曲目 + 难度 + 成绩都照样例来）：
详见下面的「两份 golden」一节。

---

## ⭐ 两份 golden，别搞混

| 文件 | 怎么捕获的 | 是什么 | 用来测什么 |
|---|---|---|---|
| `golden-commands.json` | `--songs testify,tempestissimo` | **spike 的历史行为**（自己挑到 FTR / PRS） | 早期翻译期的基线，覆盖 4:3+16:9 两套模板 |
| **`golden-commands-project.json`** | `--project samples/sample-project.json` | ⭐ **应用的真实行为**（按契约选曲） | **`internal/render` 应该对齐这个** |

**两份并存是有意的**：argv 对不上时，能一眼区分「翻译错了」还是「行为有意改了」。

### `golden-commands-project.json` 多覆盖了什么

```
3 首全 BYD 谱面      ← 旧 golden 只覆盖 FTR / PRS
难度专属曲绘          ← composite-20x157-ownjacket-tempestissimo 用 tempestissimo_byd.jpg
样例里的真实成绩      ← score 用 achievement×100000 算，不再是 fakeScore 编的
16:9 与 4:3 混合      ← testify/arcanaeden 是 4:3（overlay 58:95），tempestissimo 是 16:9（overlay 20:157）
```

> ⚠️ **`composite-20x157-ownjacket-tempestissimo` 是唯一覆盖 `chart.jacket ?? song.jacket` 的用例。**
> 删了它，难度专属曲绘就完全没测了。

---

## 覆盖到的用例（7 条）

| name | song | 说明 |
|---|---|---|
| `background` ×2 | testify / tempestissimo | 背景铺满 + `gblur=sigma=25` |
| `render-text` ×2 | — | Edge 无头截图（URL 里含注入参数） |
| `composite-855x641-jacket-audio` | testify | **4:3 模板**、有曲绘、原音轨 |
| `composite-934x526-jacket-audio` | tempestissimo | **16:9 模板**、有曲绘、原音轨 |
| `concat` | — | concat demuxer |

两套模板的差异体现在：窗口尺寸、`overlay` 位置、`scale`/`crop` 参数。

## ⚠️ **没覆盖到的**（别以为测全了）

| 分支 | 为什么没有 |
|---|---|
| **无曲绘**（4 输入、标签跳号无 `o2`） | 两首歌都有曲绘。要补需要一首无曲绘的歌（实测 26/533 首） |
| **无音轨 → 补 `anullsrc`** | 抓不到：B站 下回来的片段都带音轨。要补得先造一个无音轨的片段 |
| **`-an`（关音频）** | 用 `--no-audio` 可以抓 |

> 这三分支**都要写单测**，但期望值得你按规格 3.4 手写 —— 或者先造出触发条件再捕获。
> 尤其 **`anullsrc` 那条**：它是"各段流数一致才能 concat"的关键，
> 而 spike 曾在这里踩过一个极隐蔽的坑（`execFileSync` 写成了 `fs.execFileSync`，
> 异常被 catch 吞掉 → 永远判定"无音轨" → 成片全程静音）。

## 文件格式

```jsonc
{
  "captured_at": "…",
  "source": "scripts/spike-sample-video.ts --log-commands",
  "root_token": "<ROOT>",
  "note": "…",
  "cases": [
    { "name": "…", "song": "…", "tool": "ffmpeg.exe", "argv": ["-y", "-v", "error", …] }
  ]
}
```

`argv` **不含工具路径本身**（那个由 `internal/toolchain` 解析）。
比对时只比 `argv`，不比 `tool`。
