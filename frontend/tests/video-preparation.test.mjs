import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { mkdtemp, rm } from 'node:fs/promises';
import { createServer } from 'node:http';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';
import { build } from 'vite';

// 在真实 React 与生成绑定上控制响应顺序，不替代 Wails/WebView2 实机验收。
async function checkPreparation({ createElement, createRoot, flushSync, useVideoPreparation }) {
  const equal = (actual, expected, message) => {
    if (JSON.stringify(actual) !== JSON.stringify(expected)) throw Error(`${message}: ${JSON.stringify(actual)} !== ${JSON.stringify(expected)}`);
  };
  const deferred = () => {
    let resolve;
    const promise = new Promise((done) => { resolve = done; });
    return { promise, resolve };
  };
  const song = { song_id: 'test', difficulty: { name: 'FTR' }, video: { url: 'https://www.bilibili.com/video/test' }, clip: { confirmed_key: 'saved' } };
  const state = { index: 1, song_id: song.song_id, difficulty: 'FTR', url: song.video.url, request_id: '100', status: 'queued', confirmed: true, material_ready: false };
  const ready = { ...state, status: 'ready', material_ready: true, preview_ready: true, progress: 1 };
  const failed = { ...ready, status: 'failed', material_ready: false, preview_ready: false, preview_url: '', error: '缓存失效' };
  const passed = [];
  let queries = [];
  let operations = [];
  let listeners = new Set();
  window.go = { app: { App: {
    VideoInfos: () => { const query = deferred(); queries.push(query); return query.promise; },
    PrepareVideo: () => { const operation = deferred(); operations.push(operation); return operation.promise; },
    RetryVideoPreparation: () => { const operation = deferred(); operations.push(operation); return operation.promise; },
    CancelVideoPreparation: () => { emit({ ...ready, request_id: latest.states[0].request_id, status: 'cancelled', material_ready: false }); return Promise.resolve(); },
  } } };
  window.runtime = { EventsOnMultiple: (_, callback) => { listeners.add(callback); return () => listeners.delete(callback); } };
  let root;
  let host;
  let props;
  let latest;
  function Probe() { latest = useVideoPreparation(props.project, props.identity, props.epoch); return null; }
  const render = (next = props) => { props = next; flushSync(() => root.render(createElement(Probe))); };
  const settle = async () => { await new Promise((done) => setTimeout(done, 0)); if (root) render(); };
  const emit = (event) => flushSync(() => { for (const listener of listeners) listener(event); });
  const mount = () => {
    queries = []; operations = []; listeners = new Set();
    host = document.createElement('div'); document.body.append(host); root = createRoot(host);
    render({ project: { songs: [structuredClone(song)] }, identity: 'project', epoch: 1 });
  };
  const unmount = () => { flushSync(() => root.unmount()); root = null; host.remove(); equal(listeners.size, 0, '卸载须解除订阅'); };
  const check = async (name, run) => { mount(); try { await run(); passed.push(name); } finally { if (root) unmount(); } };

  await check('完成事件的全部字段优先于旧查询，保留查询索引', async () => {
    emit({ ...ready, index: 99 });
    queries[0].resolve([{ ...state, confirmed: false, preview_ready: false }]);
    await settle();
    equal(latest.states[0], ready, '旧快照不得覆盖完成事件');
    emit(failed);
    equal(latest.states[0], failed, '后续失败事件须能撤销 ready');
  });
  await check('后续真实查询仍可撤销同 request_id 的缓存就绪', async () => {
    queries[0].resolve([ready]); await settle();
    equal(latest.states[0].material_ready, true, '初始就绪');
    render({ ...props, epoch: 2 });
    queries[1].resolve([failed]); await settle();
    equal(latest.states[0], failed, '不能永久锁住 ready');
  });
  await check('查询缓冲只接受同歌曲、难度、来源和请求，最后事件生效', async () => {
    const stale = [{ song_id: 'other' }, { difficulty: 'BYD' }, { url: 'other-source' }, { request_id: '99' }];
    for (const change of stale) emit({ ...ready, ...change });
    queries[0].resolve([state]); await settle();
    equal(latest.states[0], state, '异任务事件不得进入查询快照');
    for (const change of stale) emit({ ...ready, ...change });
    equal(latest.states[0], state, '异任务事件不得更新已加载状态');
  });
  await check('查询期间最后一个同任务失败事件优先于较早 ready', async () => {
    emit(ready); emit(failed);
    queries[0].resolve([ready]); await settle();
    equal(latest.states[0], failed, '不得以 ready 优先级取代事件顺序');
  });

  const transitions = [
    ['换源', (value) => { value.project.songs[0].video.url = 'other-source'; }],
    ['换本地来源', (value) => { value.project.songs[0].video.local_path = '/other.mp4'; }],
    ['换难度', (value) => { value.project.songs[0].difficulty.name = 'BYD'; }],
    ['换曲', (value) => { value.project.songs[0].song_id = 'other'; }],
    ['重新确认', (value) => { value.project.songs[0].clip.confirmed_key = 'new-saved'; }],
    ['更换项目', (value) => { value.identity = 'other'; }],
    ['新一轮查询', (value) => { value.epoch++; }],
  ];
  for (const [name, change] of transitions) await check(`${name}隔离旧查询、事件和操作响应`, async () => {
    const oldListeners = [...listeners];
    const operation = latest.prepare(song);
    const next = structuredClone(props); change(next); render(next);
    queries[0].resolve([ready]); operations[0].resolve(ready);
    flushSync(() => { for (const listener of oldListeners) listener(ready); });
    await settle();
    equal(await operation, null, '旧操作须被丢弃');
    equal(latest.states, [], '旧状态不得写入新会话');
    const currentSong = next.project.songs[0];
    const current = { ...failed, song_id: currentSong.song_id, difficulty: currentSong.difficulty.name, url: currentSong.video.url };
    queries[1].resolve([current]); await settle();
    equal(latest.states, [current], '新查询须正常生效');
  });

  await check('新 request_id、取消及重试不受旧查询和旧事件覆盖', async () => {
    let operation = latest.retry(song, state);
    const newer = { ...ready, request_id: '101' };
    emit(newer); operations[0].resolve({ ...state, request_id: '101' });
    await operation; await settle();
    queries[0].resolve([state]); await settle(); emit(ready);
    equal(latest.states[0], newer, '新请求完成状态须保留');
    await latest.cancel(newer); await settle();
    equal(latest.states[0].status, 'cancelled', '取消状态须可见');
    operation = latest.retry(song, latest.states[0]);
    const retried = { ...ready, request_id: '102' };
    emit(retried); operations[1].resolve({ ...state, request_id: '102' });
    await operation; await settle(); emit({ ...newer, status: 'failed', material_ready: false });
    equal(latest.states[0], retried, '旧请求失败不得覆盖重试');
  });
  await check('卸载后旧查询、操作和事件均失效', async () => {
    const oldListeners = [...listeners];
    const operation = latest.prepare(song);
    unmount();
    queries[0].resolve([ready]); operations[0].resolve(ready);
    for (const listener of oldListeners) listener(ready);
    equal(await operation, null, '卸载后操作结果须被丢弃');
    await settle(); equal(latest.states, [], '卸载后不得发布旧响应');
  });
  return passed;
}

test('素材查询与事件时序、缓存失效和会话隔离（真实 React）', { timeout: 60000 }, async () => {
  const entry = '\0video-preparation-test';
  const frontend = resolve(import.meta.dirname, '..');
  const bundle = await build({ configFile: false, root: frontend, logLevel: 'silent', plugins: [{
    name: 'video-preparation-test', resolveId: (id) => id === entry ? entry : undefined,
    load: (id) => id === entry ? `import { createElement } from 'react'; import { createRoot } from 'react-dom/client'; import { flushSync } from 'react-dom'; import { useVideoPreparation } from ${JSON.stringify(join(frontend, 'src/useVideoPreparation.ts'))};
      (${checkPreparation.toString()})({ createElement, createRoot, flushSync, useVideoPreparation }).then(passed => fetch('/result', { method: 'POST', body: JSON.stringify({ passed }) })).catch(error => fetch('/result', { method: 'POST', body: JSON.stringify({ error: error.stack }) }));` : undefined,
  }], build: { write: false, rollupOptions: { input: entry, output: { inlineDynamicImports: true } } } });
  const script = bundle.output.find((output) => output.type === 'chunk').code;
  let finish;
  const result = new Promise((done) => { finish = done; });
  const server = createServer(async (request, response) => {
    if (request.url === '/result') {
      let body = ''; for await (const chunk of request) body += chunk;
      finish(JSON.parse(body)); response.end();
    } else if (request.url === '/test.js') {
      response.setHeader('Content-Type', 'text/javascript'); response.end(script);
    } else response.end('<!doctype html><script type="module" src="/test.js"></script>');
  });
  const directory = await mkdtemp(join(tmpdir(), 'video-preparation-'));
  let browser;
  let timer;
  try {
    server.listen(0, '127.0.0.1'); await once(server, 'listening');
    browser = spawn(process.env.QA_CHROMIUM || '/usr/bin/chromium', ['--headless=new', '--no-sandbox', '--disable-gpu', `--user-data-dir=${directory}`, `http://127.0.0.1:${server.address().port}`], { stdio: 'ignore' });
    const exit = once(browser, 'exit').then(([code]) => { throw Error(`Chromium 提前退出：${code}`); });
    const timeout = new Promise((_, reject) => { timer = setTimeout(() => reject(Error('等待浏览器断言超时')), 45000); });
    const outcome = await Promise.race([result, exit, timeout]);
    assert.equal(outcome.error, undefined, outcome.error);
    assert.equal(outcome.passed.length, 13);
    for (const name of outcome.passed) console.log(`PASS：${name}`);
  } finally {
    clearTimeout(timer);
    if (browser && browser.exitCode === null && browser.signalCode === null) { browser.kill('SIGTERM'); await once(browser, 'exit'); }
    server.closeAllConnections(); await new Promise((done) => server.close(done));
    await rm(directory, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  }
});
