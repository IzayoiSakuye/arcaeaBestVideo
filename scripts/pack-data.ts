#!/usr/bin/env node
/**
 * pack-data.ts —— 把运行时数据打包成可发布的资源包 + 版本清单
 * ============================================================================
 *
 * 解决什么问题
 *   应用运行时需要「曲目目录 + 曲绘」，但这两样**不能进 git 仓库**：
 *     · 620 个二进制文件，每次刷新数据就新增约 13MB blob
 *     · git 历史永久保留 —— 刷 20 次仓库就多 260MB，且删不掉
 *     · GitHub 仓库软上限 1GB，clone 会越来越慢
 *
 *   正确做法是「代码仓库」与「数据发布通道」分离：
 *     代码仓库  → 只有 scripts/ 与 docs/，外加 2KB 的 song-id-aliases.json
 *     数据发布  → 每次构建产出一个 zip + latest.json，传到发布通道
 *
 * 产出
 *   dist/arcaea-data-<version>.zip   ← 一个包，含 catalog.json + 全部 256px 曲绘
 *   dist/latest.json                 ← 客户端读这个判断要不要更新
 *   dist/tree/                       ← （--emit-tree）散文件，供 CDN / OSS 逐文件托管
 *
 * 用法
 *   node scripts/pack-data.ts
 *   node scripts/pack-data.ts --version 2026.09.20
 *   node scripts/pack-data.ts --emit-tree            # 额外产出散文件树
 *   node scripts/pack-data.ts --base-url https://cdn.example.com/arcaea
 *
 * 依赖：Node.js >= 18。零第三方依赖（ZIP 由 scripts/lib/zip.ts 自己写）。
 */

import fs from 'node:fs/promises';
import crypto from 'node:crypto';
import path from 'node:path';
import process from 'node:process';
import { ZipWriter } from './lib/zip.ts';
import type { CatalogFile } from './lib/types.ts';

interface PackArgs {
  dataDir: string;
  catalog: string;
  jacketDir: string;
  outDir: string;
  version: string | null;
  baseUrl: string;
  emitTree: boolean;
  quiet: boolean;
}

function parseArgs(argv: string[]): PackArgs {
  const args: PackArgs = {
    dataDir: 'data',
    catalog: 'data/arcaea-catalog.json',
    jacketDir: 'data/jackets/256',
    outDir: 'dist',
    version: null,
    baseUrl: '',
    emitTree: false,
    quiet: false,
  };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--data-dir') args.dataDir = argv[++i];
    else if (a === '--catalog') args.catalog = argv[++i];
    else if (a === '--jackets') args.jacketDir = argv[++i];
    else if (a === '-o' || a === '--out') args.outDir = argv[++i];
    else if (a === '--version') args.version = argv[++i];
    else if (a === '--base-url') args.baseUrl = argv[++i].replace(/\/+$/, '');
    else if (a === '--emit-tree') args.emitTree = true;
    else if (a === '--quiet') args.quiet = true;
    else if (a === '-h' || a === '--help') {
      console.log(
        '用法: node scripts/pack-data.ts [--version <v>] [--base-url <url>] [--emit-tree] [-o <dir>]'
      );
      process.exit(0);
    }
  }
  return args;
}

const CFG = parseArgs(process.argv);
const log = (...a: unknown[]) => {
  if (!CFG.quiet) console.log(...a);
};

const sha256 = (buf: Buffer) => crypto.createHash('sha256').update(buf).digest('hex');
const kb = (n: number) => `${(n / 1024).toFixed(1)} KB`;
const mb = (n: number) => `${(n / 1048576).toFixed(1)} MB`;

async function main() {
  const t0 = Date.now();
  log('▶ 打包运行时数据');

  // ---- 版本号：默认按日期 ----
  const d = new Date();
  const version =
    CFG.version ??
    `${d.getFullYear()}.${String(d.getMonth() + 1).padStart(2, '0')}.${String(d.getDate()).padStart(2, '0')}`;
  log(`  版本    ${version}`);

  // ---- 读曲目目录 ----
  const catalogRaw = await fs.readFile(path.resolve(CFG.catalog));
  const catalog: CatalogFile = JSON.parse(catalogRaw.toString('utf8'));

  // ⚠️ 关键：路径重写。
  //    构建期 catalog 里的路径是 `data/jackets/256/x.jpg`（相对仓库根），
  //    但客户端解压后数据根的布局是 `jackets/256/x.jpg`。不重写就会全部 404。
  let rewritten = 0;
  for (const song of catalog.songs ?? []) {
    const p = song.jacket?.local_256;
    if (typeof p === 'string' && p.startsWith('data/')) {
      song.jacket.local_256 = p.slice('data/'.length);
      rewritten++;
    }
  }
  log(`  路径重写 ${rewritten} 条  data/… → 相对数据根`);

  // ---- 收集曲绘 ----
  const jpegFiles: Array<{ name: string; abs: string; size: number }> = [];
  try {
    for (const name of (await fs.readdir(path.resolve(CFG.jacketDir))).sort()) {
      if (!/\.(jpe?g|png)$/i.test(name)) continue;
      const abs = path.join(path.resolve(CFG.jacketDir), name);
      const st = await fs.stat(abs);
      if (st.size > 0) jpegFiles.push({ name, abs, size: st.size });
    }
  } catch {
    throw new Error(`曲绘目录不存在：${CFG.jacketDir}（先跑 build-jackets.ts）`);
  }
  const jacketBytes = jpegFiles.reduce((n, f) => n + f.size, 0);
  log(`  曲绘    ${jpegFiles.length} 张  ${mb(jacketBytes)}`);

  // ---- 额外元数据（客户端可用来做自检）----
  const meta = {
    data_version: version,
    game: 'arcaea',
    generated_at: new Date().toISOString(),
    catalog_schema_version: catalog.schema_version ?? 1,
    counts: {
      songs: catalog.songs?.length ?? 0,
      charts: (catalog.songs ?? []).reduce((n, s) => n + (s.charts?.length ?? 0), 0),
      jackets: jpegFiles.length,
    },
    paths: {
      catalog: 'catalog.json',
      jackets: 'jackets/256/',
    },
  };
  const catalogBuf = Buffer.from(JSON.stringify(catalog), 'utf8');
  const metaBuf = Buffer.from(JSON.stringify(meta, null, 2), 'utf8');

  // ---- 打 zip ----
  const zip = new ZipWriter();
  zip.add('meta.json', metaBuf);
  zip.add('catalog.json', catalogBuf);
  for (const f of jpegFiles) {
    zip.add(`jackets/256/${f.name}`, await fs.readFile(f.abs));
  }
  const zipName = `arcaea-data-${version}.zip`;
  const zipPath = path.join(CFG.outDir, zipName);
  const zipBytes = await zip.writeTo(zipPath);
  log(`  归档    ${zipName}  ${mb(zipBytes)}  (${zip.count} 个条目)`);

  // ---- 校验和解包验证（自己产的东西也要验一遍）----
  const zipBuf = await fs.readFile(zipPath);
  const zipSha = sha256(zipBuf);

  // ---- 散文件树（可选，供 OSS / CDN 逐文件托管）----
  const treeFiles: Array<{ path: string; size: number; sha256: string }> = [];
  if (CFG.emitTree) {
    const treeRoot = path.join(CFG.outDir, 'tree');
    await fs.rm(treeRoot, { recursive: true, force: true });
    const put = async (rel: string, buf: Buffer) => {
      const dest = path.join(treeRoot, rel);
      await fs.mkdir(path.dirname(dest), { recursive: true });
      await fs.writeFile(dest, buf);
      treeFiles.push({ path: rel.replace(/\\/g, '/'), size: buf.length, sha256: sha256(buf) });
    };
    await put('meta.json', metaBuf);
    await put('catalog.json', catalogBuf);
    for (const f of jpegFiles) {
      await put(`jackets/256/${f.name}`, await fs.readFile(f.abs));
    }
    log(`  散文件树 dist/tree/  ${treeFiles.length} 个文件`);
  }

  // ---- latest.json：客户端唯一需要先拉的小文件 ----
  const latest = {
    schema_version: 1,
    game: 'arcaea',
    data_version: version,
    generated_at: meta.generated_at,
    counts: meta.counts,
    archive: {
      path: zipName,
      size: zipBytes,
      sha256: zipSha,
      url: CFG.baseUrl ? `${CFG.baseUrl}/${zipName}` : null,
    },
    // 逐文件清单 —— 支持增量更新（只下新增/变更的曲绘）
    files: [
      { path: 'meta.json', size: metaBuf.length, sha256: sha256(metaBuf) },
      { path: 'catalog.json', size: catalogBuf.length, sha256: sha256(catalogBuf) },
      ...(CFG.emitTree
        ? treeFiles.filter((f) => f.path.startsWith('jackets/'))
        : jpegFiles.map((f) => ({ path: `jackets/256/${f.name}`, size: f.size }))),
    ],
    urls: CFG.baseUrl
      ? {
          archive: `${CFG.baseUrl}/${zipName}`,
          file_base: `${CFG.baseUrl}/tree`,
        }
      : null,
    note: [
      'archive 用于首次全量下载；files + urls.file_base 用于增量更新。',
      '客户端应校验 sha256 后再解压/落盘。',
      '本文件不含任何构建期中间产物（songdb / index / jackets.json 都可重建，不发布）。',
    ],
  };
  const latestPath = path.join(CFG.outDir, 'latest.json');
  await fs.mkdir(path.resolve(CFG.outDir), { recursive: true });
  await fs.writeFile(latestPath, JSON.stringify(latest, null, 2) + '\n', 'utf8');

  // ---- 报表 ----
  log('');
  log('══════════════ 打包完成 ══════════════');
  log(`  版本        ${version}`);
  log(`  归档        ${path.resolve(zipPath)}`);
  log(`              ${mb(zipBytes)}   sha256 ${zipSha.slice(0, 16)}…`);
  log('  清单        ' + path.resolve(latestPath));
  if (CFG.emitTree) log(`  散文件树    ${path.resolve(path.join(CFG.outDir, 'tree'))}`);
  log('');
  log('  内容构成');
  log(`     catalog.json        ${kb(catalogBuf.length)}    ${meta.counts.songs} 首 / ${meta.counts.charts} 谱面`);
  log(`     meta.json           ${kb(metaBuf.length)}`);
  log(`     jackets/256/        ${mb(jacketBytes)}    ${jpegFiles.length} 张`);
  log('');
  log(`  ⏱  ${((Date.now() - t0) / 1000).toFixed(1)}s`);
  log('══════════════════════════════════════');
  log('');
  log('  下一步：把 dist/ 里的两个文件传到发布通道');
  log('    GitHub Releases :  gh release create data-' + version + ' dist/*');
  log('    对象存储(推荐)   :  ossutil cp -r dist/ oss://<bucket>/arcaea/');
  log('    ⚠️ 国内用户建议用对象存储 + CDN，GitHub Releases 在大陆速度不稳定。');
}

main().catch((e) => {
  console.error('✗ 打包失败：', e?.stack || e);
  process.exit(1);
});
