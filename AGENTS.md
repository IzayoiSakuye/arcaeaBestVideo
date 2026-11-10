# AGENTS.md

> 注入给每个 agent 的工作区上下文。**先读它，再读 `docs/设计文档.md`。**
> 详细论证在 `docs/`，这里只放「必须知道的结论」与「不可违反的规则」。

---

## 项目

**Arcaea Best50 视频生成器**：用户导入 Best50 成绩，工具配齐素材、按模板合成可投稿到 B站的视频。**本项目只支持 Arcaea Best50，不规划其他游戏或 Best40。**`docs/设计文档.md` 中的多游戏扩展是早期设想，以本条为准。

| 项 | 内容 |
|---|---|
| 支持范围 | **仅 Arcaea Best50** |
| 形态 | C 端桌面应用，**Windows 优先**（不是 Web 服务） |
| 定位 | 高质量爱好者工具 / 技术作品集，**不是创业项目** |

**核心差异化（竞品实测）**：50 段渲染 ≥60 分钟 → **≤10 分钟**；安装包 486MB → **控制体积，已不硬限 70MB**；
峰值内存 OOM → ≤2GB；新增主题改代码 → **写 HTML/CSS**。

---

## 当前状态（2026-11）

| 阶段 | 状态 |
|---|---|
| 方案设计 | ✅ `docs/设计文档.md`（唯一设计文档） |
| 数据层（曲库/索引/曲绘/背景/合并 + 打包） | ✅ 已迁至独立资源仓库 `IzayoiSakuye/arcaea-b50-resources`；应用可安装 R2 分发的曲库和 OP 音频，源码仓库的 `data/` 仅供本地构建 |
| P0 地基 | ✅ |
| 阶段 1–4：`catalog` / `tools` / `render` / `resource` + `cmd/pipeline` | ✅ **端到端跑通** |
| 阶段 5 性能实测 | ✅ **50 段外推 6.3 分钟**（预算 ≤10） |
| 阶段 6 收尾 | ✅ spike 已删（`b49a19c`）· 从零重跑通过 · G1 材料已写 |
| **🚦 门禁 G1** | ✅ **已过**（2026-10，多数玩家说想要） |
| **P2 成绩层** | ✅ 用户已确认测试通过；b50 图、手抄 JSON 导入、校对与成绩自检已接入 |
| **P3 素材层** | ✅ 导入后逐首获取完整录像，已缓存的跳过；可预览、改 B站或 YouTube 链接、选本地 MP4。手动来源按曲目和难度写入用户配置；片尾署名仍待完成 |
| **🚦 门禁 G3** | ✅ **2026-10-26 经用户人工确认通过**；测试样本数与全自动／人工补素材成片率尚未提供，不填写数值 |
| **P4 片段与编排** | ✅ 单曲片段、感想编辑与预览、顺序编排及感想入片已实现；视频和音频首尾 1 秒淡化。OP1–7 可从独立资源清单按首获取，自制样式暂缓 |
| **P5 打包分发** | 🔄 Windows NSIS 安装器已可构建；FFmpeg、FFprobe、yt-dlp 由用户下载并在设置中配置，干净机器验收未完成 |

**验收已过的四条**：`go build/vet/test` 干净 · 每段 ffprobe 参数一致且 `concat -c copy` 成功 ·
单段耗时 ×50 ≤10 分钟 · **运行时不写 `data/`** · 产物幂等（输入指纹 + `.part` 原子改名）。

**P3 当前边界**：下载时显示自动获取进度，结束后只列未就绪曲目；没有可重试来源且无批量错误时隐藏自动获取框。无来源曲目须计入待补，不能把 `0/0` 当作全部完成。逐段合成须在滤镜末尾转为 `yuv420p`、`tv` 色值范围；复用旧片段前检查 `pix_fmt`，`yuvj420p` 必须重渲染，不能放宽 concat 参数校验。

**单曲片段选择**：04 页用一条双端点滑条和分:秒输入设置开始／结束时间，预览只播放选中区间；点击「应用片段」写入当前 `Project.Songs[].clip`（起点毫秒 + 时长毫秒）。未设置时仍取前 10 秒；开始晚于结束、时长为 0、超出录像时长分别提示，最短 1 秒。修改区间只重渲对应片段，不重新下载完整录像；重新导入成绩会清空片段选择。自定义时长不再适用默认 10 秒的性能外推。

**P4 感想与编排**：03 页感想支持多片段、多段文字、多张装饰图、OP1–7／自选音频／静音及独立预览。OP 音频截取范围与感想时长独立，播完所选范围后循环；自选音频仍按截取范围决定时长。片段存于本次 `Project.Thoughts`，重新导入或退出会清空；05 页另存 `Project.PlayOrder`，每首歌曲和每段感想最多出现一次，生成时按该顺序拼入整片。每段视频首尾淡化 1 秒，片段交界音频各淡化 1 秒；短于 4 秒时淡化时长为片长的四分之一。仍用 concat demuxer 无损拼接，视频叠加转场与歌曲音频延续到感想留待后续版本。自制样式也移至后续版本。

**OP 音频分发决策**：本地 `templates/audio/op1.wav`–`op7.wav` 供开发使用，WAV 和 `.pkf` 不入 Git。OP 音频已在 R2 按首分发，客户端根据独立清单逐首下载并校验 SHA-256；发布前确认分发授权。缺 OP 时保持静音和自定义音频可用。

**性能实测（3 首，缓存命中）**：

```
composite   6.39s/段   85%   ← 瓶颈，动它就动这里
text        0.96s/段   13%
background  0.18s/段    2%
concat      0.5s/次
50 段外推 ≈ 6.3 分钟 ✅
```

> ⚠️ **冷启动最慢**：用户缓存目录中的 `work/edge-profile/` 不存在时首轮 text 涨到 1.55s/段，外推 **6.8 分钟**，
> 之后回落到 0.96s/段。两个数都在预算内，但报数时要说清是冷还是热。
> 真超预算时第一个该动的是 `-preset`（`medium` → `veryfast`，快 2–3 倍、文件大 10–20%）。

**G1 门禁**（`docs/设计文档.md` §9）：把成品给 **5–10 个 Arcaea 玩家**看，
问「你会想要一个吗？」**多数人说想要 → 进 P2；反应平淡 → 停下来重新想产品形态。**

> ✅ **2026-10 已过**：多数玩家说想要。反馈里唯一的**具体短板是「成绩录入太麻烦 / 要能直接导入截图」**
> —— 所以 P2 的顺序是**骨架优先**（导入 → 校对 → 出片），OCR 押后；
> 且校对表要能在录入时**实时跑自检**，把"打错数字"当场暴露（见 `docs/设计文档.md` §8）。
> ✅ 成绩图已收到，并抄成 `internal/score/testdata/b50-from-image.json`（50 行，被图自带的
> Σb50 / Σb10 / PTT 三个校验和验证过）—— G2 的标准答案有了。
> ⚠️ 只剩一条待办：**wiki 的 Anubis 盾过不去**（`anubis-fetch.ts` 连续 3 次挑战失败，
> 数据管线刷新可能已受影响）。

---

## 技术栈

已确认：**Go 1.26.4 · Wails v2.16.0 · Node v26.9.0 · ffmpeg 8.0.1 · git 2.46.0**

| 层 | 选型 | 关键约束 |
|---|---|---|
| 桌面壳 | **Wails v2**（不是 v3） | 用系统 WebView2；**不要升 v3** |
| 视频 | **ffmpeg，`os/exec` 调用** | ⚠️ **不要引入 cgo 绑定**（goav/astiav） |
| B站取流 | **`yt-dlp.exe` sidecar** | **不要自己写 B站 API 客户端**——见下 |
| 持久化 | SQLite（`modernc.org/sqlite`） | ⚠️ **不要 `mattn/go-sqlite3`**（要 cgo） |
| 构建脚本 | **Node ≥23.6 + TypeScript** | 无构建步骤，直接 `node xxx.ts` |
| Go 依赖 | **标准库 + Wails + `golang.org/x/text`** | 加第三方前先问「标准库能不能做」；x/text 是目前唯一的例外 —— 曲名归一化要 NFKC/NFD，标准库没有（见 `docs/设计文档.md` §3） |
| 成绩图文字 OCR | **PP-OCRv6 small ONNX 本地 sidecar** | 构建到 `build/bin/ocr/`，运行时本地调用；Go 不引入 cgo，不启动 HTTP 服务 |

成绩图的文字只走 ONNX 识别；旧字形模板和对应测试已删除。图片导入缺少本地 OCR 组件时应明确报错。

### B站取流：yt-dlp sidecar（2026-09 实测选定）

**不要自己写 B站 API 客户端。** 实测未签名的 `x/player/playurl` 只返回 480p/360p；
yt-dlp 自己做 WBI 签名才拿得到 1080p30。而 `x/space/wbi/arc/search` **即使签名完整也报 `-352 风控`**。
**自己写 = 每次 B站 改风控都要发应用新版**，直接违反架构决策 6。

选它而不选 BBDown（已归档）/ lux（两年没发版）/ you-get（停更 20 个月）——
判据是**「B站 改接口时谁先跟上」**，不是 star 数。**yt-dlp 不入安装包，当前由用户下载并在设置中配置路径**。

调用要点：`--ffmpeg-location` 必须显式指定 · 选 `vcodec^=avc1`（H.264）· 进度按 `PROG|` 前缀解析 ·
**限速，50 首串行，不要并发轰炸**。

手动 YouTube 来源仍走 yt-dlp；完整支持需要 JS 运行时。当前下载命令启用 Node，P5 打包时需解决用户机器无 Node 的情况。

### ✅ 已裁决：运行时获取谱面录像（方案 A）—— 铁律 7 的唯一例外

配套四件事**缺一不可**：

1. **本地缓存**：缓存键 `hash(song_id + difficulty + 来源链接 + full)`，下过就不再下
2. **失败降级**：`video.sources[]` 多来源（871 条 / 220 个谱面多来源），主源失效换下一个
3. **手动兜底**：必须保留「手动粘 B站或 YouTube 视频链接 / 选本地文件」的入口（见 `docs/设计文档.md` §4：索引来自单个 UP 主）
4. **如实署名**：UI 与片尾必须列出发布者 + 平台视频 ID + 链接（铁律 8）

> **只开这一个口子。** 曲库、定数、曲绘、OP 音频由自托管资源包更新；OCR 模型随安装包提供。FFmpeg、FFprobe、yt-dlp 由用户自行配置，不从第三方站点静默下载。

### ❌ 不需要 Web 框架

Wails 不是 C/S —— Go 进程就是后端，前端通过**自动生成的 Bindings** 直接调 Go 函数。
加 Gin/Echo 的代价：开本地端口（任何本机进程都能打你的 API）、丢掉类型安全、端口冲突、
防火墙弹窗、CORS —— **收益为 0**。

概念对应：路由→`app.Bind()`；SSE/WebSocket→`runtime.EventsEmit`；静态文件→`assetserver`。
**真正要写的是任务编排层**（JobManager：goroutine + context 取消 + 进度推送 + SQLite 落库）。

---

## 目录结构

```
AGENTS.md / .gitignore / main.go / wails.json / go.mod
internal/                  Go 代码（用户手写）
  app/                    Wails bindings：工具、导入、校对、生成任务
  catalog/                曲目元数据只读查询 + JacketPath
    testdata/             单测夹具（专撞 nullable 边界）
  resource/               按 catalog 取素材（背景清单 / 画幅判定 / 录像缓存）
  tools/                  定位 ffmpeg / ffprobe / Edge / yt-dlp
  render/                 调 ffmpeg + Edge 渲染
    testdata/             从真实运行捕获的 golden argv
  score/                  成绩契约 / 校验 / 自检①–⑦ / ptt 口径
  ocr/                    b50 成绩图识别（网格 / ONNX 文字识别 / 字段解析）
  importer/               成绩图与手抄 JSON → score.Project（实现 score.Provider）
  job/                    渲染编排 + 依赖注入
  store/                  SQLite 持久化占位目录，尚未实现
cmd/pipeline/             ★ 端到端 CLI（不参与 wails build）
sidecar/ocr/              PP-OCRv6 small ONNX 文字识别程序、固定版本依赖和测试
samples/                  sample-project.json（P1 的输入契约）
frontend/                 React 19 + Vite 7（导入、感想、识别、编排、生成、导出、设置）
templates/                ★ 成片模板：bg16-9.png / bg4-3.png / text.html / 字体
build/                    Wails 打包资源（是源码，要提交）；build/bin/ 是产物，忽略
docs/                     设计文档.md + *-research.md / 音游成绩数据获取方式调研.md（**都不入库**）
scripts/build-ocr.ts      构建本地 OCR 组件（产物在 build/bin/ocr）
资源构建脚本和别名表已迁至独立仓库 IzayoiSakuye/arcaea-b50-resources；本仓库不包含 resource/。
data/                     应用当前读取的数据目录，不进版本库
  arcaea-catalog.json     ⭐ 应用唯一读取的目录（564 曲 / 1849 谱面）
  arcaea-backgrounds.json 曲目→背景映射（权威）
  arcaea-songdb.json / arcaea-index.json / arcaea-jackets.json   原始层
  jackets/256/*.jpg       620 张 256px 曲绘
  backgrounds/960/*.jpg   147 张 960×540 背景
dist/                     发布包产物（不进版本库）
.cache/                   开发及 CLI 缓存（不进版本库）；桌面应用运行缓存位于用户缓存目录
  ocr-venv/               OCR 构建用的隔离 Python 环境（不入发布包）
  ocr-onnx-model/         官方 PP-OCRv6 small 模型缓存（构建时校验 sha256）
  clips/                  完整录像缓存（按曲目、难度、来源链接生成哈希文件名）
  pipeline/               cmd/pipeline 的工作区（每段一个目录）
```

---

## 数据层

### 构建管线（在独立资源仓库执行）

```bash
node scripts/build-song-db.ts        # ① 曲库       联网  ~1s
node scripts/build-arcaea-index.ts   # ② 视频索引    联网  冷跑 ~180s
node scripts/build-jackets.ts        # ③ 曲绘+哈希   联网  冷跑 ~208s / 热跑 ~2s
node scripts/build-backgrounds.ts    # ④ 游玩背景    联网  冷跑 ~100s
node scripts/build-catalog.ts        # ⑤ 合并目录    本地  0s    ← 应用读这个
node scripts/report-arcaea-index.ts  # ⑥ 质量报告
node scripts/pack-data.ts            # ⑦ 打包发布
node scripts/build-release.ts --version YYYY.MM.DD # 一键发布准备
```

**① ② ③ ④ 是原始层，⑤ 是合并层。应用读取已安装的曲库，更新时访问自托管分发源。**

**⚠️ ④ 读的是上一轮 ⑤ 的产物** —— 改了 ⑤ 之后要再跑一轮 ④⑤ 才收敛。

### 权威来源

| 数据 | 来源 |
|---|---|
| **曲库主源** | wiki `Template:Songlist.json`（554 首，游戏原始数据，官方 song_id） |
| 定数 / 物量 / 时长 | wiki `Template:ChartConstant` / `ChartNotes` / `SongDuration` |
| 曲绘 | wiki 文件页：`Songs_<id>.jpg`，难度变体 `Songs_<id>_<难度>.jpg` |
| 游玩背景 | wiki `Template:Songlist.json`（含 `bg`/`bg_inverse`/`side`） |
| 中文别名 / 曲包显示名 | arcsong.json（GitHub）—— **唯一还依赖 GitHub 的两项，拉不到不致命** |
| 视频素材 | B站「谱面保管室」合集（索引由标题正则构建，非人工审核） |

> **2026-09 已把曲库主源从 2023 年的 GitHub 仓库切到 wiki** —— 可解析曲目 335 → 553，
> 「元数据不全」的曲目 198 → **11**。踩过的坑见独立资源仓库的 `README.md`。
> ⚠️ GitHub 那两份仍停在 2023 年，只作补充与交叉校验。

### song_id 三级对齐

索引里的 `song_id` 从曲名推导（`deriveSongId`），官方 ID 有大量无法自动推导的改写：
`Aleph-0 → alephzero`、`Alone & Lorn → aloneandlorn`、`Ashen 6oundary → ashenboundary`。

对齐顺序：**① 精确命中 → ② 互为前缀（≥6 字符）→ ③ 资源仓库的人工映射表 `data/song-id-aliases.json`**。

---

## 🚨 铁律（违反会破坏项目）

1. **绝不提交本仓库 `data/` 下任何文件。**
   620 张二进制 + 15MB，刷新一次就在 git 历史里永久留下十几 MB。`.gitignore` 已配好，别改。

2. **绝不手工编辑 `data/` 下的任何 JSON。** 全是构建产物。
   要改数据 → 到独立资源仓库修改脚本或别名表 → 重跑管线并发布资源包。

3. **不要"简化"独立资源仓库 `scripts/lib/anubis-fetch.ts` 的三个约束**，每个都是实测踩出来的：
   必须锁 **TLS 1.2**（WAF 拦 TLS 1.3 指纹）· 必须做 **cookie 往返** · `redir` 必须是绝对 URL。

4. **抓取要限速。** wiki.arcaea.cn 是社区个人维基，不是 CDN。用 `--delay`，不要并发轰炸。

5. **绝不自动套用 song_id 别名建议。** `--suggest-aliases` 只产出**建议**，必须人工核对。
   **给一首曲配上别人的曲绘，比缺图糟糕得多。**

6. **改动独立资源仓库的标题解析逻辑后，必须在那里跑 `node scripts/build-arcaea-index.ts --self-test`。**
   曲名带方括号（`[X]`、`AI[UE]OON`）曾被解析器吃没，10 条回归用例就是为此存在的。

7. **应用运行时原则上不依赖任何外部站点。** 曲库、定数、曲绘、模型、工具链全部离线化或自托管。
   竞品的 manifest 地址现在已 404 —— 这就是单点依赖的下场。客户端要支持
   **多镜像源 + sha256 校验 + 失败降级用旧版本**。
   - ⚠️ **唯一例外：谱面录像**（无法自托管）。边界严格限定，**不许开第二个口子**。

8. **版权红线。** 曲绘 / 谱面录像 / 曲目音频都有版权。当前依据是「谱面保管室明确可非商业使用」
   + 曲绘来自社区维基。因此：**署名机制必须保留**（不能因为"麻烦"砍掉）·
   商业化前重新评估授权 · **不要打包游戏原版字体**（竞品仓库里有，是侵权项）。

---

## 开发约定

- **注释与文档一律用中文。**
- **注释写「为什么」，不写「是什么」——但要短。** 三五行说清踩坑经过就够，
  **不要写成长篇设计说明**（用户明确要求过）。长篇论证放 `docs/`。
- 注释，报错等语言要**准确精炼**，不要有太口语化的表述
- **宁可记录也不丢弃。** 解析失败、数据冲突、无法归类的条目，一律进
  `unparsed` / `constant_candidates` / `unresolved` 并标注原因，**不要静默跳过**。
- **产物必须可重建、可验证。** 自己产出的二进制格式要用**独立工具**验一遍。
- **校验自己的产出。** 外部进程（Edge / ffmpeg）会「退出码 0 但什么都没写」——
  渲染完必须 `os.Stat` 产物，否则失败会静默到下游才炸成一句看不懂的错。
- **传给外部进程的路径一律绝对化。** `filepath.Join` 会 Clean 掉 `.` 段，
  所以 flag 默认值 `"."` 出来仍是相对路径 —— Edge 拿相对路径会直接失败。
- **构建脚本零第三方依赖**，只用 Node 标准库。**Go 侧只用标准库 + Wails**（唯一例外 `golang.org/x/text`，理由见技术栈表）。
- **脚本用 TypeScript，由 Node 直接跑**（原生类型擦除）。三条硬约束：
  只能写**可擦除语法**（无 `enum`/`namespace`/参数属性/装饰器）·
  相对 import **必须写 `.ts` 扩展名** · `tsc` 只是检查工具，不进运行链路。
- ⚠️ **agent 会话里 `NODE_ENV=production`**（会话注入，不在用户/系统变量里）——
  npm 会因此跳过 devDependencies，报 "up to date" 却什么都不装。
  **agent 跑 npm 要加 `--include=dev`；用户自己终端里不受影响。**
- 独立资源仓库的破坏性变更要同步更新其 `README.md` 与相关 `--help`。
- **不要 `git add` / `commit` / `push`** —— 用户手动验收。
- 写注释**简单**一点，不要添加过多agent自身的思考文本、docs文本与后端细节，只要说清楚是干什么的就行
- 遇到不确定的部分要询问用户，不要自作主张
- 每次写完一部分以后，一定要说明改动文件，以及新增方法的思路、逻辑、为什么这样做以及怎么做的
- **当且仅当你的身份（Skills）是pm-master**：你可以在用户决定开始编码时调用subagent并为其发送提示词进行编码等操作，但所有agent都要遵循该文档

---

## 常用命令

```bash
# Go
go build ./...            # ⚠️ 必须带 /... —— 只 go build 不编译 main 没引用的包
go vet ./...              # ⚠️ 格式串错误（%d 打在 string 上）只有 vet 抓得到
go test ./... -count=1
go run ./cmd/pipeline --limit=1 --force     # 只渲一首，调试主力

# 数据管线（在独立资源仓库根目录运行；改数据逻辑后全跑一遍）
node scripts/build-song-db.ts && node scripts/build-arcaea-index.ts --quiet \
  && node scripts/build-jackets.ts --quiet && node scripts/build-backgrounds.ts --quiet \
  && node scripts/build-catalog.ts

node scripts/build-backgrounds.ts --coverage   # 背景映射覆盖率（不下载）
node scripts/build-arcaea-index.ts --self-test # 解析器回归（改解析逻辑后必跑）
node scripts/report-arcaea-index.ts            # 质量报告
node scripts/check-ptt-formula.ts              # 潜力值公式自检（改公式 / 曲库刷新后必跑）
node scripts/check-normalize.ts                # 归一化落差审计（改归一化规则后必跑）
node scripts/pack-data.ts --emit-tree --base-url https://<域名>/arcaea
```

资源构建缓存位于独立资源仓库的 `.cache/`；桌面应用运行缓存位于用户缓存目录。需要强制刷新资源时加 `--refresh`。

---

## 文档索引

| 文档 | 内容 |
|---|---|
| `docs/设计文档.md` | ★★ **唯一设计文档**：产品定义、架构决策、数据层、渲染管线、成绩层契约与自检、OCR、界面、门禁与验收 |
| `docs/ui设计.md` / `docs/设计图/` | 七页界面细则与视觉参考；与设计文档冲突时以设计文档 §8 为准 |
| `internal/render/testdata/README.md` | golden argv 怎么捕获、**两份 golden 的区别**、翻译时的坑 |
| 独立资源仓库的 `README.md` | 脚本用法、Anubis 过盾、曲绘命名规律、数据分发策略 |
| `docs/*-research.md` | 调研原始报告（结论以 `设计文档.md` 为准） |

> ⚠️ **`docs/` 与 `_research/` 不入库是「有意为之」**，不是漏配。
> 后果：克隆这个仓库**拿不到任何设计文档**。

---

## 关键架构决策（已定型，不要重新讨论）

1. **不做实时爬取，走索引库。** 素材获取抽象成可替换的 `ResourceProvider`：
   `索引库 → 本地渲染 → 实时搜索（兜底）`。

2. **模板 = HTML/CSS，WebView2 就是渲染器。** 同一套模板同时服务实时预览、导出 PNG、最终合成。
   用户自定义模板 = 写 HTML。

3. **两阶段渲染 + concat demuxer。** 逐曲用**完全统一的编码参数**产出中间文件，
   最后 `-f concat -c copy` 无损秒拼。收益：断点续跑、增量重渲染、单段预览。

4. **时长锚点**：默认 10s/首 ≈ 9 分钟；逐首调整展示时长后，总片长与渲染耗时随之变化。另有精华版 ≈ 90 秒。

5. **Arcaea 成绩图导入使用 OCR。** 只处理 Arcaea Best50，
   且必须「**曲绘 pHash 定曲目 + OCR 读文字 + 自检①–⑦**」。
   已定形的做法（2026-10）：**只支持 Yurisaki 一种图源** → 坐标量死，不做版式识别；
   曲绘匹配不确定时，OCR 曲名仅在唯一精确命中曲库且属于曲绘候选时用于纠正；高置信冲突提示人工校对。所有文字行用随包离线 ONNX 模型。

6. **数据与代码分离发布。** 应用发版与曲库更新是两条独立流水线 ——
   游戏加新曲时重跑数据管线 + 打包即可，**用户不用更新应用**。

7. **谱面录像运行时按链接取流，本地缓存**（铁律 7 的唯一例外）。索引默认 B站，用户可手动指定 B站或 YouTube 视频页。
   用 `yt-dlp.exe` sidecar，不自研 B站 API 客户端。配套四件事见「技术栈」。
