# AGENTS.md

> 注入给每个 agent 的工作区上下文。**先读它，再读 `docs/00-方案讨论.md`。**
> 详细论证在 `docs/`，这里只放「必须知道的结论」与「不可违反的规则」。

---

## 项目

**为音游玩家自动生成成绩视频的桌面应用**：用户提供 best50/40，工具自动配齐素材、按模板合成一段可投稿到 B站的视频。

| 项 | 内容 |
|---|---|
| 首个游戏 | **Arcaea**（后续扩 osu! → Phigros → maimai/CHUNITHM） |
| 形态 | C 端桌面应用，**Windows 优先**（不是 Web 服务） |
| 定位 | 高质量爱好者工具 / 技术作品集，**不是创业项目** |

**核心差异化（竞品实测）**：50 段渲染 ≥60 分钟 → **≤10 分钟**；安装包 486MB → **≤70MB**；
峰值内存 OOM → ≤2GB；新增主题改代码 → **写 HTML/CSS**；支持的游戏数 → **多三个**。

---

## 当前状态（2026-10）

| 阶段 | 状态 |
|---|---|
| 方案设计 | ✅ `docs/00-方案讨论.md`（§0–§13 + 附录） |
| 数据层（曲库/索引/曲绘/背景/合并 + 打包） | ✅ `scripts/` → `data/` |
| P0 地基 | ✅ |
| 阶段 1–4：`catalog` / `tools` / `render` / `resource` + `cmd/pipeline` | ✅ **端到端跑通** |
| 阶段 5 性能实测 | ✅ **50 段外推 6.3 分钟**（预算 ≤10） |
| 阶段 6 收尾 | ✅ spike 已删（`b49a19c`）· 从零重跑通过 · G1 材料已写 |
| **🚦 门禁 G1** | ✅ **已过**（2026-10，多数玩家说想要） |
| **P2 成绩层** | 🔄 契约 / 校验 / 六条自检 / 编排注入 **已完成**（`score` + `job`）；b50 图识别：网格 + 曲绘 pHash **49/50**、**一格里定数/单曲ptt/分数/通关类型 50/50 全对**（`ocr`）；下一步：判定行（另一套更小字体）→ 「图 → Project」适配层，规格见 `docs/04`、`docs/05` |

**验收已过的四条**：`go build/vet/test` 干净 · 每段 ffprobe 参数一致且 `concat -c copy` 成功 ·
单段耗时 ×50 ≤10 分钟 · **运行时不写 `data/`** · 产物幂等（输入指纹 + `.part` 原子改名）。

**性能实测（3 首，缓存命中）**：

```
composite   6.39s/段   85%   ← 瓶颈，动它就动这里
text        0.96s/段   13%
background  0.18s/段    2%
concat      0.5s/次
50 段外推 ≈ 6.3 分钟 ✅
```

> ⚠️ **冷启动最慢**：`.cache/edge-profile/` 不存在时首轮 text 涨到 1.55s/段，外推 **6.8 分钟**，
> 之后回落到 0.96s/段。两个数都在预算内，但报数时要说清是冷还是热。
> 真超预算时第一个该动的是 `-preset`（`medium` → `veryfast`，快 2–3 倍、文件大 10–20%）。

**G1 门禁**（`docs/01-开发流程.md` §3.7）：把成品给 **5–10 个 Arcaea 玩家**看，
问「你会想要一个吗？」**多数人说想要 → 进 P2；反应平淡 → 停下来重新想产品形态。**

> ✅ **2026-10 已过**：多数玩家说想要。反馈里唯一的**具体短板是「成绩录入太麻烦 / 要能直接导入截图」**
> —— 所以 P2 的顺序是**骨架优先**（导入 → 校对 → 出片），OCR 押后；
> 且校对表要能在录入时**实时跑六条自检**，把"打错数字"当场暴露（见 `docs/04-P2实现规格.md` §0）。
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
| Go 依赖 | **标准库 + Wails + `golang.org/x/text`** | 加第三方前先问「标准库能不能做」；x/text 是目前唯一的例外 —— 曲名归一化要 NFKC/NFD，标准库没有（见 `docs/05` §6.1） |

### B站取流：yt-dlp sidecar（2026-09 实测选定）

**不要自己写 B站 API 客户端。** 实测未签名的 `x/player/playurl` 只返回 480p/360p；
yt-dlp 自己做 WBI 签名才拿得到 1080p30。而 `x/space/wbi/arc/search` **即使签名完整也报 `-352 风控`**。
**自己写 = 每次 B站 改风控都要发应用新版**，直接违反架构决策 6。

选它而不选 BBDown（已归档）/ lux（两年没发版）/ you-get（停更 20 个月）——
判据是**「B站 改接口时谁先跟上」**，不是 star 数。体积 17 MB，**首启按需下载，不入安装包**。

调用要点：`--ffmpeg-location` 必须显式指定 · 选 `vcodec^=avc1`（H.264）· 进度按 `PROG|` 前缀解析 ·
**限速，50 首串行，不要并发轰炸**。

### ✅ 已裁决：运行时从 B站 取流（方案 A）—— 铁律 7 的唯一例外

配套四件事**缺一不可**：

1. **本地缓存**：缓存键 `hash(song_id + difficulty + bvid + p + 起止时间)`，下过就不再下
2. **失败降级**：`video.sources[]` 多来源（871 条 / 220 个谱面多来源），主源失效换下一个
3. **手动兜底**：必须保留「手动粘 BV 号 / 选本地文件」的入口（§13.6 风险 1：索引来自单个 UP 主）
4. **如实署名**：UI 与片尾必须列出 UP 主 + BV 号 + 链接（铁律 8）

> **只开这一个口子。** 曲库、定数、曲绘、OCR 模型、ffmpeg、yt-dlp 本体一律离线化或自托管。

### ❌ 不需要 Web 框架

Wails 不是 C/S —— Go 进程就是后端，前端通过**自动生成的 Bindings** 直接调 Go 函数。
加 Gin/Echo 的代价：开本地端口（任何本机进程都能打你的 API）、丢掉类型安全、端口冲突、
防火墙弹窗、CORS —— **收益为 0**。

概念对应：路由→`app.Bind()`；SSE/WebSocket→`runtime.EventsEmit`；静态文件→`assetserver`。
**真正要写的是任务编排层**（JobManager：goroutine + context 取消 + 进度推送 + SQLite 落库）。

---

## 目录结构

```
AGENTS.md / .gitignore / app.go / main.go / wails.json / go.mod
internal/                  Go 代码（用户手写）
  catalog/                曲目元数据只读查询 + JacketPath
    testdata/             单测夹具（专撞 nullable 边界）
  resource/               按 catalog 取素材（背景清单 / 画幅判定 / 录像缓存）
  tools/                  定位 ffmpeg / ffprobe / Edge / yt-dlp
  render/                 调 ffmpeg + Edge 渲染
    testdata/             从真实运行捕获的 golden argv
  score/                  成绩契约 / 校验 / 六条自检 / ptt 口径
  ocr/                    b50 成绩图识别（网格 / 曲绘 pHash 定曲 / 字形模板认数字与通关类型）
  job/                    渲染编排 + 依赖注入
  store/                  ⬜ 尚未实现（P3 再上 SQLite）
cmd/pipeline/             ★ 端到端 CLI（不参与 wails build）
samples/                  sample-project.json（P1 的输入契约）
frontend/                 React 19 + Vite 7（最小壳，只调 Version() 验证 bindings）
templates/                ★ 成片模板：bg16-9.png / bg4-3.png / text.html / 字体
build/                    Wails 打包资源（是源码，要提交）；build/bin/ 是产物，忽略
docs/                     00-方案讨论.md / 01-开发流程.md / 02-P1实现规格.md / 03-G1门禁材料.md / 04-P2实现规格.md / 05-validate规格.md（**都不入库**）
scripts/                  构建期工具（TS，零依赖，不参与运行时）
  lib/types.ts            数据契约
  lib/anubis-fetch.ts     Anubis PoW 反爬客户端（三个必读的坑）
  lib/wiki.ts             wiki JSON 页面客户端
  build-song-db.ts        ① 曲库（主源 = wiki 的 4 个 JSON 页面）
  build-arcaea-index.ts   ② 视频索引（含 --self-test）
  build-jackets.ts        ③ 曲绘 + 感知哈希
  build-backgrounds.ts    ④ 背景图 + 曲目→背景映射（含 --coverage）
  build-catalog.ts        ⑤ 合并成应用唯一读取的目录
  report-arcaea-index.ts  ⑥ 质量报告
  pack-data.ts            ⑦ 打包发布
  check-ptt-formula.ts    § 潜力值公式自检（**不产数据**；Go 版公式的对拍 oracle）
  check-normalize.ts      § 归一化落差审计（Go 复刻 TS 归一化会漏几首 → 决定引不引 x/text）
data/                     ⚠️ 构建产物，不进版本库（例外：song-id-aliases.json）
  arcaea-catalog.json     ⭐ 应用唯一读取的目录（564 曲 / 1849 谱面）
  arcaea-backgrounds.json 曲目→背景映射（权威）
  arcaea-songdb.json / arcaea-index.json / arcaea-jackets.json   原始层
  jackets/256/*.jpg       620 张 256px 曲绘
  backgrounds/960/*.jpg   147 张 960×540 背景
dist/                     发布包产物（不进版本库）
.cache/                   管线 + 渲染缓存（不进版本库，可安全删除）
  clips/                  录像片段缓存（<song_id>.mp4）
  pipeline/               cmd/pipeline 的工作区（每段一个目录）
```

---

## 数据层

### 构建管线

```bash
node scripts/build-song-db.ts        # ① 曲库       联网  ~1s
node scripts/build-arcaea-index.ts   # ② 视频索引    联网  冷跑 ~180s
node scripts/build-jackets.ts        # ③ 曲绘+哈希   联网  冷跑 ~208s / 热跑 ~2s
node scripts/build-backgrounds.ts    # ④ 游玩背景    联网  冷跑 ~100s
node scripts/build-catalog.ts        # ⑤ 合并目录    本地  0s    ← 应用读这个
node scripts/report-arcaea-index.ts  # ⑥ 质量报告
node scripts/pack-data.ts            # ⑦ 打包发布
```

**① ② ③ ④ 是原始层，⑤ 是合并层。应用只读 ⑤ 的产物，运行时零网络依赖。**

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
> 「元数据不全」的曲目 198 → **11**。踩过的坑见 `scripts/README.md`。
> ⚠️ GitHub 那两份仍停在 2023 年，只作补充与交叉校验。

### song_id 三级对齐

索引里的 `song_id` 从曲名推导（`deriveSongId`），官方 ID 有大量无法自动推导的改写：
`Aleph-0 → alephzero`、`Alone & Lorn → aloneandlorn`、`Ashen 6oundary → ashenboundary`。

对齐顺序：**① 精确命中 → ② 互为前缀（≥6 字符）→ ③ 人工映射表 `data/song-id-aliases.json`**。

---

## 🚨 铁律（违反会破坏项目）

1. **绝不提交 `data/` 下除 `song-id-aliases.json` 外的任何文件。**
   620 张二进制 + 15MB，刷新一次就在 git 历史里永久留下十几 MB。`.gitignore` 已配好，别改。

2. **绝不手工编辑 `data/` 下的任何 JSON。** 全是构建产物。
   要改数据 → 改 `scripts/` 或 `song-id-aliases.json` → 重跑管线。

3. **不要"简化" `scripts/lib/anubis-fetch.ts` 的三个约束**，每个都是实测踩出来的：
   必须锁 **TLS 1.2**（WAF 拦 TLS 1.3 指纹）· 必须做 **cookie 往返** · `redir` 必须是绝对 URL。

4. **抓取要限速。** wiki.arcaea.cn 是社区个人维基，不是 CDN。用 `--delay`，不要并发轰炸。

5. **绝不自动套用 song_id 别名建议。** `--suggest-aliases` 只产出**建议**，必须人工核对。
   **给一首曲配上别人的曲绘，比缺图糟糕得多。**

6. **改动标题解析逻辑后必须跑 `node scripts/build-arcaea-index.ts --self-test`。**
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
- 破坏性变更要同步更新 `scripts/README.md` 与相关 `--help`。
- **不要 `git add` / `commit` / `push`** —— 用户手动验收。
- 写注释**简单**一点，不要添加过多agent自身的思考文本、docs文本与后端细节，只要说清楚是干什么的就行
- 遇到不确定的部分要询问用户，不要自作主张
- 每次写完一部分以后，一定要说明改动文件，以及新增方法的思路、逻辑、为什么这样做以及怎么做的

---

## 常用命令

```bash
# Go
go build ./...            # ⚠️ 必须带 /... —— 只 go build 不编译 main 没引用的包
go vet ./...              # ⚠️ 格式串错误（%d 打在 string 上）只有 vet 抓得到
go test ./... -count=1
go run ./cmd/pipeline --limit=1 --force     # 只渲一首，调试主力

# 数据管线（改任何数据相关逻辑后全跑一遍）
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

缓存位于 `.cache/`，重跑很快。需要强制刷新时加 `--refresh`。

---

## 文档索引

| 文档 | 内容 |
|---|---|
| `docs/00-方案讨论.md` | ★ **主文档**：设计评审、可行性、架构与流水线、性能预算、数据契约、界面结构、Arcaea 专项、竞品逆向 |
| `docs/01-开发流程.md` | ★ **路线图**：P0–P6 阶段、**门禁**、风险回退、分工 |
| `docs/02-P1实现规格.md` | ★ **P1 实现规格**：阶段 0–6，含接口签名、锁定常量、golden test 方法、验收点 |
| `docs/03-G1门禁材料.md` | ★ **G1 材料**：开场白、判定标准、耗时/覆盖度数据、**怎么问才不问坏** |
| `docs/04-P2实现规格.md` | ★ **P2 实现规格**：成绩层契约、字段权威性、六条自检、编排搬迁、验收点（**§2 三个前提已裁决**） |
| `docs/05-validate规格.md` | ★ **validate 规格**：阻断路径、Severity 四档、三级曲目定位、**归一化跨语言缺口**、夹具与验收点 |
| `internal/render/testdata/README.md` | golden argv 怎么捕获、**两份 golden 的区别**、翻译时的坑 |
| `scripts/README.md` | 脚本用法、Anubis 过盾、曲绘命名规律、数据分发策略 |
| `docs/*-research.md` | 调研原始报告（部分结论已被主文档修正，**冲突时以主文档为准**） |

> ⚠️ **`docs/` 与 `_research/` 不入库是「有意为之」**，不是漏配。
> 后果：克隆这个仓库**拿不到任何设计文档**，本文里「见 §13.6」之类的引用在仓库内无法跳转。

---

## 关键架构决策（已定型，不要重新讨论）

1. **不做实时爬取，走索引库。** 素材获取抽象成可替换的 `ResourceProvider`：
   `索引库 → 本地渲染 → 实时搜索（兜底）`。

2. **模板 = HTML/CSS，WebView2 就是渲染器。** 同一套模板同时服务实时预览、导出 PNG、最终合成。
   用户自定义模板 = 写 HTML。

3. **两阶段渲染 + concat demuxer。** 逐曲用**完全统一的编码参数**产出中间文件，
   最后 `-f concat -c copy` 无损秒拼。收益：断点续跑、增量重渲染、单段预览。

4. **时长锚点**：标准版 10s/首 ≈ 9 分钟；另有精华版 ≈ 90 秒。

5. **OCR 是降级路径，不是主路径。** 头部游戏一次 API 就拿全 B50；Arcaea 无 API 才走 OCR，
   且必须「**曲绘 pHash 定曲目 + OCR 只读数字** + 六条一致性自检」。
   已定形的做法（2026-10）：**只支持 Yurisaki 一种图源** → 坐标量死，不做版式识别；
   **曲名不认**（pHash 定曲，曲名从曲库取）；数字用**字形模板**匹配，不引入 AI 模型。

6. **数据与代码分离发布。** 应用发版与曲库更新是两条独立流水线 ——
   游戏加新曲时重跑数据管线 + 打包即可，**用户不用更新应用**。

7. **谱面录像运行时按 BV号 取流，本地缓存**（铁律 7 的唯一例外）。
   用 `yt-dlp.exe` sidecar，不自研 B站 API 客户端。配套四件事见「技术栈」。
