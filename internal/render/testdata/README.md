# testdata/（render 包）

`golden-commands.json` —— **从真实运行里捕获的 argv**，给 `internal/render` 的 golden test 当期望值。

## 为什么是"捕获"而不是"手抄"

Go 侧要把 spike 的命令**逐字翻译**过去，而防翻译漂移的唯一可靠手段是 golden test。
但期望值如果靠人肉抄，**抄错就等于没测** —— 这个项目真抄错过两次
（把「键缺席」写成「显式 null」、把 `file:///` 三斜杠说成"两斜杠会失败"）。

所以做法是：给 spike 加一个 `--log-commands <file>`（**只记录、不改变行为**），
跑一次真实管线，让它**自己把 argv 吐出来**。

## 怎么重新捕获

```bash
# 在仓库根目录
node scripts/spike-sample-video.ts --songs testify,tempestissimo \
  --log-commands /tmp/cmds.jsonl --quiet
```

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
