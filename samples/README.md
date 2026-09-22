# samples/

手工数据。P1 阶段的输入 —— 成绩层（P2）做完之前，这里就是"成绩从哪来"的答案。

## `sample-project.json`

ScoreRecord（`docs/00-方案讨论.md` §10.1）的**子集**：只保留 P1 渲染真正要用的字段。

### ⚠️ 用之前要替换的东西

| 字段 | 现状 | 要做什么 |
|---|---|---|
| `player.name` / `id` / `rating` | 占位值 `（请替换成你的名字）` | **换成你自己的** |
| `songs[].score` | 我按 `pure+far+lost == note` 反推的**自洽但非真实**的分数 | 换成你的真实成绩（G1 才有说服力） |
| `snapshot_at` | 固定值 | 改成导出时间 |

> 三首歌（Testify / Tempestissimo / Arcana Eden）是**故意选的** ——
> `.cache/clips/` 里已经有它们的谱面录像，`cmd/pipeline` 一写出来就能跑通。
>
> 但注意：如果阶段 4 用了正确的缓存键（含难度/BV号/时间区间），
> 那三份旧缓存**不会命中**，会重新下载一遍（3 段 × 约 8 秒）。

### 字段怎么来的

| 字段 | 来源 |
|---|---|
| `song_id` | **主键**。渲染时拿它去 `data/arcaea-catalog.json` 补全其余元数据 |
| `difficulty.name` | 难度代号：`PST` / `PRS` / `FTR` / `BYD` / `ETR` |
| `difficulty.level` | **显示等级**（如 `12`），和 `constant` 不一定相同 |
| `difficulty.constant` | 定数（如 `12.0`），参与算分 |
| `score.achievement` | 达成率百分比（`99.84241` = 99.84241%） |
| `score.pure/far/lost` | 判定计数。**三者之和应当等于谱面的物量 `note`** |
| `user.comment` | 评语。**多行用 `\|` 分隔**（和 `text.html` 的注入约定一致） |
| `defaults.clip.start_ms` | 从录像的第几毫秒开始取。默认 55s —— 避开前奏，是 spike 实测的经验值 |
| `defaults.clip.duration_ms` | 取多长。默认 10s —— 对齐社区标准（§7.5：竞品也是 10s） |

### 不在这个文件里的东西（会被自动补全）

这些**不需要你填** —— 渲染时从 `data/arcaea-catalog.json` 查出来：

```
title_en / artist / bpm / set / version / time_sec / bg / jacket
charts[<difficulty>].note / constant / video.sources[]
clip.credit.uploader      ← 来自 catalog 的 owner_name（阶段 0 补上的）
clip.credit.video_url     ← 来自 catalog 的 sources[].url
```

**署名（credit）是自动带出来的**，不用手填 —— 这也是为什么阶段 0 要把它补进数据层。

### 一条有用的自检

`pure + far + lost == note`（物量守恒）是 §13.5 六条自检之一。

本文件里的三首都满足：

| 曲 | pure | far | lost | 合计 | note |
|---|---|---|---|---|---|
| Testify BYD | 2215 | 5 | 1 | **2221** | 2221 |
| Tempestissimo BYD | 1537 | 3 | 0 | **1540** | 1540 |
| Arcana Eden BYD | 2132 | 2 | 0 | **2134** | 2134 |

> 数据可能来自 OCR、可能手抄错 —— 这条自检能在**渲染之前**就把明显不对的抓出来。
> 建议 `cmd/pipeline` 在加载项目文件时就跑一遍，不一致就报错（或至少警告）。

### 怎么加一首

1. 在 catalog 里查 `song_id`：
   ```bash
   node -e "const c=require('./data/arcaea-catalog.json');console.log(c.songs.filter(s=>/关键词/i.test(s.title_en)).map(s=>s.song_id+'  '+s.title_en).join('\n'))"
   ```
2. 确认它有录像来源：
   ```bash
   node -e "const c=require('./data/arcaea-catalog.json');const s=c.songs.find(x=>x.song_id==='你的id');console.log(s.charts.map(ch=>ch.difficulty+' '+(ch.constant??'?')+' 来源'+(ch.video?.sources?.length??0)).join('\n'))"
   ```
3. 加进 `songs[]`，`index` 从 1 递增。

> ⚠️ **一半以上的谱面没有录像**（实测 705/1350）。加之前先确认 `video.sources` 非空，
> 否则那首歌会在渲染时被跳过。
