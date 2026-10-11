# scripts/

构建期工具。**不属于应用运行时**，只在开发/发版时跑，产出物进 `data/`。

运行时依赖：**Node.js ≥ 18**（用内置 `fetch`），**零第三方依赖**，不需要 `npm install`。

---

## ⭐ 2026-09：曲库主源已切到 wiki

① 不再以 2023 年的 GitHub `arcsong.json`（335 首）为主源，改用 wiki 的 4 个 JSON 内容模型页面：

| 页面 | 提供 |
|---|---|
| `Template:Songlist.json` | 曲名 / 曲师 / BPM / 曲包 / 版本 / 日期 / 背景 / 难度等级（554 首） |
| `Template:ChartConstant.json` | 精确定数（小数） |
| `Template:ChartNotes.json` | 物量 |
| `Template:SongDuration.json` | 时长 |

`arcsong.json` 降为**补充源**，只留两个用途：**中文别名**、**`set_friendly` 显示名**。**拉不到不致命**。

**效果（实测）**：`song_id_source: derived`（元数据不全的曲目）**198 → 11 首**，
谱面 1350 → 1849，定数 1045 → 1832，索引的 song_id 关联率 355/650 → 638/650。

抓取统一走 `lib/wiki.ts`（缓存 + 重试 + Anubis 过盾）——**不要在脚本里另抄一份**。

> ⚠️ **本文档下文凡是写「上游停在 2023」「198 首 derived」「对不上 12 首」的地方，
> 都是切换前写的，保留原文是为了记下当时的推理过程。** 数字以本节与实际 `data/` 为准。

---

## 执行顺序

```bash
# ── 构建期：产出运行时数据 ──
node scripts/build-song-db.ts        # ① 曲库（原始层，联网，~1s）
node scripts/build-arcaea-index.ts   # ② 视频索引（原始层，联网，冷跑 ~180s）
node scripts/build-jackets.ts        # ③ 曲绘 + 感知哈希（联网，冷跑 ~208s / 热跑 ~2s）
node scripts/build-backgrounds.ts    # ④ 游玩背景图 + 曲目→背景映射（联网，冷跑 ~100s）
node scripts/build-catalog.ts        # ⑤ ⭐ 合并成应用唯一读取的目录（纯本地，0s）
node scripts/report-arcaea-index.ts  # ⑥ 质量报告

# ── 发布期：打包成资源包 ──
node scripts/pack-data.ts --emit-tree --base-url https://cdn.example.com/arcaea
```

### 三类数据，三种去处

| 文件 | 体积 | 变化频率 | 去处 |
|---|---|---|---|
| `data/song-id-aliases.json` | 2 KB | 人工维护 | ✅ **进主仓库**（它是配置，不是产物） |
| `data/arcaea-catalog.json` | 1.3 MB | 每周~每月 | 📦 **发布包** |
| `data/jackets/256/*.jpg` | 12.1 MB | 每月（新曲） | 📦 **发布包** |
| `arcaea-songdb.json` / `arcaea-index.json` / `arcaea-jackets.json` | 1.5 MB | — | ❌ **不发布**（构建中间产物，随时可重建） |

`.gitignore` 已经这样配好了：`data/*` 全忽略，只放行 `song-id-aliases.json`。

### 发布通道选型

| 方案 | 成本 | 大陆速度 | 结论 |
|---|---|---|---|
| **对象存储 + CDN**（阿里云 OSS / 腾讯云 COS / Cloudflare R2） | ~¥5/月（13MB × 千级用户） | ✅ 快 | ⭐ **推荐** |
| GitHub Releases | 免费 | ⚠️ 慢/不稳 | 起步可用，国内体验差 |
| GitHub 仓库直接放 | 免费 | ⚠️ 慢 | ❌ **会污染 git 历史** |
| Git LFS | 免费额度 1GB 存储 / 1GB 月流量 | ⚠️ | ❌ 一两次刷新就打爆 |

> 竞品用的就是阿里云 OSS（`nickbit-maigen-images.oss-cn-shanghai.aliyuncs.com`）——
> 而**我实测那个地址现在返回 404**，正好说明单点依赖的风险。
> 建议客户端支持**多个镜像源**，并在 `latest.json` 里带 sha256 以便校验。

### 客户端流程

```
首次运行：
  1. GET <mirror>/latest.json              ← 只有几百字节
  2. 与本地 version.json 比对 data_version
  3. 全量：下载 archive（13MB，带断点续传）→ 校验 sha256 → 解压到用户数据目录
     增量：按 files[] 逐文件比对 sha256，只下差异项 → 适合日常小更新
  4. 落版本号，之后完全离线运行

后续运行：
  · 直接读本地数据，零网络
  · 每 N 天或用户手动触发检查更新
  · 更新失败 → 继续用本地旧版本（不要因为更新失败让程序不可用）
```

### pack-data.ts 做了什么

```bash
node scripts/pack-data.ts --emit-tree --base-url https://cdn.example.com/arcaea
```

| 产出 | 用途 |
|---|---|
| `dist/arcaea-data-<版本>.zip` | 首次全量下载（622 条目 / 13.0MB） |
| `dist/latest.json` | 客户端唯一需要先拉的小文件（版本 + sha256 + 逐文件清单） |
| `dist/tree/`（`--emit-tree`） | 散文件树，供 OSS/CDN 逐文件托管 + 增量更新 |

**两个关键点：**

1. **路径重写**。构建期 catalog 里的路径是 `data/jackets/256/x.jpg`（相对仓库根），
   解压后数据根的布局是 `jackets/256/x.jpg`。打包时会自动改写（实测重写 507 条），
   不重写客户端会全部 404。
2. **ZIP 自己写**（`scripts/lib/zip.ts`）。Node 标准库没有 ZIP 写入能力，
   而发布包全是已压缩的 JPEG —— 用 **store 模式**（不压缩）最优：
   收益 <2%，但打包 13MB 只要 0.5s，且零依赖跨平台。

> 打包后**务必用独立工具验证**（本项目实测：`Expand-Archive` 能解、`ffprobe` 能解码
> 解出的 JPEG、catalog 路径重写为 0 条残留）。自己写的二进制格式不验证等于没写。

---

## 数据分层

```
原始层（保留，可审计、可重建、可脱离网络重跑④）
  data/arcaea-songdb.json     ← GitHub 曲库
  data/arcaea-index.json      ← B站 视频索引
  data/arcaea-jackets.json    ← 维基曲绘 + 感知哈希

资源
  data/jackets/256/*.jpg      ← 内置用的 256px 曲绘（~12MB）

合并层（⭐ 应用只读这一个文件）
  data/arcaea-catalog.json    ← 以 song_id 为主键的完整曲目目录
```

**为什么合并层是必要的，而不是让应用读三个文件：**

1. **主键不统一** —— 曲库用 `song_id`，索引用 `(曲名, 难度)`。运行时做 join 等于把归一化逻辑复制一份进 Go，两边必然漂移。
2. **OCR 纠错要预计算匹配表** —— 曲名 / 日文名 / `song_id` / 全部别名 / 宽松键，在 ④ 里拍平成 `match_keys`，应用侧只做一次 Map 查找。
3. **定数会多源打架** —— ④ 保留全部候选值 + 标注来源 + 按优先级选推荐值，而不是让应用猜。

---

## lib/anubis-fetch.ts —— 过 Anubis 反爬

`wiki.arcaea.cn`（及镜像 `arcwiki.mcd.blue`）由 [Anubis](https://anubis.techaro.lol/)
保护：一个针对 AI 爬虫的 Proof-of-Work 门。**所有路径**（含 `/images/`、`Special:FilePath`）
都返回挑战页 —— **HTTP 200 是挑战页的状态码，不是数据**。

### 🕳 三个实测踩出来的坑（缺一不可）

**坑 1️⃣：WAF 按 TLS 指纹拦截 Node 的 TLS 1.3 握手。**

```
❌ fetch('https://wiki.arcaea.cn/...')            → ECONNRESET
❌ https.get(..., { /* 默认 */ })                  → ECONNRESET
✅ https.get(..., { minVersion:'TLSv1.2', maxVersion:'TLSv1.2' })  → 200
```

连接在 **TLS 阶段就被 RST**，HTTP 请求根本没发出去。curl（Schannel）能过，Node（OpenSSL 3）
不能。所以本模块**不用 `fetch`**，改用 `node:https` 并锁定 TLS 1.2。

**坑 2️⃣：Anubis 有 cookie 往返测试。**

挑战页会下发 `techaro.lol-anubis-cookie-verification`，**提交答案时必须带回**，
否则服务端返回 500 并明说：

> Your browser is configured to disable cookies. Anubis requires cookies for the legitimate
> interest of making sure you are a valid client.

**坑 3️⃣：`pass-challenge` 的 `redir` 必须是绝对 URL。**

对照官方 `main.mjs`：`redir = window.location.href`。传相对路径不起作用。

### PoW 算法（逐行对齐官方 worker）

源码：`/.within.website/x/cmd/anubis/static/js/worker/sha256-webcrypto.mjs`

```js
c = Math.floor(difficulty / 2)   // 前 c 个「字节」必须为 0
l = difficulty % 2 !== 0         // 奇数难度时，再要求第 c 字节高 4 位为 0
hash = SHA256(randomData + String(nonce))   // ← nonce 是十进制字符串
```

⚠️ 是「**字节**」不是「十六进制字符」。实测该站 `difficulty: 2`，毫秒级可解。

### 完整流程

```
① GET 目标页                          → 拿到 anubis_challenge JSON + 验证 cookie
② 解 PoW：sha256(randomData + nonce)  → 前 floor(d/2) 字节为 0
③ GET pass-challenge?id&response&nonce&redir(绝对URL)&elapsedTime
        带上①的 cookie                → HTTP 302 + techaro.lol-anubis-auth (JWT)
④ 带 auth cookie 重放原请求            → 正常数据
```

`CookieJar` 会持久化到 `.cache/anubis-cookies.json`，**同一台机器上后续请求不再解题**
（实测首次 ~1.5s，之后 ~0.2s）。若 cookie 过期，服务端会返回 `challenge: null` 的错误页，
客户端会自动清空 cookie 并重试。

> ⚠️ 请遵守站点规则，控制请求频率（脚本内置 `--delay`），不要把这个当爬虫工具滥用。

---

## build-jackets.ts

抓取曲绘并生成感知哈希索引。

```bash
node scripts/build-jackets.ts                      # 只下 256px 缩略图（~12MB）
node scripts/build-jackets.ts --sizes 256,full     # 同时下 768px 原图（~175MB）
node scripts/build-jackets.ts --update-catalog     # 顺带回填 catalog 的 jacket/phash
node scripts/build-jackets.ts --limit 20           # 调试用
node scripts/build-jackets.ts --refresh            # 忽略缓存，强制重下 + 重算哈希
```

### ⚠️ 感知哈希与限速都有缓存，别把它们重算一遍

感知哈希（aHash / dHash）**每个各 spawn 一次 ffmpeg**。620 张曲绘就是
**1240 个进程、约 160 秒** —— 这是整条数据管线里最贵的一步，而哈希对同一个文件是**确定值**。

所以脚本会读上一轮的 `arcaea-jackets.json`：**图片文件命中缓存（没重新下载）时直接复用旧哈希**。
只有「上一轮压根没算出哈希」（当时缺 ffmpeg / 哈希失败）才重算，会自愈。

限速同理：`--delay`（默认 120ms）**只在真的发过请求之后**才 sleep。
无条件 sleep 的话 620 张 × 120ms = 74 秒纯白等。

两个加起来，重复跑从 **173s 降到 2.2s**，整条数据管线 **166s → 2.8s**。

> `--refresh` 会同时关掉这两个复用（重新下载 + 重新哈希），这是它的语义。
> 实测产物逐项一致：620 条哈希完全相同。

### ⭐ 曲绘文件命名规律

维基上的曲绘统一命名为 **`Songs_<song_id>.jpg`**：

```
Songs_aegleseeker.jpg
Songs_abstrusedilemma.jpg
Songs_aiueoon.jpg          ← 曲名是 AI[UE]OON
```

**`<song_id>` 与 Arcaea 官方 ID 同构**，正好等于 `build-arcaea-index.ts` 里
`deriveSongId()` 推导出来的形式，所以能直接 join 上曲目目录。

实测规模：**625 张，原生 768×768，合计 174.8MB**。

### 缩略图不用自己缩

MediaWiki 能按任意宽度实时生成：

```
原图    https://wiki.arcaea.cn/images/5/5a/Songs_aegleseeker.jpg
256px   https://wiki.arcaea.cn/images/thumb/5/5a/Songs_aegleseeker.jpg/256px-Songs_aegleseeker.jpg
```

所以默认只下 256px（约 12MB），768px 按需。

### 感知哈希

用 **ffmpeg** 把图解码成 9×8 灰度裸数据，再在 Node 里算：

| 算法 | 做法 | 输出 |
|---|---|---|
| `ahash` | 缩到 8×8，与均值比较 | 16 位十六进制（64 位） |
| `dhash` | 缩到 9×8，比较水平相邻像素 | 16 位十六进制（64 位） |

**用途**：OCR 管线里**用曲绘指纹识别曲目**，比识别花体曲名可靠得多
（见 `docs/00-方案讨论.md` §13.4 层 2）。

> 💡 **哈希只有 16 字节/张，620 张合计约 10KB —— 可以放心内置；高清曲绘不行。**

用临时文件而非 stdio 管道传 ffmpeg 输出，避免跨平台的管道坑。

---

## build-backgrounds.ts

构建「曲目 → 游玩背景图」映射并抓取背景图 → `data/arcaea-backgrounds.json` + `data/backgrounds/960/*.jpg`。

```bash
node scripts/build-backgrounds.ts --coverage        # 只量覆盖率，不下载（改逻辑后先跑这个）
node scripts/build-backgrounds.ts                   # 产出 960px / 16:9（默认）
node scripts/build-backgrounds.ts --sizes 960,1280  # 多档
node scripts/build-backgrounds.ts --no-crop         # 保留 4:3 原比例
node scripts/build-backgrounds.ts --refresh         # 忽略缓存
node scripts/build-backgrounds.ts --delay 600       # 请求间隔（毫秒）
```

### 为什么需要它：catalog 的 `bg` 字段不可信

| 曲目来源 | 数量 | `bg === song_id` 的比例 |
|---|---|---|
| official（来自 arcsong.json） | 335 | 9%（巧合） |
| **derived（2023 年后新曲）** | **198** | **100%** ← 构建时的占位值 |

另有 38 首 v1.x 老曲干脆没有 `bg`。所以背景**必须从 wiki 重建**，不能沿用 catalog 的字段。

### ⭐ 数据来源：`Template:Songlist.json`（游戏原始数据）

wiki 上有 **26 个 JSON 内容模型页面**（`contentmodel: json`），实测全部可取。
其中 `Template:Songlist.json` 直接带背景字段：

```jsonc
{
  "id": "sayonarahatsukoi",
  "bg": "base_light",          // ← 该曲该用的背景
  "bg_inverse": "base_conflict",
  "side": 0,                   // 0=光芒侧  1=纷争侧
  "difficulties": [{ "ratingClass": 0, "rating": 1, "chartDesigner": "Nitro" }]
}
```

**`side` 与 `bg` 严格对应**：`side: 0` 时 `bg` 是光芒侧、`bg_inverse` 是纷争侧；`side: 1` 时相反。
**`bg` 永远是这首歌该用的那张，没有歧义、不需要猜。**

覆盖率：**520/533 首（97.6%）**，`bg` 值全部有对应图片（0 个缺图），输出 140 张 / 9.5 MB。

### 🕳 曾经绕过的弯路（留作记录，别再走）

最初是用 `Category:采用<bg>背景的曲目` 反查——**175 次请求**、覆盖率 97.7%，
而且会撞上「一首歌落在多个背景分类里」的假冲突。当时我为此设计了一套优先级规则
（`candidates` + 选择启发式），**结果发现冲突根本不存在**：

| 曲目 | 分类反查"猜"的 | `Songlist.bg` 实际值 |
|---|---|---|
| `bethere` | ❌ `single2_light` | ✅ `base_light` |
| `dropdead` | ❌ `byd_conflict` | ✅ `base_conflict` |
| `aterlbus` | ❌ `aterlbus` | ✅ `dynamix_conflict` |

歧义是分类粒度不够（只到曲目、分不出 Light/Conflict 两面）**造出来的**。
换成 `Songlist.bg` 后：**175 次请求 → 3 次**，49.4s → **2.8s**，优先级规则整套删掉。

> **教训：能查到事实就不要猜。** 猜测逻辑会自我合理化——你为它写规则、留候选、
> 记 unresolved，看上去很严谨，但它解决的问题本来就不存在。

### 交叉对齐：官方 ID vs 推导 ID

`Songlist` 用**官方 song_id**，而 catalog 里有 198 首的 ID 是从曲名推导的，两者不同：

```
官方 azalea    我们的 zle
官方 awakening 我们的 wakeinhour
官方 aishite   我们的 lovemelovemeloveme   ← wiki 用日文名
```

所以按 **id → 严格归一化曲名 → 宽松键** 三级回退对齐。
实测：精确 id **507** ｜ 曲名 **14** ｜ 宽松键 0 ｜ 对不上 12。

产物里带 `official_id` —— 这是后续把曲库主源切到 wiki 的桥。

### 🕳 坑一：非标准档位的缩略图会 404

该站可用的缩略图宽度**只有标准档位**：`240 / 300 / 600 / 800 / 1280 / 1600`；
`360 / 480 / 720 / **960** / 1080` 一律 404 —— 第一次跑时 145 张里失败了 63 张。

⚠️ 这个 404 返回的是 **34KB 的 MediaWiki HTML 错误页**，体积很像正常内容。
**必须校验 JPEG 魔数（`FF D8`），不能只看状态码。**

**对策：从标准档位下源图（1280 优先，逐级回退）→ 本地 ffmpeg 缩放到目标尺寸。**
额外收益：源图缓存在 `.cache/backgrounds/src/<bg>@<宽度>.jpg`，
**以后想换输出宽度或裁法只改参数重跑，不会重新联网**（实测重跑 0.1s、0 请求）。

### 画幅：原生 4:3 → 成片 16:9

原生 `1920×1440`（4:3），成片恒定 16:9，所以按「等比覆盖 → 居中裁掉多余高度」处理：

```
1280×960  --crop(iw:iw*9/16, 居中)-->  1280×720  --scale-->  960×540
```

裁切放在**构建期**（而不是渲染期）是因为它确定；源图缓存让改裁法零成本。
`--no-crop` 可保留 4:3。

### 产物结构

```jsonc
{
  "output_aspect": "16:9",
  "backgrounds": {
    "base_light": {
      "file": "Background_base_light.jpg",
      "native": "1920x1440",
      "source_width": 1280,
      "files": { "960": { "path": "data/backgrounds/960/Background_base_light.jpg", "bytes": 61615 } },
      "songs": ["sayonarahatsukoi", "..."]     // 38 首
    }
  },
  "songs": {
    "dropdead": { "bg": "base_conflict", "bg_inverse": "base_light",
                  "official_id": "dropdead", "matched_via": "id" },
    "zle":      { "bg": "azalea", "bg_inverse": null,
                  "official_id": "azalea", "matched_via": "title" }
  },
  "unresolved": { "songs_not_in_songlist": [], "bg_without_image": [] }
}
```

> ⚠️ **路径一律正斜杠。** catalog 里已有的路径就是正斜杠，而 `pack-data.ts`
> 要按前缀重写它们。Windows 上 `path.join` 产出反斜杠，混进来会让重写失配、客户端全部 404。

### 对不上 Songlist 的 12 首

双曲目 medley（`Axium Crisis & Grievous Lady`、`Ether Strike & Fracture Ray`）、
变体曲（`PRAGMATISM -RESURRECTION-`、`Vicious [ANTi] Heroism`）、
以及视频索引带来的曲名偏差（`Snow Shite`、`OΔYΣΣEIA`、`Last | Moment`）。
全部进 `unresolved`，**不静默丢弃**。

> 根源是这些曲目的 `title_en` 来自 **B站视频标题**（它们是 derived 曲），不是曲库。
> 把曲库主源切到 wiki 之后，这类偏差会自然消失。

### 顺带发现：wiki 上有 39 首我们没有的歌

`Cataclysm Cry` / `DREAD AREA` / `Riven Pilgrimage` / `Liar` / `Tic! Tac! Toe!` …
—— 大多是 v7.0 新曲与 CHUNITHM / DJMAX / 音击 / Muse Dash 联动曲。
**这是 catalog 停留在 2023 年的独立佐证**（见 `docs/00-方案讨论.md` §13.3）。

---

## build-catalog.ts

把曲库与视频索引合并成应用目录。**纯文件转换，不联网，0 秒完成。**

```bash
node scripts/build-catalog.ts
node scripts/build-catalog.ts --song-db data/arcaea-songdb.json --index data/arcaea-index.json
```

> 💡 **本文件导出了 `normalizeTitle` / `looseKey` / `deriveSongId`，可以被其它脚本 import**
> （`build-backgrounds.ts` 就是这么用的，以保证 join 口径一致）。
> 文件末尾有**入口守卫**：只有被直接执行时才跑 `main()`。
> 加这道守卫之前，`import` 一次会把整个合并流程重跑一遍 ——
> 表现是你只想借一个函数，却看到满屏「目录合并完成」的输出。

产出结构（节选）：

```jsonc
{
  "difficulty_class_order": ["PST","PRS","FTR","BYD","ETR","INS"],
  "constant_priority": { "wiki": 0, "songdb": 1, "video": 2, "derived": 3 },
  "songs": [
    {
      "song_id": "aegleseeker",
      "song_id_source": "official",        // official | derived
      "title_en": "Aegleseeker",
      "title_jp": "",
      "title_norm": "aegleseeker",
      "aliases": ["光追","RTX","辉神星", "..."],
      "match_keys": ["aegleseeker","光追","rtx", "..."],  // ← OCR 纠错用
      "artist": "Silentroom vs Frums",
      "bpm": 234,
      "version": "3.6",
      "jacket": { "bundled": false, "source_hint": "user_supplied" },  // 待后续步骤填充
      "phash": null,                                                    // 待后续步骤填充
      "charts": [
        {
          "difficulty": "PRS",
          "level": "9",
          "constant": 9.1,
          "constant_source": "wiki",
          "constant_candidates": { "wiki": 9.1, "video": 8.8 },  // ← 分歧可见
          "note": 1235,                                          // ← OCR 判定数守恒校验用
          "chart_designer": "⟨OBSRV⟩ boundary(?°a,?°A)",
          "video": {
            "sources": [                                          // ← 多来源，抗单点失效
              { "bvid": "BV1uf4y1r793", "p": 52, "duration_sec": 151,
                "url": "https://www.bilibili.com/video/BV1uf4y1r793?p=52",
                "part_title": "(9.1) Aegleseeker [PRS 9]",
                "collections": ["Arcaea谱面保管室（难度分类）"] }
            ]
          }
        }
      ]
    }
  ],
  "unresolved": [ /* 无法归入任何曲目的索引条目（auto_part_title 等），原样带过来，不丢弃 */ ]
}
```

### 定数优先级

| 来源 | 说明 | 可信度 |
|---|---|---|
| `wiki` | `ChartConstant.json`（2023-05，wiki 工作组维护） | 最高 |
| `songdb` | `arcsong.json`（2023-07）—— 与 wiki 在 318 首重叠上 **0 处不一致** | 高 |
| `video` | 视频分P标题里的 `(11.2)` —— 汇编视频多为 2022–2023 年发布，**20/104 与 wiki 不符**，判定为过时值 | 中 |
| `derived` | 由等级标签推算 | 仅占位 |

**43 处真实分歧全部是 `wiki` vs `video`**，一致地印证了"汇编视频带的是发布当时的定数"。
分歧不会被静默丢弃 —— `constant_candidates` 完整保留，`constant` 是按优先级选出的推荐值。
同一数值的多个来源会被合并（只留优先级最高的那个），避免候选堆积。

### `song_id_source: "derived"` 的含义

索引里有 198 首曲不在上游曲库中（都是 2023 年后的新曲）。这些曲的 `song_id`
由曲名推导（`Abstruse Dilemma` → `abstrusedilemma`）—— Arcaea 官方 ID 恰好就是这个形式，
命中率很高，但**仍需与游戏真实 ID 核对**。若推导出的 ID 与官方曲目撞车，会加 `-x` 后缀并计入
`stats.derived_id_collisions`。

### `jacket` / `phash` 字段

由 `build-jackets.ts` 产出的曲绘表填充（见上文），并顺带用官方 ID 清单校正推导 ID。

---

## data/song-id-aliases.json —— 人工 song_id 映射表

索引里的 `song_id` 是从曲名推导的（`deriveSongId`），但官方 ID 有大量**无法自动推导**的命名改写：

```
Aleph-0               → alephzero       数字写成单词
Alone & Lorn          → aloneandlorn    & 写成 and
Ashen 6oundary        → ashenboundary   标题里 6 是风格化的 b
CHAIN2NITE            → chaintonite     2 写成 to
FANTA5Y               → fantasy         5 写成 s
LIVHT MY WΔY          → lightmyway      leet / 希腊字母
```

对齐分三级：**① 精确命中 → ② 互为前缀（≥6 字符，唯一或最长不并列）→ ③ 人工映射表**。

### 生成建议

```bash
node scripts/build-catalog.ts --suggest-aliases
```

用字符二元组 Dice 相似度打分，把建议写进 `_suggestions`（下划线前缀，加载时被忽略）。
**人工核对后**再把条目挪进 `map`。

> ⚠️ **不要盲目自动套用建议** —— 给一首曲配上别人的曲绘，比缺图糟糕得多。
> 当前 `_pending` 里还留着 2 条低置信度建议待确认。

当前效果：**34 个推导 ID 被校正为官方 ID**，曲绘覆盖从 472 首提到 **507/533 首（95.1%）**。

---

## build-song-db.ts

从 GitHub 构建 Arcaea 曲库 → `data/arcaea-songdb.json`。

```bash
node scripts/build-song-db.ts
node scripts/build-song-db.ts --refresh
```

| 源 | 内容 | 规模 | 新鲜度 |
|---|---|---|---|
| [`ArcaeaSongDatabase`](https://github.com/Arcaea-Infinity/ArcaeaSongDatabase) `arcsong.json` | 曲名·曲师·BPM·**物量 note**·**别名 alias[]**·定数 | 335 首 | ⚠️ 2023-07 |
| [`chart-constant`](https://github.com/zh-arc-wiki-re-wg/chart-constant) `ChartConstant.json` | 定数（用于交叉校验） | 318 首 | ⚠️ 2023-05 |

> 🔴 **两份上游数据都停留在 2023 年**，不含 v5.4 定数大调整后的变动，也不含 2024–2026 新曲。
> 运行时会打印告警。补充方式见 `docs/00-方案讨论.md` §13.3。

> ⚠️ **`difficulties[].difficulty` 不是难度类别，而是「等级 × 2」**
> （Aegleseeker = 10/18/22 → 等级 5/9/11；19/21 → `9+`/`10+`）。
> **难度类别只能按数组顺序推**：`PST / PRS / FTR / BYD / ETR`。
> 本脚本按此实现，并用 wiki 定数表交叉校验过 —— **318 首命中、0 处不一致**，映射正确。

> ⚠️ `raw.githubusercontent.com` 在部分网络不可达，`api.github.com` 只有 60 次/小时。
> 脚本**镜像优先**（`gh-proxy.com` → `ghfast.top` → 官方 raw），带本地缓存。

---

## build-arcaea-index.ts

从 B站 UP 主的「谱面保管室」合集构建视频索引 → `data/arcaea-index.json`。

```bash
node scripts/build-arcaea-index.ts                    # 默认 UP: 441390256
node scripts/build-arcaea-index.ts --refresh --delay 600
node scripts/build-arcaea-index.ts --self-test        # 解析器回归用例
node scripts/build-arcaea-index.ts --no-song-db       # 不做 song_id 关联
```

产出结构：

```jsonc
{
  "stats": { /* 覆盖率统计 */ },
  "charts": [
    {
      "chart_key": "fracture ray|ftr",
      "title": "Fracture Ray",
      "title_norm": "fracture ray",   // 归一化匹配键
      "difficulty": "FTR",
      "level": "11",
      "constant": 11.2,               // 精确定数
      "constant_source": "video_title", // video_title | song_db
      "song_id": "fractureray",       // ← 曲库关联结果
      "song_match_via": "title_en",   // title_en | alias | song_id | loose
      "sources": [                    // 多来源 → 抗单点失效
        { "bvid": "BV1T54y1s7f6", "p": 3, "duration_sec": 164,
          "url": "https://www.bilibili.com/video/BV1T54y1s7f6?p=3" }
      ]
    }
  ],
  "unparsed": [ /* 解析失败清单，需人工裁决 */ ]
}
```

### 数据从哪来

| 接口 | 用途 |
|---|---|
| `x/polymer/web-space/seasons_series_list` | 列出全部合集与系列 |
| `x/polymer/web-space/seasons_archives_list` | **合集的分页视频**（列表接口里的 `archives` 只是截断预览！） |
| `x/series/archives` | 系列的分页视频 |
| `x/web-interface/view?bvid=` | 取 `pages[]`（**分P标题 + 时长**） |

> ⚠️ **不要尝试 `x/space/wbi/arc/search`**。即使带上完整正确的 WBI 签名
> （`img_key`/`sub_key` 提取 + 64 位 mixin 表重排 + `wts` + md5 得 `w_rid`），
> 它依然返回 `code=-352 风控校验失败` —— 还需要 cookie。
> 目标 UP 把内容都组织进了合集/系列，上面四个接口免签名就够。

### 关键设计

- **以 `pages[].part` 为权威**，不是 `view.title`。两者会不一致
  （实例 `BV1oi4C6DExx`：视频标题 `[INS 12]`，分P标题 `[FTR 11]`）。
  不一致时记到 `sources[].alt_difficulty`，交人工裁决。
- **分P标题含精确定数**：形如 `(11.2) Fracture Ray [FTR 11]`。
- **多来源保留**：同一谱面出现在汇编视频和单曲视频里时全部保留，视频被删可自动降级。
- **画幅（`video_width` / `video_height`）**：`view` 接口的 `pages[].dimension` 顺手存下来，
  **零额外请求**。为什么必须有：实测 **871 个来源里 37.5% 是 4:3**
  （1920×1440 / 2732×2048 / 3840×2880），按谱面算 **321/645 根本没有 16:9 来源** ——
  渲染层要据此选模板，而**不能在下载之后才知道**。
  存的是**显示尺寸**（`rotate` 为 90/270 时交换宽高），渲染层不用再判断。
- **署名（`owner_name` / `owner_mid`）**：同样来自 `view` 接口的响应（`data.owner`），**零额外请求**。
  为什么必须有：铁律 8 要求片尾/简介列出「**原作者** + BV号 + 链接」——
  只给 BV 号不说作者是谁，那是**引用**不是**署名**（§13.6 把产品定位成「二创署名器」，
  署名是核心卖点而不是免责声明）。
  > 这一项**曾经被漏采**：`getVideoPages()` 只取了 `bvid/title/videos/duration`，
  > `owner` 就在同一个响应里却丢掉了，导致下游只有 `source.mid`（441390256）没有名字。
  > 补上后重跑是**零联网**的（响应全在 `.cache/arcaea-index/`）。
- **限速 + 重试 + 本地缓存**（`.cache/`）。冷跑 ~180s，热跑 0.1s。**加字段后重跑是零联网的**。
- **宁可记录也不丢弃**：解析失败进 `unparsed` 并分类，不静默跳过。

> ⚠️ **`build-catalog.ts` 里 sources 是字段白名单** —— 索引层加了新字段必须同步加过去，
> 否则会被**静默丢掉**（`video_width` 第一次加进来时就踩过这个坑：索引里有了、catalog 里没有）。

### 标题解析：从右往左只吃"像标签的"方括号

这是本脚本最容易写错的地方。**Arcaea 里存在曲名本身带方括号的歌**
（`[X]`、`AI[UE]OON`），无脑 `replace(/\[[^\]]*\]/g, '')` 会把它们吃没。

策略：先剥 `【…】` 前缀与 `谱面确认/演出` 后缀，然后**从右往左**吃掉像标签的
`[...]`（难度代号 / `???` / `Path III`），遇到不像标签的立刻停下 —— 剩下整体是曲名。

实测覆盖的变体：

```
(10.0) Alexandrite [FTR 10]                                      ✅
【…】Lilly [FTR 9+] 谱面确认                                       ✅
【…】un [ETR] [Path III] 谱面确认                                  ✅ 有难度无等级
【…】Suomi [ETR8+] 谱面确认                                        ✅ 难度与等级间无空格
【…】[X] [FTR 10] 谱面确认                                         ✅ 曲名带方括号
【…】AI[UE]OON [FTR 10] 谱面确认                                   ✅ 曲名内嵌方括号
【…】DEINOS PHAINEIN [INS 12] 谱面演出                             ✅ INS 是新难度
【…】Future 10 谱面确认                                            ⚠️ 容器标题，非曲目
```

改动解析逻辑后**务必跑 `--self-test`**（10 条回归用例）。

### 已知的解析失败类型

| reason | 含义 | 处理 |
|---|---|---|
| `no_difficulty` | 容器标题，或 `[???]` 定数未知 | 多为正常，可忽略 |
| `auto_part_title` | UP 主没改分P名（`Akasha-20260829-065532`） | **需人工补曲名** |
| `no_title` | 标题里没有曲名 | 人工处理 |
| `fetch_failed` | 接口失败 | 重跑即可 |

---

## report-arcaea-index.ts

检查索引质量与覆盖度。

```bash
node scripts/report-arcaea-index.ts
node scripts/report-arcaea-index.ts --no-song-db
```

输出：总览、难度分布、每首曲的难度覆盖、多来源可用性、**需人工裁决清单**、
解析失败分类、**真实覆盖度**（索引 ↔ 曲库），以及（可选）与外部定数表的对照。

> ⚠️ 对照 `ChartConstant.json` 时注意：它的键是**内部 song_id**
> （`ifi`、`grievouslady`），不是显示曲名。**以报告里的「真实覆盖度」一节为准**，
> 那一节走的是索引里已写入的 `song_id`，口径与索引一致。

> ⚠️ **定数不一致通常不是谁错了**：汇编视频多为 2022–2023 年发布，
> 其分P标题里的定数是**发布当时的**，Arcaea 后续经历过定数调整。
> 以 wiki 最新值为准，冲突项人工复核。

---

## check-ptt-formula.ts

潜力值公式自检 —— **不产出数据，只做验证**（不参与构建管线，随时可跑）。

```bash
node scripts/check-ptt-formula.ts
node scripts/check-ptt-formula.ts --catalog data/arcaea-catalog.json --expect 13.438
```

两部分：

1. **单曲 ptt 参考节点**（15 项断言）：wiki「潜力值」页给的 7 个分数节点 × TL/TC 两列，
   外加一条下界（`max(0, …)`）。
2. **用曲库反推理论最高 PTT**：每个谱面按"1000 万分 + PM"（定数 + 2.2）取最高的 50 个，
   算 `(Σb50 + Σb10) / 60`，与 wiki 公布的 **13.438** 对拍（容差 0.001，不过就 exit 1）。

**为什么值得单独留一个脚本：**

- **数据层回归**：13.438 是**独立于我们曲库的第三方事实**。定数分布被上游改坏（漏抓、错位、
  版本回退），这个数立刻漂 —— 比翻 catalog 快得多。
- **Go 版公式的 oracle**：`internal/score` 的单曲 ptt 写完，用同一张表对拍。
  **只让 Go 自己跟自己比不行** —— 那只能证明它自洽，证明不了它没抄错。

> 当前实测（2026-10，`data/arcaea-catalog.json` 1832 条定数）：
> 最高定数 12（`deinosphainein` BYD）、第 50 名单曲 ptt 13.0000（`blackmind` FTR）、
> 理论最高 PTT **13.4383** ✅ 与 wiki 的 13.438 一致。

> ⚠️ 13.438 是**池相关**的量：受"最高定数的谱面数量有限"约束，**不是**公式上限
> （单曲 ptt 上限是 最高定数 + 2.2）。别把它当成常量写死进代码去断言。

---

## check-normalize.ts

归一化落差审计 —— **不产出数据，只做验证**。

```bash
node scripts/check-normalize.ts              # 默认期望落差 2 首
node scripts/check-normalize.ts --expect 5   # 曲库变了就调这个数
```

**为什么需要它**：曲库里的 `title_norm` / `match_keys` 是构建期用 TS 归一化过的
（`build-catalog.ts:244`：NFKC → NFD → 剥组合记号 → 引号/破折号 → 小写）。
而**应用侧（Go）标准库没有 NFKC / NFD** —— 这是成绩导入「曲目定位」的真实缺口（`docs/05` §6.1）。

脚本把 Go 能做到的部分实现一遍（**不做 NFKC 兼容分解**），对拍 catalog 自带的
564 组 `(title_en, title_norm)`，报出落差：

```
曲目总数 564，title 含非 ASCII 的 32 首
方案 B（不引 x/text）失配 2 首：
  ii          raw=" ͟͝͞Ⅱ́̕ "       曲库="ii"           方案B="ⅱ"
  incarnator  raw="INCARNATOR₀₀"  曲库="incarnator00"  方案B="incarnator₀₀"
```

**这个数字决定「引不引 `golang.org/x/text/unicode/norm`」**：

- 落差 2 首 → 引依赖（`x/text` 已在 go.mod 里，只是 indirect→direct），或接受这 2 首走人工候选
- 落差变大 → TS 侧新增了依赖 NFKC 的写法，需重新评估

> ⚠️ **这个 2 是"下限"，不是"方案 B 的全部代价"。** 本脚本用 TS 实现方案 B，所以它**还有 NFD 可用**；
> Go 手写连 NFD 都要自己仿 —— 实测第一次跑就另外漏了两类（`go test ./internal/catalog/...`）：
>
> ```
> neokosmo    "nέο κόsmo" → 曲库 "nεο κοsmo"   希腊文 tonos（预组合，仍需分解）
> sacrosanct  "⊥⊬"        → 曲库 "⊥⊢"          ⊬ 的规范分解 = ⊢ + 叠加斜线
> ```
>
> **NFD 是全 Unicode 的分解表，手写必然漏。** 所以 2026-10 的结论是**走方案 A（引 `x/text`）**，
> 最终由 `internal/catalog` 的 564 组对拍守住。本脚本留着，是为了记录"当初若不引依赖要付多少代价"。

> ⚠️ **不要为了让它变绿而改 TS 侧归一化** —— 那会改掉 564 首既有的 `match_keys`，要重跑管线，
> 且把"应用侧能力不足"伪装成"数据侧没问题"。落差要如实暴露。

> ⭐ 更强的对拍在 Go 侧：`catalog` 的单测应遍历全部 564 首断言
> `NormalizeKey(title_en) == title_norm`。那条测试测的是**真的实现**；
> 本脚本只回答"值不值得引依赖"这个设计问题。

---

## 索引重建频率

合集是"活的"——UP 主可以随时增删改合集与分P。建议：

- 发版前跑一次，产出带版本号的索引打进安装包
- 或做成定期任务，索引作为可独立更新的资源包下发

---

## spike-sample-video.ts（已删除）

P1 的临时样片脚本，2026-10 阶段 6 删掉 —— 它的使命是**把管线跑通、把编码参数定死、把坑暴露出来**。
argv 已被 `internal/render/testdata/golden-commands*.json` 逐字固化，Go 侧照它对齐；
它原来的活（`.cache/spike/work/`、`dist/sample/sample.mp4`）现在由 `go run ./cmd/pipeline` 接管。

---

## ⭐ ffmpeg 的 Range 行为（用本地代理实测出来的）

**结论：`-ss` 放在 `-i` 前面时，ffmpeg 只拉两段就把片段取回来了，不会下全片。**

做法是写一个本地代理（转发请求并记录 `Range` 头），让 ffmpeg 从代理拉流：

```
#1  GET  Range: bytes=0-          →  读到 262144 字节就 abort（256KB，第一段探测）
#2  GET  Range: bytes=413468-     →  读到 335791 字节就 abort（定位到目标后按需读）
                                      合计约 580KB，而整个文件是 5.8MB
```

两点值得记住：

1. **CDN 必须支持 Range** —— 实测 B站 CDN 返回 `206 Partial Content` + `Accept-Ranges: bytes`，
   中段 seek 也正常。如果换到不支持 Range 的源，这套就退化成"从头下"。
2. **第一段固定读 256KB 是 ffmpeg 的探测行为**，不是浪费 —— 它要先拿到容器头（moov）才能知道时间戳到字节的映射。

> 实测四种 seek 策略（`-ss` 前置 / 后置 / 不 copy 重编码 / 远距离 seek）**日志内容一致** ——
> 说明**策略怎么选都一样**，不用为这个纠结。真正决定下载量的是 `--download-sections` 的区间宽度。
