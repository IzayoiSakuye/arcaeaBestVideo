import { useEffect, useRef, useState } from 'react';
import { CancelRender, CheckTool, Import, Jacket, PickFile, PickOutput, RevealOut, Review, SearchSongs, SetToolPath, StartRender, Status } from '../wailsjs/go/app/App';
import { EventsOn } from '../wailsjs/runtime/runtime';
import type { app, score } from '../wailsjs/go/models';
import { eventText, nameOf, scoreText, type Event } from './format';

const pages = ['环境检查', '导入图片 / JSON', '填写感想', '识别信息', '编排顺序', '生成视频', '完成与导出'];
const toolNames = ['ffmpeg', 'ffprobe', 'yt-dlp', 'edge'];
const clearTypes = ['TL', 'NC', 'HC', 'EC', 'FR', 'PM'];
const diffNames = ['PST', 'PRS', 'FTR', 'BYD', 'ETR'];
type LogEntry = Event & { time: string };

function errorText(error: unknown) { return error instanceof Error ? error.message : String(error); }

function App() {
  const [page, setPage] = useState(0);
  const [status, setStatus] = useState<app.AppStatus | null>(null);
  const [checks, setChecks] = useState<Record<string, { result?: string; error?: string; busy?: boolean }>>({});
  const [importMode, setImportMode] = useState<'image' | 'json'>('image');
  const [importPath, setImportPath] = useState('');
  const [jsonText, setJsonText] = useState('');
  const [importBusy, setImportBusy] = useState(false);
  const [importError, setImportError] = useState('');
  const [notes, setNotes] = useState<string[]>([]);
  const [judgmentTexts, setJudgmentTexts] = useState<Record<string, string>>({});
  const [scoreTexts, setScoreTexts] = useState<Record<number, string>>({});
  const [jacket, setJacket] = useState('');
  const [project, setProject] = useState<score.Project | null>(null);
  const [review, setReview] = useState<app.ReviewResult | null>(null);
  const [reviewError, setReviewError] = useState('');
  const [songIndex, setSongIndex] = useState(0);
  const [onlyProblems, setOnlyProblems] = useState(false);
  const [search, setSearch] = useState('');
  const [choices, setChoices] = useState<app.SongChoice[]>([]);
  const [output, setOutput] = useState('');
  const [renderState, setRenderState] = useState<'idle' | 'starting' | 'running' | 'failed' | 'done'>('idle');
  const [renderError, setRenderError] = useState('');
  const [jobID, setJobID] = useState('');
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [finished, setFinished] = useState('');
  const outputRef = useRef('');
  const reviewSeq = useRef(0);

  useEffect(() => {
    Status().then(setStatus).catch(error => setImportError(`环境状态读取失败：${errorText(error)}`));
    return EventsOn('job:event', (event: Event) => {
      setLogs(old => [...old, { ...event, time: new Date().toLocaleTimeString('zh-CN', { hour12: false }) }]);
      if (event.stage === 'done' && event.status === 'done') {
        setFinished(outputRef.current);
        setRenderState('done');
        setPage(6);
      } else if (event.status === 'failed') {
        setRenderError(event.error || event.message || '生成失败');
        setRenderState('failed');
        setPage(5);
      }
    });
  }, []);

  useEffect(() => {
    if (!project) { setReview(null); return; }
    const seq = ++reviewSeq.current;
    const timer = window.setTimeout(() => {
      Review(project).then(result => {
        if (seq === reviewSeq.current) { setReview(result); setReviewError(''); }
      }).catch(error => {
        if (seq === reviewSeq.current) { setReview(null); setReviewError(errorText(error)); }
      });
    }, 220);
    return () => window.clearTimeout(timer);
  }, [project]);

  useEffect(() => {
    if (!search.trim()) { setChoices([]); return; }
    let current = true;
    const timer = window.setTimeout(() => SearchSongs(search, 8).then(result => {
      if (current) setChoices(result);
    }).catch(error => { if (current) setReviewError(errorText(error)); }), 180);
    return () => { current = false; window.clearTimeout(timer); };
  }, [search]);

  useEffect(() => {
    const current = project?.songs[songIndex];
    if (!current?.song_id || !current.difficulty.name) { setJacket(''); return; }
    let active = true;
    setJacket('');
    Jacket(current.song_id, current.difficulty.name).then(value => { if (active) setJacket(value); }).catch(() => { if (active) setJacket(''); });
    return () => { active = false; };
  }, [project?.songs[songIndex]?.song_id, project?.songs[songIndex]?.difficulty.name]);

  async function refreshStatus() { setStatus(await Status()); }
  async function testTool(name: string) {
    setChecks(old => ({ ...old, [name]: { busy: true } }));
    try {
      const result = await CheckTool(name);
      setChecks(old => ({ ...old, [name]: { result: result.result } }));
    } catch (error) { setChecks(old => ({ ...old, [name]: { error: errorText(error) } })); }
  }
  async function chooseTool(name: string) {
    try {
      const path = await PickFile();
      if (!path) return;
      setChecks(old => ({ ...old, [name]: { busy: true } }));
      const result = await SetToolPath(name, path);
      setChecks(old => ({ ...old, [name]: { result: result.result } }));
      await refreshStatus();
    } catch (error) { setChecks(old => ({ ...old, [name]: { error: errorText(error) } })); }
  }
  async function autoTool(name: string) {
    try {
      setChecks(old => ({ ...old, [name]: { busy: true } }));
      const result = await SetToolPath(name, '');
      setChecks(old => ({ ...old, [name]: { result: result.result } }));
      await refreshStatus();
    } catch (error) { setChecks(old => ({ ...old, [name]: { error: errorText(error) } })); }
  }
  async function chooseImport() {
    try { const path = await PickFile(); if (path) { setImportPath(path); if (importMode === 'json') setJsonText(''); } }
    catch (error) { setImportError(errorText(error)); }
  }
  async function doImport() {
    if (renderState === 'starting' || renderState === 'running') return;
    setImportError('');
    setImportBusy(true);
    try {
      const draft = await Import(importMode, importPath, importMode === 'json' ? jsonText : '');
      if (!draft.Project) throw new Error('导入结果没有成绩数据');
      setProject(draft.Project);
      setNotes(draft.Notes || []);
      setJudgmentTexts({});
      setScoreTexts({});
      setSongIndex(0);
      setReview(null);
      setFinished('');
      setOutput('');
      setRenderState('idle');
      setRenderError('');
      setLogs([]);
      setJobID('');
      setPage(3);
    } catch (error) { setImportError(errorText(error)); }
    finally { setImportBusy(false); }
  }
  function editSong(edit: (song: score.Song) => void) {
    if (!project) return;
    const copy = structuredClone(project);
    edit(copy.songs[songIndex]);
    reviewSeq.current++;
    setReview(null);
    setProject(copy);
  }
  function jumpProblem(index: number) { if (project && index > 0 && index <= project.songs.length) { setSongIndex(index - 1); setPage(3); } }
  function stepSong(direction: number) {
    if (!project) return;
    const indexes = onlyProblems
      ? [...new Set((review?.problems || []).map(p => p.song_index).filter(i => i > 0))].map(i => i - 1).sort((a, b) => a - b)
      : project.songs.map((_, i) => i);
    const next = direction > 0 ? indexes.find(i => i > songIndex) : [...indexes].reverse().find(i => i < songIndex);
    if (next !== undefined) setSongIndex(next);
  }
  async function pickOutput() {
    try { const path = await PickOutput(); if (path) { setOutput(path); setRenderError(''); } }
    catch (error) { setRenderError(errorText(error)); }
  }
  async function startRender() {
    if (!review?.project || !output) return;
    setRenderError('');
    setLogs([]);
    setRenderState('starting');
    setFinished('');
    outputRef.current = output;
    try {
      let replace = false;
      try {
        const result = await StartRender(review.project, output, replace);
        setJobID(result.job_id);
        outputRef.current = result.output;
        setRenderState(state => state === 'starting' ? 'running' : state);
      } catch (error) {
        const message = errorText(error);
        if (!message.includes('输出文件已存在，请确认覆盖')) throw error;
        if (!window.confirm(`输出文件已存在，确认覆盖？\n${output}`)) { setRenderState('idle'); return; }
        replace = true;
        const result = await StartRender(review.project, output, replace);
        setJobID(result.job_id);
        outputRef.current = result.output;
        setRenderState(state => state === 'starting' ? 'running' : state);
      }
    } catch (error) { setRenderError(errorText(error)); setRenderState('failed'); }
  }
  async function cancelRender() {
    try { await CancelRender(jobID); }
    catch (error) { setRenderError(errorText(error)); }
  }

  const count = project?.songs.length || 0;
  const row = review?.rows?.[songIndex];
  const song = project?.songs[songIndex];
  const ownProblems = (review?.problems || []).filter(problem => problem.song_index === songIndex + 1);
  const globalProblems = (review?.problems || []).filter(problem => problem.song_index === 0);
  const doneCount = new Set(logs.filter(event => event.stage === 'render' && ['done', 'skipped'].includes(event.status)).map(event => event.song_index)).size;
  const active = renderState === 'starting' || renderState === 'running';

  return <div className="shell">
    <header className="topbar"><div className="brand-mark">◈</div><div><strong>回忆视频生成器</strong><small>ARCAEA MEMORY VIDEO MAKER</small></div><span className="top-version">BEST 50 · VIDEO STUDIO</span></header>
    <div className="workspace">
      <aside className="sidebar"><div className="sidebar-title">制作流程 <span>WORKFLOW</span></div><nav aria-label="制作流程">{pages.map((label, index) =>
        <button key={label} className={`nav-item ${page === index ? 'selected' : ''} ${[2, 4].includes(index) ? 'future' : ''}`} onClick={() => setPage(index)} aria-current={page === index ? 'page' : undefined}>
          <span className="nav-number">{String(index + 1).padStart(2, '0')}</span><span className="nav-label">{label}{[2, 4].includes(index) && <small>待开发</small>}</span><span className="nav-dot">{page === index ? '●' : '○'}</span>
        </button>)}</nav><div className="sidebar-foot">当前版本 · Arcaea</div></aside>
      <main className="main-area"><div className="page-heading"><div><span className="eyebrow">STEP {String(page + 1).padStart(2, '0')} / 07</span><h1>{pages[page]}</h1></div><span className="heading-tag">{page === 3 && count ? `第 ${songIndex + 1} / ${count} 首` : 'ARCAEA · BEST 50'}</span></div>

      {page === 0 && <div className="content-grid"><section className="card wide"><h2>工具检测 <span>01 / 04</span></h2><p className="muted">自动检测工具；找不到时点「⋯」选择文件。</p><div className="tool-list">{toolNames.map(name => {
        const found = status?.tools?.find(tool => tool.Name === name);
        const check = checks[name];
        return <div className="tool-row" key={name}><div className="tool-identity"><span className="tool-icon">{name === 'edge' ? 'E' : '⌘'}</span><div><strong>{name === 'edge' ? 'Microsoft Edge' : name}</strong><small>{name === 'yt-dlp' ? 'P3 素材下载使用' : name === 'edge' ? '文字渲染使用' : '视频处理使用'}</small></div></div><div className="tool-path" title={found?.Path || ''}>{found?.Path || '尚未找到可执行文件'}</div><div className="tool-actions"><button className="button quiet pick-tool" onClick={() => chooseTool(name)} disabled={check?.busy} title="手动选择工具文件" aria-label={`手动选择${name}工具文件`}>⋯</button><button className="button quiet" onClick={() => testTool(name)} disabled={check?.busy} title="实际运行工具，验证是否可用">验证可用</button><button className="button quiet" onClick={() => autoTool(name)} disabled={check?.busy} title="清除手动路径，重新定位并验证工具">自动检测</button></div><div className={`tool-result ${check?.error ? 'bad' : check?.result ? 'good' : ''}`}>{check?.busy ? '检测中…' : check?.error || check?.result || (found?.Path ? '待测试' : '未找到')}</div></div>;
      })}</div>{status?.data_error && <div className="notice bad">数据包：{status.data_error}</div>}{status?.tool_error && <div className="notice bad">工具配置：{status.tool_error}</div>}</section><aside className="card side-card"><h2>当前状态</h2><div className="status-figure"><strong>{status?.song_count ?? '—'} <small>/ {status && !status.data_error ? status.song_count : '—'}</small></strong><span>已载入 / 数据包共计（首）</span></div><div className="rule"/><p>三种工具可在这里一次配置。Edge 用于文字渲染，也需要检查。</p><p>yt-dlp 将在 P3 素材下载功能接入后使用。</p></aside></div>}

      {page === 1 && <div className="content-grid"><section className="card wide"><h2>导入方式</h2><div className="segmented"><button className={importMode === 'image' ? 'active' : ''} onClick={() => { setImportMode('image'); setImportPath(''); setImportError(''); }}>图片导入</button><button className={importMode === 'json' ? 'active' : ''} onClick={() => { setImportMode('json'); setImportPath(''); setImportError(''); }}>JSON 导入</button></div><div className="file-box"><span className="file-glyph">↑</span><strong>{importPath ? nameOf(importPath) : importMode === 'image' ? '选择一张 B50 成绩图' : '选择 JSON 文件'}</strong>{importMode === 'json' && <small>也可以在下方直接粘贴 JSON</small>}<button className="button quiet" onClick={chooseImport}>选择文件</button></div>{importMode === 'json' && <label className="field full">粘贴 JSON<textarea value={jsonText} onChange={event => { setJsonText(event.target.value); setImportPath(''); }} placeholder="在此粘贴成绩 JSON…" rows={7}/></label>}<div className="action-line"><button className="button primary" disabled={importBusy || active || (!importPath && !(importMode === 'json' && jsonText.trim()))} onClick={doImport}>{importBusy ? '正在导入…' : '导入并校对 →'}</button></div>{importError && <div className="notice bad" role="alert">{importError}</div>}</section><aside className="card side-card"><h2>导入结果</h2>{project ? <><div className="status-figure"><strong>{count}</strong><span>首成绩已导入</span></div><div className="rule"/><p>请在「识别信息」校对成绩。</p></> : <p className="empty-aside">尚未导入成绩</p>}</aside></div>}

      {(page === 2 || page === 4) && <section className="card placeholder"><span className="placeholder-symbol">◇</span><h2>{pages[page]}</h2><p>待开发</p></section>}

      {page === 3 && (song ? <><div className="review-toolbar"><label className="field compact">跳转到第 <input type="number" min={1} max={count} value={songIndex + 1} onChange={event => { const value = Number(event.target.value); if (value >= 1 && value <= count) setSongIndex(value - 1); }}/>{count} 首</label><label className="checkline"><input type="checkbox" checked={onlyProblems} onChange={event => setOnlyProblems(event.target.checked)}/> 只看问题行</label><div className="spacer"/><button className="button quiet" onClick={() => stepSong(-1)}>← 上一首</button><button className="button quiet" onClick={() => stepSong(1)}>下一首 →</button></div><div className="content-grid review-grid"><section className="card wide"><h2>成绩信息 <span>#{String(songIndex + 1).padStart(2, '0')}</span></h2><div className="song-title">{jacket ? <img className="jacket-placeholder" src={jacket} alt={row?.title ? `曲绘：${row.title}` : '曲绘'}/> : <div className="jacket-placeholder" aria-label="暂无曲绘">♫</div>}<div><span className="eyebrow">SONG PROFILE</span><h3>{row?.title || song.song_id || '未匹配曲目'}</h3><p>{row?.artist || '请核对曲目匹配'}</p></div></div><div className="form-grid"><div className="field full"><span>曲目匹配</span><div className="inline-field"><input value={search} onChange={event => setSearch(event.target.value)} placeholder={song.song_id || '搜索歌名或 song_id'}/><span className="current-id">当前：{song.song_id || '未匹配'}</span></div>{choices.length > 0 && <div className="search-results">{choices.map(choice => <button key={choice.song_id} onClick={() => { editSong(item => { item.song_id = choice.song_id; item.difficulty.name = choice.difficulties[0] || ''; item.difficulty.constant = 0; item.difficulty.level = ''; }); setSearch(''); }}>{choice.title}<small>{choice.artist} · {choice.song_id}</small></button>)}</div>}</div><label className="field">难度<select value={song.difficulty.name || ''} onChange={event => editSong(item => { item.difficulty.name = event.target.value; item.difficulty.constant = 0; item.difficulty.level = ''; })}><option value="">请选择</option>{[...new Set([song.difficulty.name, ...diffNames].filter(Boolean))].map(name => <option key={name} value={name}>{name}</option>)}</select></label><label className="field">分数<input type="number" min={0} max={10000000} value={scoreTexts[songIndex] ?? scoreText(song.score.achievement)} onChange={event => { setScoreTexts(old => ({ ...old, [songIndex]: event.target.value })); editSong(item => { item.score.achievement = Number(event.target.value) / 100000; }); }}/></label><label className="field">通关类型<select value={song.score.clear_type || ''} onChange={event => editSong(item => { item.score.clear_type = event.target.value; })}><option value="">未填写</option>{[...new Set([song.score.clear_type, ...clearTypes].filter(Boolean))].map(name => <option key={name} value={name}>{name}</option>)}</select></label><div className="field">单曲 PTT<div className="readout">{row?.ptt == null ? '待计算' : row.ptt.toFixed(3)}</div></div><div className="field">曲库定数<div className="readout">{row?.constant == null ? '未匹配' : row.constant}</div></div><div className="field">评级<div className="readout">{row?.rank || '待计算'}</div></div><div className="judgment-row full">{(['pure', 'far', 'lost'] as const).map(key => <label className="field" key={key}>{key.toUpperCase()}<input type="number" min={0} value={judgmentTexts[songIndex + '.' + key] ?? (project?.source.type === 'image' && song.score[key] === 0 ? '' : song.score[key])} placeholder={project?.source.type === 'image' ? '未读取' : ''} onChange={event => { setJudgmentTexts(old => ({ ...old, [songIndex + '.' + key]: event.target.value })); editSong(item => { item.score[key] = Number(event.target.value); }); }}/></label>)}</div><label className="field full">单曲评语<textarea rows={4} maxLength={500} value={song.user.comment} onChange={event => editSong(item => { item.user.comment = event.target.value; })} placeholder="写下关于这首歌的回忆…"/></label></div></section><aside className="right-stack"><section className="card"><h2>校对结果 <span>{ownProblems.length} 项</span></h2>{reviewError && <div className="notice bad">{reviewError}</div>}{ownProblems.length ? <div className="problem-list">{ownProblems.map((problem, index) => <div className={`problem ${problem.severity}`} key={`${problem.rule}-${index}`}><b>{problem.severity.toUpperCase()}</b><span>{problem.message}</span></div>)}</div> : <p className="muted">{review ? '本首暂无问题' : '正在校对…'}</p>}</section>{notes.length > 0 && <section className="card"><h2>识别问题 <span>{notes.length} 项</span></h2><ul className="note-list">{notes.map((note, index) => <li key={index}>{note}</li>)}</ul></section>}<section className="card"><h2>待开发</h2><div className="future-tile">视频预览与展示片段 <span>P3</span></div><div className="future-tile">主题选择 <span>P4</span></div></section></aside></div>{globalProblems.length > 0 && <div className="notice warn">全局问题：{globalProblems.map(problem => problem.message).join('；')}</div>}{review?.problems && review.problems.some(problem => problem.song_index > 0 && problem.song_index !== songIndex + 1) && <section className="card problem-overview"><h2>其他待校对歌曲</h2><div className="problem-chips">{[...new Set(review.problems.filter(problem => problem.song_index > 0 && problem.song_index !== songIndex + 1).map(problem => problem.song_index))].map(index => <button key={index} onClick={() => jumpProblem(index)}>第 {index} 首</button>)}</div></section>}</> : <section className="card placeholder"><span className="placeholder-symbol">⌑</span><h2>尚无成绩</h2><p>请先在「导入图片 / JSON」导入一份成绩。</p><button className="button primary" onClick={() => setPage(1)}>前往导入</button></section>)}

      {page === 5 && <div className="content-grid"><section className="card wide"><h2>生成任务</h2><div className="output-picker"><div><span>输出文件</span><strong>{output ? nameOf(output) : '尚未选择 MP4 路径'}</strong><small title={output}>{output || '开始生成前请选择保存位置'}</small></div><button className="button quiet" onClick={pickOutput} disabled={active}>选择路径</button></div><div className="progress-head"><strong>{renderState === 'done' ? '已完成' : active ? '正在生成' : renderState === 'failed' ? '生成失败' : '等待开始'}</strong><span>{doneCount} / {count} 段完成</span></div><div className="progress-track"><div style={{ width: count ? `${Math.min(100, doneCount / count * 100)}%` : '0%' }}/></div><div className="action-line"><button className="button primary" disabled={!review?.renderable || !output || active} onClick={startRender}>开始生成</button>{active && <button className="button quiet" disabled={!jobID} onClick={cancelRender}>取消生成</button>}</div>{!project && <div className="notice warn">请先导入并校对成绩。</div>}{review && !review.renderable && <div className="notice warn">成绩存在 fatal 问题，请先在「识别信息」校对。</div>}{renderError && <div className="notice bad" role="alert">{renderError}</div>}</section><aside className="card log-card"><h2>运行记录 <span>{logs.length}</span></h2><div className="log-list" role="log" aria-live="polite">{logs.length ? logs.map((event, index) => <div className={`log-entry ${event.status}`} key={index}><time>{event.time}</time><span>{eventText(event)}</span></div>) : <div className="log-empty">生成开始后，这里会显示实际任务事件。</div>}</div></aside></div>}

      {page === 6 && (finished ? <section className="card completion"><div className="completion-icon">✓</div><span className="eyebrow">RENDER COMPLETE</span><h2>视频已生成</h2><p>成片已保存到所选位置。</p><div className="completed-file"><span>MP4 文件</span><strong>{nameOf(finished)}</strong><small>{finished}</small></div><div className="completion-actions"><button className="button primary" onClick={() => RevealOut(finished).catch(error => setRenderError(errorText(error)))}>打开文件路径</button><button className="button quiet" onClick={() => setPage(0)}>返回首页</button></div>{renderError && <div className="notice bad">{renderError}</div>}</section> : <section className="card placeholder"><span className="placeholder-symbol">✓</span><h2>尚无完成的视频</h2><p>生成成功后，成片信息会显示在这里。</p></section>)}

      </main></div>{page !== 6 && <footer className="footer"><button className="button quiet" disabled={page === 0} onClick={() => setPage(page - 1)}>← 上一步</button><span>{String(page + 1).padStart(2, '0')} / 07 · {pages[page]}</span><button className="button primary" onClick={() => setPage(page + 1)}>下一步 →</button></footer>}
  </div>;
}

export default App;
