# AGENTS.md

> 这份文件会注入给每个 agent 作为工作区上下文。**先读它，再读 `docs/00-方案讨论.md`。**
> 详细论证在 `docs/`，这里只放「必须知道的结论」与「不可违反的规则」。

---

## 项目

**为音游玩家自动生成成绩视频的桌面应用**：用户提供 best50/40 成绩，工具自动配齐素材、按模板合成一段可投稿到 B 站的视频。

| 项 | 内容 |
|---|---|
| 首个目标游戏 | **Arcaea**（后续扩 osu! → Phigros → maimai/CHUNITHM） |
| 形态 | C 端桌面应用，**Windows 优先**（不是 Web 服务） |
| 目标用户 | 愿意把成绩做成视频投稿的音游玩家（小众、付费意愿低） |
| 定位 | 高质量爱好者工具 / 技术作品集，**不是创业项目** |

**核心差异化（来自竞品实测）**：

| 维度 | 竞品（mai-gen-videob50，230★） | 本项目目标 |
|---|---|---|
| 50 段渲染耗时 | ≥60 分钟（无 GPU 3 小时+） | **≤10 分钟** |
| 安装包 | **486MB**（双 zip 手动合并） | **≤70MB 单包** |
| 峰值内存 | 16GB 机器 OOM | ≤2GB |
| 新增主题 | 改 Python 代码 | 写 HTML/CSS |
| 支持的游戏 | maimai / CHUNITHM / Ongeki | **+ Arcaea / osu! / Phigros（全无竞品）** |

---

## 当前状态

| 阶段 | 状态 | 位置 |
|---|---|---|
| 可行性调研 + 方案设计 | ✅ 完成 | `docs/00-方案讨论.md`（§0–§13 + 附录 A + 参考来源） |
| 数据层（曲库 / 视频索引 / 曲绘 / 合并目录） | ✅ 完成 | `scripts/` + `data/` |
| 数据发布打包 | ✅ 完成 | `scripts/pack-data.ts` → `dist/` |
| 应用脚手架（Go / Wails / React） | 🟡 **只有模板脚手架，零业务代码** | `app.go` `main.go` `frontend/` |
| 版本控制 | 🟡 **已 `git init`，但 0 个提交** | 见下方说明 |
| 下一步 | **M0 · Arcaea 版**：见 §13.8 | 见 §13.8 + §8 M0 |

**脚手架的真实状态（别被"有 app.go"骗了）**：`app.go` 里只有一个 `Greet(name)` 的 Wails 模板 demo；`main.go` 是模板默认的 1024×768 窗口；`frontend/src/App.jsx` 是模板默认页；`frontend/node_modules` **尚未安装**。**业务代码一行都没有。**

**⚠️ 版本控制说明**：仓库已初始化（分支 `master`），但**一次提交都还没有** —— 也就是说铁律 1「绝不提交 `data/`」虽然有了 `.gitignore` 兜着，却**还没有被任何一次提交验证过**。首次提交前务必确认 `git status` 里 `data/` 下**只有** `song-id-aliases.json`。

**⚠️ `docs/` 与 `research/` 不入库是「有意为之」，不是漏配。** `.gitignore` 里那两行是显式决定，别当成配置错误去"修"它。后果要说清楚：克隆这个仓库**拿不到任何设计文档**（含 `00-方案讨论.md` 这个主文档），`AGENTS.md` 里那些「见 §13.6」的引用在仓库内无法跳转。

**M0 的两个版本（都保留，别搞混）**：

| 出处 | 内容 | 适用 |
|---|---|---|
| §8 M0（通用） | 手工挑 3 首 → 模板渲染 + PNG 图层 + ffmpeg overlay + concat → 产出成品视频 | 验证**技术管线**跑得通 |
| **§13.8 M0（Arcaea，优先）** | ① 收 3–5 张真实 best50 成绩图（AOL / 查分器 / 游戏内各一张）② 手工抄 50 行跑 §13.5 六条自检 ③ **做一段 90 秒动画** ④ 量渲染耗时 | 验证**需求是否真实存在** |

> §13.8 的第 3 步是关键：B站 证据显示 Arcaea 玩家目前只用**静态分表图**（§13.1.3），
> 这既可能"没人做过好的"、也可能"大家真的只要一张图"。**在投入 OCR 工程之前先问出这个问题。**

**数据层现状快照**（`data/arcaea-catalog.json`）：533 首曲目 / 1350 个谱面 / 1045 个定数 / 1044 个物量 / 645 个谱面有视频 / 507 首有曲绘与感知哈希。

---

## 技术栈

已确认可用：**Go 1.26.4 · Wails v2.16.0 · Node v26.9.0 · ffmpeg 8.0.1 · git 2.46.0**

| 层 | 选型 | 说明 |
|---|---|---|
| 桌面壳 | **Wails v2**（不是 v3） | 用系统 WebView2，不打包 Chromium；**不要升 v3**（演进中） |
| 后端 | Go | |
| 前端 | React + Vite | 跑在 WebView2 里 |
| 视频 | **ffmpeg，`os/exec` 调用** | ⚠️ **不要引入 cgo 绑定**（goav/astiav）—— 会让交叉编译与分发变噩梦 |
| B站取流 | **`yt-dlp.exe` sidecar（Bootstrap 按需下载，不入安装包）** | 见下方「为什么是 yt-dlp」。⚠️ **不要自己写 B站 API 客户端** |
| 持久化 | SQLite（`modernc.org/sqlite`，纯 Go） | ⚠️ **不要用 `mattn/go-sqlite3`**（需要 cgo） |
| 构建期脚本 | **Node ≥23.6 + TypeScript** | ⚠️ **无构建步骤** —— Node 原生类型擦除，`node xxx.ts` 直接跑。`tsc` 仅用于检查 |

### 为什么是 yt-dlp（2026-09 实测，不是查文档）

**结论：用 `yt-dlp.exe` 当 sidecar，走首启按需下载（E3 模式），不要自己实现 B站 API 客户端。**

| 候选 | 最新发版 | 许可证 | Windows 体积 | 仓库状态（2026-09-19 查） | 判断 |
|---|---|---|---|---|---|
| **yt-dlp** | 2026.08.19 | **Unlicense** | **17.01 MB**（`yt-dlp.exe`） | 192k★，**3 天前还有提交** | ✅ **选它** |
| BBDown | 1.6.3（2024-08-14） | MIT | 7.67 MB | 13.9k★，**仓库已 archived** | ❌ **正式归档停更**，且需 .NET 运行时 |
| lux | v0.24.1（2024-05-06） | MIT | 8.13 MB | 31.7k★，有提交（2026-03）但**两年多没发 release** | ❌ 要自己编译，无现成二进制 |
| you-get | v0.4.1743（2025-01-04） | MIT | 无独立 exe | 停更 20 个月 | ❌ 需 Python 运行时 |

> **判断依据不是"谁 star 多"，而是"B站 改接口时谁先跟上"。** B站 的 wbi / 风控 / 清晰度策略变化频繁，
> 选一个已归档或两年没发版的工具，等于把风险留给自己。yt-dlp 是唯一还在高频发版的。

**实测证据（全部本地跑过，BV1T54y1s7f6?p=3）**：

| 验证项 | 结果 |
|---|---|
| 精确选分P | ✅ `?p=3` → `p03 (11.2) Fracture Ray [FTR 11]`，时长 163.583s |
| **只取时间区间** | ✅ `--download-sections "*60-73"` → **只下了 6.83 MiB**（整个 1080p 视频是 49.49 MiB），**4.6 秒**完成，产出 13.233s / 1920×1080@30 / h264+aac |
| **1080p 要不要登录** | ✅ **不要**。免 cookie 就拿到 `30080 1920x1080 30fps`（**只有 1080P60 要大会员**） |
| CDN Range 支持 | ✅ `206 Partial Content` + `Accept-Ranges: bytes`，中段 seek 正常 |
| Go 侧解析 | ✅ `--print "%(...)j"` 输出 **ASCII 安全的 JSON**（`\uXXXX` 转义），绕开 Windows 控制台 GBK 代码页的乱码问题 |

> ⚠️ **关键坑：必须让 yt-dlp 做 WBI 签名，而你裸调 API 做不到。**
> 实测未签名的 `x/player/playurl` 只返回 **480p/360p**（`quality` 字段会骗你显示 64，但 `dash.video[]` 里没有高清）；
> yt-dlp 日志里明写 `Downloading wbi sign`，签完就能拿 1080p30。
> 而 §13.6 已经记录过：`x/space/wbi/arc/search` **即使签名完整也报 `-352 风控校验失败`**。
> **自己写客户端 = 每次 B站 改风控都要发应用新版**，直接违反架构决策 6（数据与代码分离发布）。

**调用规范**：

```bash
yt-dlp.exe \
  --no-warnings --newline \
  -f "bv*[height<=1080][vcodec^=avc1]+ba[ext=m4a]" \
  --download-sections "*60-73" \
  --ffmpeg-location "<随包/首启下载的 ffmpeg 路径>" \
  --merge-output-format mp4 \
  --progress-template "PROG|%(progress._percent_str)s|%(progress._speed_str)s|%(progress._eta_str)s" \
  -o "<cache>/clip.%(ext)s" \
  "https://www.bilibili.com/video/BV1T54y1s7f6?p=3"
```

- **`--ffmpeg-location` 一定要显式指定**，否则 yt-dlp 去 PATH 找 ffmpeg，用户机器上大概率没有。
- 选 `vcodec^=avc1`（H.264）而不是 HEVC —— HEVC 在部分机器上解码/编码链路更麻烦。
- 进度按行解析 `PROG|` 前缀，不要用 `--print` 的 JSON 去轮询。
- **限速**：单条视频发 2–4 个请求，50 首串行即可（竞品也是串行）。**不要并发轰炸 B站。**

### ✅ 已裁决：运行时从 B站 取流（方案 A）

**结论：应用运行时按 BV号 从 B站 取流，本地缓存；这是铁律 7 的唯一例外。**

为什么另两条路被否掉：

| 方案 | 否决理由 |
|---|---|
| B. 用户手动提供视频文件 | ❌ 50 首曲子要用户自己录/下 50 段 —— §13.1.2 已判定这是"没人用的高级功能"（触发率太低） |
| C. 自己托管切片资源包 | ❌ 需要你承担存储/带宽，且把「引用」变成「再分发」——**版权性质变了**，风险远大于收益 |

**方案 A 必须配套的四件事（缺一不可）**：

1. **本地缓存**：以 `hash(song_id + difficulty + bvid + p + 起止时间)` 为缓存键，**下过就不再下**。重复渲染 / 增量重渲染不该产生任何网络请求。
2. **失败降级**：catalog 里每个谱面已预存 `video.sources[]`（871 个来源 / 220 个谱面是多来源）。主源失效就换下一个，全部失效才标 `failed`。
3. **手动兜底**：必须保留「手动粘 BV 号 / 选本地文件」的入口（§13.6 风险 1：整个索引来自一个 UP 主，是单点依赖）。
4. **如实署名**：UI 与成片片尾必须列出素材来源（UP 主 + BV 号 + 链接）—— 见铁律 8。

> ⚠️ 这个例外的边界要说清楚：**只有「谱面录像」走运行时取流**。
> 曲库、定数、曲绘、OCR 模型、ffmpeg、yt-dlp 本体**一律离线化或走自托管资源包**，不许开第二个口子。

### ❌ 不需要 Web 框架

**不要引入 Gin / go-zero / Echo。** Wails 不是 C/S 架构 —— Go 进程就是后端，前端通过**自动生成的 Bindings** 直接调 Go 函数，没有 HTTP 层。

加 Web 框架的真实代价：开本地端口（**任何本机进程都能打你的 API**）、丢掉 bindings 的类型安全、端口冲突/防火墙弹窗/CORS —— **收益为 0**。

概念对应：路由→`app.Bind()` 的导出方法；SSE/WebSocket→`runtime.EventsEmit`；静态文件→`assetserver.Options{Middleware}`。

**真正需要写的是一个任务编排层**（JobManager：goroutine + `context` 取消 + 逐曲进度推送 + SQLite 落库），不是 HTTP 层。云端服务（若将来要做）用 `net/http` + `chi` 即可。

---

## 目录结构

```
AGENTS.md                  ← 本文件
.gitignore                 ← data/* 全忽略（仅放行 song-id-aliases.json）；docs/ 暂不入库（见下）
app.go                     🟡 Wails 脚手架：只有一个 Greet() demo，没有业务代码
main.go                    🟡 Wails 模板默认入口（1024×768 窗口）
wails.json                 Wails 配置（前端构建命令、输出文件名）
go.mod / go.sum            Go 1.25 模块（wails v2.16.0）
frontend/                  🟡 React 19 + Vite 7 模板默认页（node_modules 未安装）
  src/App.jsx              模板默认的 Wails logo 打招呼页
  wailsjs/                 自动生成的 Go↔JS bindings（勿手改）
build/                     Wails 打包资源（图标 / manifest / NSIS 脚本）
docs/
  00-方案讨论.md            ★ 主文档：设计评审/可行性/架构/数据契约/界面结构（§0–§13 + 附录 A）
  01-开发流程.md            ★ 开发路线图：P0–P6 阶段、门禁、风险回退、分工
  *-research.md            调研原始报告（部分结论已被主文档修正，以主文档为准）
scripts/                   ★ 构建期工具（TypeScript，零依赖，不参与应用运行时）
  package.json             `{"type":"module"}` + devDeps（typescript / @types/node，仅供检查）
  tsconfig.json            类型检查配置（strict）。⚠️ 只影响编辑器与 `npm run check`，运行时不读
  node_modules/            仅类型定义，git 忽略。删掉不影响脚本运行
  lib/types.ts             数据契约（各脚本间传递的 JSON 结构，改产出时同步改这里）
  lib/anubis-fetch.ts      Anubis PoW 反爬客户端（含三个必读的坑）
  lib/zip.ts               自写的 store-only ZIP 写入器
  build-song-db.ts        ① 曲库（⚠️ 上游停在 2023，待切 wiki）
  build-arcaea-index.ts   ② 视频索引（含 --self-test）
  build-jackets.ts        ③ 曲绘 + 感知哈希
  build-backgrounds.ts    ④ 游玩背景图 + 曲目→背景映射（含 --coverage）
  build-catalog.ts        ⑤ 合并成应用唯一读取的目录（导出归一化函数供 ④ 复用）
  report-arcaea-index.ts  ⑥ 质量报告
  pack-data.ts            ⑦ 打包成可发布的资源包
  spike-sample-video.ts   ⚠️ **临时** P1 样片管线（背景→视频→曲绘→装饰→文字→concat）
  README.md                ★ 脚本用法 + Anubis 过盾细节 + 数据分发策略 + 各类实测坑
data/                      ⚠️ 构建产物，不进版本库（例外见下）
  arcaea-catalog.json      ⭐ 应用唯一读取的目录（533 曲 / 1350 谱面）
  arcaea-backgrounds.json  曲目→背景映射（权威，来自 Songlist.json）
  arcaea-songdb.json       原始层：曲库（335 首，2023）
  arcaea-index.json        原始层：B站视频索引（650 谱面）
  arcaea-jackets.json      原始层：曲绘 + 感知哈希
  jackets/256/*.jpg        620 张 256px 曲绘（~12MB）
  backgrounds/960/*.jpg    140 张 960×540 背景（~9.5MB，已裁 16:9）
  song-id-aliases.json     ✅ 唯一进版本库的文件（人工配置）
dist/                      发布包产物（不进版本库）：zip + latest.json + tree/
.cache/                    管线缓存（不进版本库，可安全删除）
  backgrounds/src/         背景源图 1280px（~18MB）—— 换输出尺寸不用重新联网
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

### 权威来源与优先级

| 数据 | 来源 | 备注 |
|---|---|---|
| 成绩（运行时不涉及） | 查分器 API / OCR | 见 §3.5 数据源矩阵 |
| 视频素材 | B站「谱面保管室」合集 | 索引由标题正则构建，非人工审核 |
| 曲绘 + 官方 song_id | Arcaea 中文维基 | 文件名 `Songs_<song_id>.jpg` |
| **游玩背景** | wiki `Template:Songlist.json` | ⭐ **游戏原始数据**，含 `bg`/`bg_inverse`/`side`，零歧义 |
| 定数 | **wiki > songdb > video** | 汇编视频带的是发布当时的旧值，43 处已知分歧 |
| 物量 note | arcsong.json | 用于 OCR 判定数守恒校验 |

> ⭐ **wiki 上有 26 个 JSON 内容模型页面**（`contentmodel: json`），实测全部可取，
> 包括 `Songlist` / `ChartConstant` / `ChartNotes` / `SongDuration` / `ChartConstantHistory` 等。
> 其中 `Songlist`（554 首）与 `ChartNotes`（554 首，与现有值 **1041 一致 / 0 不一致**）
> **都已超过 2023 年的上游数据**（arcsong.json 335 首 / ChartConstant.json 318 首）。
> **把曲库主源切到 wiki 是既定方向，但会改动 song_id**，详见 `scripts/README.md`。

**⚠️ 上游曲库（arcsong.json / ChartConstant.json）停留在 2023 年**，不含 2024–2026 新曲。脚本每次运行都会打印告警。

### song_id 的三级对齐

索引里的 `song_id` 是从曲名推导的（`deriveSongId`），官方 ID 有大量无法自动推导的命名改写：

```
Aleph-0      → alephzero      数字写成单词
Alone & Lorn → aloneandlorn   & 写成 and
Ashen 6oundary → ashenboundary  标题里 6 是风格化的 b
```

对齐顺序：**① 精确命中 → ② 互为前缀（≥6 字符）→ ③ 人工映射表 `data/song-id-aliases.json`**。

---

## 🚨 铁律（违反会破坏项目）

1. **绝不提交 `data/` 下除 `song-id-aliases.json` 外的任何文件。**
   620 个二进制 + 15MB，每次刷新数据都会在 git 历史里永久留下约 13MB。
   `.gitignore` 已配好，**不要为了"方便"改它**。

2. **绝不手工编辑 `data/` 下的任何 JSON。** 它们全是构建产物。
   要改数据 → 改 `scripts/` 里的脚本或 `song-id-aliases.json` → 重跑管线。

3. **不要"简化" `scripts/lib/anubis-fetch.ts` 的三个约束**，每一个都是实测踩出来的：
   - 必须锁 **TLS 1.2**（WAF 拦截 Node 的 TLS 1.3 指纹，默认 `fetch` 一律 ECONNRESET）
   - 必须做 **cookie 往返**（挑战页的 verification cookie 要在提交答案时带回）
   - `pass-challenge` 的 **`redir` 必须是绝对 URL**

4. **抓取要限速。** wiki.arcaea.cn 是社区个人维基，不是 CDN。用 `--delay`，
   不要并发轰炸，不要把这个能力用于批量爬取。

5. **绝不自动套用 song_id 别名建议。** `--suggest-aliases` 只产出**建议**到
   `_suggestions` / `_pending`，**必须人工核对后**才挪进 `map`。
   给一首曲配上别人的曲绘，比缺图糟糕得多。

6. **改动标题解析逻辑后，必须跑 `node scripts/build-arcaea-index.ts --self-test`。**
   曲名带方括号（`[X]`、`AI[UE]OON`）曾经被解析器吃没，10 条回归用例就是为此存在的。
   **新增标题变体时同步补充用例。**

7. **应用运行时原则上不依赖任何外部站点。** 曲库、定数、曲绘、模型、工具链
   全部离线化或走自己托管的版本化资源包。竞品的 manifest 地址现在已 404 ——
   这就是单点依赖的下场，所以客户端要支持**多镜像源 + sha256 校验 + 失败降级用旧版本**。
   - ⚠️ **唯一例外：谱面录像。** 它无法自托管（版权 + 体积），只能运行时按 BV号
     从 B站 取流。**边界严格限定在「谱面录像」这一类素材上，不许开第二个口子。**
     配套四件事见「技术栈 → 已裁决：运行时从 B站 取流」。

8. **版权红线。** 音游的曲绘、谱面录像、曲目音频都有版权。当前依据是「谱面保管室
   明确可非商业使用」+ 曲绘来自社区维基。因此：
   - **署名机制（素材来源列表）必须保留**，不能因为"麻烦"砍掉
   - 商业化前必须重新评估素材授权
   - **不要打包游戏原版字体**（竞品仓库里有，是侵权项）

---

## 开发约定

- **注释与文档一律用中文。** 现有代码已建立该风格。
- **构建脚本零第三方依赖**，只用 Node 标准库。加依赖前先问「标准库能不能做」。
- **脚本用 TypeScript，但由 Node 直接跑**（原生类型擦除，Node ≥23.6）。三条硬约束：
  - **只能写「可擦除语法」** —— 不能有 `enum` / `namespace` / 构造函数参数属性 / 装饰器
    （这些要生成运行时代码）。类型标注、接口、泛型、`as`、`satisfies` 都可以。
  - **相对 import 必须写 `.ts` 扩展名**（`from './lib/zip.ts'`），Node 不会自动补。
  - **`tsc` 只是检查工具，不进运行链路**。`scripts/tsconfig.json` 只影响编辑器与
    `npm run check`；`node xxx.ts` 根本不读它，删掉 `node_modules` 脚本照样跑。
- **`scripts/` 的 devDependencies 装了 `typescript` + `@types/node`**，仅供类型检查。
  ⚠️ 本机环境变量 `NODE_ENV=production`，npm 会**默认跳过 devDependencies** —— 装的时候
  要加 `--include=dev`，否则它会报 "up to date" 却什么都不装（排查过一次）。
- **注释写「为什么」，不写「是什么」。** 参考 `lib/anubis-fetch.ts` 的注释密度 ——
  每个反直觉的写法旁边都要说明踩坑经过，否则后人会"顺手优化"掉。
- **宁可记录也不丢弃。** 解析失败、数据冲突、无法归类的条目，一律进
  `unparsed` / `constant_candidates` / `unresolved` 字段并标注原因，**不要静默跳过**。
- **产物必须可重建、可验证。** 自己产出的二进制格式（如 ZIP）要用**独立工具**验一遍
  （实测做法：`Expand-Archive` 解压 + `ffprobe` 解码解出的 JPEG）。
- 破坏性变更要同步更新 `scripts/README.md` 与相关 `--help`。
- 后期后端搭建**不要自己开始生成代码**（除非显式指定），我需要自己手写来锻炼开源
- 不要自己`git add`和`git commit`和`git push`，我手动验收

---

## 常用命令

```bash
# 数据管线（改任何数据相关逻辑后全跑一遍）
node scripts/build-song-db.ts && node scripts/build-arcaea-index.ts --quiet \
  && node scripts/build-jackets.ts --quiet && node scripts/build-backgrounds.ts --quiet \
  && node scripts/build-catalog.ts

# 背景映射覆盖率（改背景逻辑后先跑这个，不下载）
node scripts/build-backgrounds.ts --coverage

# 解析器回归（改解析逻辑后必跑）
node scripts/build-arcaea-index.ts --self-test

# 质量报告（看覆盖率、定数分歧、待人工裁决项）
node scripts/report-arcaea-index.ts

# 打包发布
node scripts/pack-data.ts --emit-tree --base-url https://<你的域名>/arcaea
```

缓存位于 `.cache/`，重跑很快（索引 180s → 0.1s）。需要强制刷新时加 `--refresh`。

---

## 文档索引

| 文档 | 内容 |
|---|---|
| `docs/00-方案讨论.md` | ★ **主文档**。设计评审、可行性评分、三视角点子库、Top5 优先级、架构与流水线、性能预算、数据契约、界面信息架构、Go 选型、Arcaea 专项方案、竞品逆向附录 |
| `docs/01-开发流程.md` | ★ **开发路线图**。P0 地基 → P1 渲染管线 → P2 成绩层 → P3 素材层 → P4 模板产品化 → P5 打包 → P6 投稿闭环；每阶段的产出、**门禁**、风险回退与分工 |
| `scripts/README.md` | ★ 脚本用法、Anubis 过盾细节、曲绘命名规律、感知哈希方案、数据分发策略 |
| `docs/音游成绩数据获取方式调研.md` | 11 款音游的成绩数据源逐款核实 |
| `docs/arcaea-score-image-research.md` | Arcaea 成绩图来源与 OCR 公式 |
| `docs/research-arcaea-assets-ecosystem.md` | Arcaea 谱面渲染器与视频生态 |

> ⚠️ 调研报告是**原始记录**，其中若干结论已被主文档修正
> （例：`wiki.arcaea.cn` 的 API 被 Anubis 挡住、`phi-chart-render` 是废弃项目）。
> **有冲突时以 `docs/00-方案讨论.md` 为准。**

---

## 关键架构决策（已定型，不要重新讨论）

1. **不做实时爬取，走索引库。** 竞品用两年时间收敛到这个结论（README 明写
   "bilibili api 自 2026-06 起不再维护，转向数据库 Metadata"）。素材获取抽象成
   可替换的 `ResourceProvider`：`索引库 → 本地渲染 → 实时搜索（兜底）`。

2. **模板 = HTML/CSS，WebView2 就是渲染器。** 同一套模板同时服务
   实时预览、导出 PNG 图层、最终合成。用户自定义模板 = 写 HTML。

3. **两阶段渲染 + concat demuxer。** 逐曲用**完全统一的编码参数**产出中间文件，
   最后 `-f concat -c copy` 无损秒拼。收益：断点续跑、增量重渲染、单段预览。
   ⚠️ 硬编码加速可选，但**同一批次必须用同一个编码器**。

4. **时长锚点**：标准版 10s/首 ≈ 9 分钟（对齐社区习惯）；另有精华版 ≈ 90 秒。

5. **OCR 是降级路径，不是主路径。** 头部游戏（maimai/CHUNITHM/osu!）一次 API 请求
   就能拿全 B50。Arcaea 因无可用 API 才走 OCR，且必须「曲绘 pHash 定曲目 +
   OCR 只读数字 + 六条一致性自检」。

6. **数据与代码分离发布。** 应用发版与曲库更新是两条独立流水线：游戏加新曲时
   重跑数据管线 + 打包即可，**用户不用更新应用**。

7. **谱面录像运行时按 BV号 取流，本地缓存（铁律 7 的唯一例外）。** 用 `yt-dlp.exe`
   sidecar，不自研 B站 API 客户端。必须配套：缓存键去重、多来源降级、
   手动粘 BV 号兜底、片尾如实署名。理由与实测数据见「技术栈」一节。
