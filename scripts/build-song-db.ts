#!/usr/bin/env node
/**
 * build-song-db.ts —— 构建 Arcaea 曲库（曲名 / 曲师 / 难度 / 定数 / 物量 / 别名）
 * ============================================================================
 *
 * 为什么需要它
 *   曲库是整个应用的基础设施，至少有三个用途：
 *     1. OCR 纠错 —— 曲名归一化 + 别名表，用于把识别结果拉回真实曲目
 *     2. 定数校验 —— 反解定数、交叉验证 OCR 结果（见 docs/00-方案讨论.md §13.5）
 *     3. 素材关联 —— 把索引里的视频挂到 song_id 上，再取曲绘、算 PTT
 *
 * 数据源（都是 GitHub 托管，可全量离线）
 *   A. arcsong.json        Arcaea-Infinity/ArcaeaSongDatabase   335 首 / 2023-07
 *                          曲名·曲师·BPM·物量 note·**别名 alias[]**·难度定数
 *   B. ChartConstant.json  zh-arc-wiki-re-wg/chart-constant     318 首 / 2023-05
 *                          仅定数，可作交叉校验
 *
 * ⚠️ 两份数据都停留在 2023 年，不含 v5.4 定数大调整之后的变动，也不含 2024–2026 新曲。
 *    运行时会打印告警。补充方式见 docs/00-方案讨论.md §13.3。
 *
 * ⚠️ 关键坑：arcsong.json 的 `difficulties[].difficulty` **不是难度类别，而是「等级×2」**
 *      （Aegleseeker = 10/18/22 → 等级 5/9/11；19/21 → 9+/10+）
 *      难度类别**只能按数组顺序推**：PST / PRS / FTR / BYD / ETR
 *
 * 用法
 *   node scripts/build-song-db.ts
 *   node scripts/build-song-db.ts -o data/arcaea-songdb.json --refresh
 *
 * 依赖：Node.js >= 18（内置 fetch）。零第三方依赖。
 */

import fs from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';
import { pathToFileURL } from 'node:url';

// ---------------------------------------------------------------------------
// 配置
// ---------------------------------------------------------------------------

/** 难度类别按数组顺序排列 —— 不要试图从 `difficulty` 字段推导 */
const DIFFICULTY_CLASSES = ['PST', 'PRS', 'FTR', 'BYD', 'ETR'];

const SOURCES = {
  arcsong: {
    label: 'arcsong.json',
    repo: 'Arcaea-Infinity/ArcaeaSongDatabase',
    file: 'arcsong.json',
    branch: 'main',
  },
  constants: {
    label: 'ChartConstant.json',
    repo: 'zh-arc-wiki-re-wg/chart-constant',
    file: 'ChartConstant.json',
    // 该仓库的默认分支不固定，多试几个
    branch: 'main',
  },
};

/**
 * GitHub 镜像。裸 raw.githubusercontent.com 在部分网络环境不可达，
 * 而 api.github.com 只有 60 次/小时，构建脚本很容易打爆 —— 所以镜像优先。
 */
const MIRRORS: Array<(repo: string, branch: string, file: string) => string> = [
  (repo, branch, file) => `https://gh-proxy.com/https://raw.githubusercontent.com/${repo}/${branch}/${file}`,
  (repo, branch, file) => `https://ghfast.top/https://raw.githubusercontent.com/${repo}/${branch}/${file}`,
  (repo, branch, file) => `https://raw.githubusercontent.com/${repo}/${branch}/${file}`,
];

type SourceKey = keyof typeof SOURCES;

// ---------------------------------------------------------------------------
// 上游 JSON 的最小建模
// ---------------------------------------------------------------------------

/**
 * 两份上游 JSON 都是外部数据，字段随版本变动 —— 只标注本脚本真正读取的字段，
 * 其余不建模；缺字段仍由原来的 `??` 兜底，运行行为不变。
 */
interface RawDifficulty {
  /** ⚠️ 不是难度类别，是「等级 × 2」 */
  difficulty?: number;
  /** 定数 × 10 */
  rating?: number;
  note?: number;
  chart_designer?: string;
  jacket_designer?: string;
  version?: string;
  // 曲级字段被复制在每个难度条目上，取第 0 个即可
  name_en?: string;
  name_jp?: string;
  artist?: string;
  bpm?: string;
  bpm_base?: number;
  set?: string;
  set_friendly?: string;
  side?: number;
  date?: number;
  time?: number;
  remote_download?: boolean;
  bg?: string;
}

interface RawSong extends RawDifficulty {
  song_id?: string;
  difficulties?: RawDifficulty[];
  alias?: string[];
}

/** ChartConstant.json 的值：每个谱面一个条目 */
interface WikiConstantEntry {
  constant?: number;
}

const DEFAULT_UA =
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 ' +
  '(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36';

interface CliArgs {
  out: string;
  cacheDir: string;
  refresh: boolean;
  quiet: boolean;
}

function parseArgs(argv: string[]): CliArgs {
  const args: CliArgs = {
    out: 'data/arcaea-songdb.json',
    cacheDir: '.cache/arcaea-songdb',
    refresh: false,
    quiet: false,
  };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (a === '-o' || a === '--out') args.out = argv[++i];
    else if (a === '--cache-dir') args.cacheDir = argv[++i];
    else if (a === '--refresh') args.refresh = true;
    else if (a === '--quiet') args.quiet = true;
    else if (a === '-h' || a === '--help') {
      console.log('用法: node scripts/build-song-db.ts [-o <输出>] [--refresh]');
      process.exit(0);
    }
  }
  return args;
}

const CFG = parseArgs(process.argv);
const log = (...a: unknown[]) => {
  if (!CFG.quiet) console.log(...a);
};

// ---------------------------------------------------------------------------
// 工具
// ---------------------------------------------------------------------------

/** 与 build-arcaea-index.ts 保持完全一致的归一化，否则两边 join 不上 */
export function normalizeTitle(s: string): string {
  return String(s)
    .normalize('NFKC')
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[\u2018\u2019\u02BC]/g, "'")
    .replace(/[\u201C\u201D]/g, '"')
    .replace(/[\u2010-\u2015\u2212]/g, '-')
    .replace(/\s+/g, ' ')
    .trim()
    .toLowerCase();
}

/**
 * `difficulty` 字段 = 等级 × 2。把它还原成展示用的等级标签。
 *   10 → "5"    19 → "9+"    21 → "10+"    22 → "11"    24 → "12"
 */
export function levelFromCode(code: unknown): string | null {
  if (typeof code !== 'number' || Number.isNaN(code)) return null;
  const lv = code / 2;
  return Number.isInteger(lv) ? String(lv) : `${Math.floor(lv)}+`;
}

async function ensureDir(p: string): Promise<void> {
  await fs.mkdir(p, { recursive: true });
}

async function readCache(name: string): Promise<unknown> {
  if (CFG.refresh) return null;
  try {
    return JSON.parse(await fs.readFile(path.join(CFG.cacheDir, name + '.json'), 'utf8'));
  } catch {
    return null;
  }
}

async function writeCache(name: string, data: unknown): Promise<void> {
  try {
    await ensureDir(CFG.cacheDir);
    await fs.writeFile(path.join(CFG.cacheDir, name + '.json'), JSON.stringify(data));
  } catch {
    /* 缓存失败不影响主流程 */
  }
}

/** 依次尝试各镜像拉取一个 JSON 文件。返回值是外部 JSON，收窄交给调用方。 */
async function fetchJson(fileKey: SourceKey): Promise<{ data: unknown; via: string }> {
  const cached = await readCache(fileKey);
  if (cached) {
    log(`  ${fileKey}: 命中缓存`);
    return { data: cached, via: 'cache' };
  }

  const src = SOURCES[fileKey];
  const branches = fileKey === 'constants' ? [src.branch, 'master'] : [src.branch];
  const errors: string[] = [];

  for (const branch of branches) {
    for (const mk of MIRRORS) {
      const url = mk(src.repo, branch, src.file);
      const via = new URL(url).host;
      try {
        const res = await fetch(url, {
          headers: { 'User-Agent': DEFAULT_UA, Accept: 'application/json, */*' },
          signal: AbortSignal.timeout(30000),
        });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const text = await res.text();
        const data = JSON.parse(text);
        await writeCache(fileKey, data);
        log(`  ${fileKey}: ${via} (${(text.length / 1024).toFixed(0)} KB)`);
        return { data, via: `${via}@${branch}` };
      } catch (e) {
        errors.push(`${via}@${branch}: ${e instanceof Error ? e.message : String(e)}`);
      }
    }
  }
  throw new Error(`拉取 ${fileKey} 失败，已尝试所有镜像：\n    ${errors.join('\n    ')}`);
}

// ---------------------------------------------------------------------------
// 主流程
// ---------------------------------------------------------------------------

async function main() {
  const t0 = Date.now();
  log('▶ 构建 Arcaea 曲库');

  // ---- 拉取 ----
  const { data: arcsongRaw, via: viaA } = await fetchJson('arcsong');
  const { data: ccRaw, via: viaB } = await fetchJson('constants');

  const rawSongs = Array.isArray(arcsongRaw)
    ? (arcsongRaw as RawSong[])
    : ((arcsongRaw as { songs?: RawSong[] }).songs);
  if (!Array.isArray(rawSongs)) throw new Error('arcsong.json 结构异常：找不到 songs 数组');
  log(`  arcsong songs: ${rawSongs.length}`);
  log(`  ChartConstant: ${Object.keys(ccRaw as Record<string, unknown>).length}`);

  // ---- ChartConstant 建索引（键是 song_id）----
  const ccBySongId = new Map<string, WikiConstantEntry[]>();
  for (const [k, v] of Object.entries(ccRaw as Record<string, unknown>)) {
    ccBySongId.set(k, Array.isArray(v) ? (v as WikiConstantEntry[]) : []);
  }

  // ---- 合并 ----
  const songs = [];
  let ccMatched = 0;
  let constantMismatch = 0;

  for (const raw of rawSongs) {
    const songId = raw.song_id;
    if (!songId) continue;

    const diffsRaw = Array.isArray(raw.difficulties) ? raw.difficulties : [];
    if (!diffsRaw.length) continue;

    // 曲级字段从第 0 个难度条目里取（arcsong.json 把它们重复在每个难度上）
    const head = diffsRaw[0];
    const ccSlots = ccBySongId.get(songId);
    if (ccSlots) ccMatched++;

    const difficulties = diffsRaw.map((d, idx) => {
      const cls = DIFFICULTY_CLASSES[idx] ?? `UNKNOWN_${idx}`;
      const constant = typeof d.rating === 'number' ? Number((d.rating / 10).toFixed(2)) : null;

      // 交叉校验：wiki 定数表按同样的数组顺序对齐
      let constantWiki: number | null = null;
      if (ccSlots && ccSlots[idx] && typeof ccSlots[idx].constant === 'number') {
        constantWiki = ccSlots[idx].constant;
      }
      const mismatch =
        constant != null && constantWiki != null && Math.abs(constant - constantWiki) > 1e-9;
      if (mismatch) constantMismatch++;

      return {
        class: cls,
        level: levelFromCode(d.difficulty),
        level_code: d.difficulty, // 原样保留 = 等级×2，便于反查
        constant,
        constant_wiki: constantWiki,
        ...(mismatch ? { constant_mismatch: true } : {}),
        note: d.note ?? null, // 物量，用于判定数守恒校验
        chart_designer: d.chart_designer ?? '',
        jacket_designer: d.jacket_designer ?? '',
        version: d.version ?? '',
      };
    });

    const titleEn = head.name_en || songId;
    const titleJp = head.name_jp || '';
    const alias = Array.isArray(raw.alias) ? raw.alias.filter(Boolean) : [];

    // 用于 join 的归一化键集合：英文名 / 日文名 / 全部别名 / song_id 本身
    const matchKeys = [...new Set(
      [titleEn, titleJp, songId, ...alias].filter(Boolean).map(normalizeTitle)
    )].filter(Boolean);

    songs.push({
      song_id: songId,
      title_en: titleEn,
      title_jp: titleJp,
      title_norm: normalizeTitle(titleEn),
      alias,
      alias_norm: alias.map(normalizeTitle),
      match_keys: matchKeys,
      artist: head.artist ?? '',
      bpm: head.bpm_base ?? (head.bpm ? Number(head.bpm) : null),
      set: head.set ?? '',
      set_friendly: head.set_friendly ?? '',
      side: head.side ?? null,
      version: head.version ?? '',
      date: head.date ?? null,
      time_sec: head.time ?? null,
      remote_download: head.remote_download ?? false,
      // 曲绘：官方 CDN 实测 403，此处仅记录 bg 名（通常与 song_id 同构）
      bg: head.bg ?? songId,
      difficulties,
    });
  }

  // ---- 输出 ----
  const out = {
    schema_version: 1,
    game: 'arcaea',
    generated_at: new Date().toISOString(),
    generator: 'scripts/build-song-db.ts',
    difficulty_class_order: DIFFICULTY_CLASSES,
    freshness_warning:
      '上游数据停留在 2023 年，不含 v5.4 定数大调整后的变动与 2024–2026 新曲，' +
      '请以 wiki 最新值为准并定期重建。',
    sources: [
      { name: SOURCES.arcsong.label, repo: SOURCES.arcsong.repo, fetched_via: viaA, songs: rawSongs.length },
      { name: SOURCES.constants.label, repo: SOURCES.constants.repo, fetched_via: viaB, matched_songs: ccMatched },
    ],
    stats: {
      songs: songs.length,
      songs_with_alias: songs.filter((s) => s.alias.length).length,
      charts: songs.reduce((n, s) => n + s.difficulties.length, 0),
      charts_with_constant: songs.reduce((n, s) => n + s.difficulties.filter((d) => d.constant != null).length, 0),
      constant_mismatch: constantMismatch,
      matched_by_chart_constant: ccMatched,
      class_histogram: songs
        .flatMap((s) => s.difficulties.map((d) => d.class))
        .reduce<Record<string, number>>((m, c) => ((m[c] = (m[c] ?? 0) + 1), m), {}),
      elapsed_sec: Number(((Date.now() - t0) / 1000).toFixed(1)),
    },
    songs,
  };

  await ensureDir(path.dirname(path.resolve(CFG.out)));
  await fs.writeFile(path.resolve(CFG.out), JSON.stringify(out, null, 2) + '\n', 'utf8');

  // ---- 报表 ----
  const s = out.stats;
  log('');
  log('══════════════ 曲库构建完成 ══════════════');
  log(`  输出          ${path.resolve(CFG.out)}`);
  log(`  曲目          ${s.songs} 首（${s.songs_with_alias} 首带别名）`);
  log(`  谱面          ${s.charts} 个（${s.charts_with_constant} 个有定数）`);
  log(`  难度分布      ${Object.entries(s.class_histogram).map(([k, v]) => `${k}:${v}`).join('  ')}`);
  log(`  定数交叉校验  ${s.matched_by_chart_constant} 首命中 wiki 定数表，其中 ${s.constant_mismatch} 个谱面定数不一致`);
  log(`  耗时          ${s.elapsed_sec}s`);
  log('');
  log('  ⚠️  两份上游数据均停留在 2023 年 —— 不含 v5.4 定数大调整之后的变动，');
  log('      也不含 2024–2026 新曲。实际使用前需按 §13.3 补充最新数据。');
  log('══════════════════════════════════════════');
}

// ⚠️ 必须用 pathToFileURL 比较。`import.meta.url === \`file://${argv[1]}\`` 在 Windows 上
//    **恒为 false** —— import.meta.url 是三斜杠的 `file:///D:/...`，而模板串给出 `file://D:\...`。
//    之前还有个 `endsWith('...mjs')` 兜底，改名成 .ts 之后也失效了，
//    结果 isMain 恒 false、main() 永不执行、脚本静默什么都不做（退出码 0）。
const isMain =
  process.argv[1] !== undefined &&
  import.meta.url === pathToFileURL(process.argv[1]).href;
if (isMain) {
  main().catch((e) => {
    console.error('✗ 构建失败：', e?.stack || e);
    process.exit(1);
  });
}
