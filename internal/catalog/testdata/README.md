# testdata/

`catalog-mini.json` —— `internal/catalog` 单测用的**手写夹具**。

## 为什么不用 `data/arcaea-catalog.json`

它在 `.gitignore` 里（构建产物，24 MB）。用它做单测会变成
「我本机过、别人克隆下来挂」——**测试必须能独立跑**。

## 为什么不用真实数据的切片

真实数据的**边界是不可控的**：这一版恰好有 26 首无曲绘，下一版可能就没有了，
测试会在数据刷新时莫名其妙地开始漏测。

这份夹具**每一首歌都是为撞某个 nullable 字段而设计的**：

| 歌 | 设计意图 | 撞到的边界 |
|---|---|---|
| `complete` | 全字段齐备的基线 | 所有指针都非 nil，用来对照 |
| `sparse` | **最大 null 密度** | `bpm`/`time_sec`/`phash` = null；`jacket.local_256` 与 `native` **整键缺席**；`full_url` = **显式 null**；谱面的 `level`/`constant`/`constant_source`/`note`/`video` 全 null |
| `mixed` | 混合形态 | **4:3 来源**（1920×1440）；**两个来源**，第二个**不带画幅字段**（模拟老产物/上游没给）；`alt_difficulty`；`phash` **只有 ahash 没有 dhash** |

## 注意

- ⚠️ **夹具里没有 `_comment` / `_why` 之类的说明字段** —— 是故意留白的。
  加了的话，单测只要用 `decoder.DisallowUnknownFields()`（很多人会加）就会报
  「unknown field」。说明写在本文件里，夹具保持 schema 纯净。
- 唯一的例外是 `mixed` 的第二、三个 chart 用**同一个 song 的不同 difficulty**
  来覆盖 `video: null` 与「有视频」两条路径 —— 这是 schema 内的正当用法。
- 「`sparse` 里的 `jacket` 有 `bundled: false` 但**没有 `local_256` 键**」
  和「`full_url` 是 `null`」**是两种不同的缺失**，Go 里都解成 `nil`，
  但你要知道产物里两种都会出现（见 `docs/02-P1实现规格.md` 的字段说明）。
- **改 `data/arcaea-catalog.json` 的 schema 时同步改这里**，否则测试会假装通过。

## 这份夹具覆盖不到的

- 大文件（533 首）的性能特征 → 用真实数据做**基准测试**，不用单元测试
- 中文/日文曲名的编码 → 夹具里有 `完璧な曲` 和 `别名`，够用
- 真实产物的全部字段 → 夹具只声明**代码实际读取**的字段，多余字段不影响解析

### 故意省略的字段（P1 不读）

夹具已与真实 `data/arcaea-catalog.json` 做过**键集合比对**，确认没有编造字段。
下面是**真实产物里有、但夹具刻意不写**的 —— 因为规格里的 Go 结构体不读它们：

| 层 | 省略的字段 | 为什么可以不写 |
|---|---|---|
| 顶层 | `generator` / `constant_priority` / `notes` / `provenance` / `stats` / `unresolved` / `difficulty_class_order` | 全是构建期元信息与诊断数据，P1 用不到 |
| chart | `constant_candidates` | 多来源定数冲突时的候选值，**渲染不读**（P2 的定数校对才用） |

> 如果将来 Go 结构体开始读这些字段，**记得回来给夹具补上**，
> 否则那部分代码就是没测的（而且测试还是会绿）。
