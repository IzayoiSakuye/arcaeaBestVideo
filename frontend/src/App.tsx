import { useEffect, useRef, useState } from 'react';
import {
  ApplyVideoLink,
  CancelRender,
  CancelVideoDownload,
  CheckTool,
  DownloadVideo,
  Import,
  Jacket,
  PickFile,
  PickOutput,
  PickVideoFile,
  RevealOut,
  Review,
  SearchSongs,
  SetToolPath,
  StartRender,
  Status,
  VideoInfos,
} from '../wailsjs/go/app/App';
import { EventsOn } from '../wailsjs/runtime';
import type { app, score } from '../wailsjs/go/models';
import { eventText, nameOf, scoreText, type Event } from './format';
import { noteSongIndex, pendingSongIndexes } from './issues';
import { videoQueues } from './video';

const pages = [
  '环境检查',
  '导入图片 / JSON',
  '填写感想',
  '识别信息',
  '编排顺序',
  '生成视频',
  '完成与导出',
];
const toolNames = ['ffmpeg', 'ffprobe', 'yt-dlp', 'edge'];
const clearTypes = ['TL', 'NC', 'HC', 'EC', 'FR', 'PM'];
type LogEntry = Event & { time: string };
type VideoBatch = { processed: number; total: number; failed: number; running: boolean };

function errorText(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

function App() {
  const [page, setPage] = useState(0);
  const [status, setStatus] = useState<app.AppStatus | null>(null);
  const [checks, setChecks] = useState<
    Record<string, { result?: string; error?: string; busy?: boolean }>
  >({});
  const [importMode, setImportMode] = useState<'image' | 'json'>('image');
  const [importPath, setImportPath] = useState('');
  const [jsonText, setJsonText] = useState('');
  const [importBusy, setImportBusy] = useState(false);
  const [importError, setImportError] = useState('');
  const [notes, setNotes] = useState<string[]>([]);
  const [acknowledgedNotes, setAcknowledgedNotes] = useState<number[]>([]);
  const [judgmentTexts, setJudgmentTexts] = useState<Record<string, string>>({});
  const [scoreTexts, setScoreTexts] = useState<Record<number, string>>({});
  const [jacket, setJacket] = useState('');
  const [project, setProject] = useState<score.Project | null>(null);
  const [review, setReview] = useState<app.ReviewResult | null>(null);
  const [reviewPending, setReviewPending] = useState(false);
  const [reviewError, setReviewError] = useState('');
  const [videoStates, setVideoStates] = useState<app.VideoState[]>([]);
  const [videoLink, setVideoLink] = useState('');
  const [videoError, setVideoError] = useState('');
  const [videoBusy, setVideoBusy] = useState(false);
  const [videoBatch, setVideoBatch] = useState<VideoBatch | null>(null);
  const [videoStopping, setVideoStopping] = useState(false);
  const [videoBatchError, setVideoBatchError] = useState('');
  const [videoFailures, setVideoFailures] = useState<Record<number, string>>({});
  const [songIndex, setSongIndex] = useState(0);
  const [onlyProblems, setOnlyProblems] = useState(false);
  const [search, setSearch] = useState('');
  const [choices, setChoices] = useState<app.SongChoice[]>([]);
  const [output, setOutput] = useState('');
  const [renderState, setRenderState] = useState<
    'idle' | 'starting' | 'running' | 'failed' | 'done'
  >('idle');
  const [renderError, setRenderError] = useState('');
  const [jobID, setJobID] = useState('');
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [finished, setFinished] = useState('');
  const outputRef = useRef('');
  const reviewSeq = useRef(0);
  const autoVideoPending = useRef(false);
  const stopVideoBatch = useRef(false);
  const videoBusyRef = useRef(false);

  useEffect(() => {
    Status()
      .then(setStatus)
      .catch((error) => setImportError(`环境状态读取失败：${errorText(error)}`));
    return EventsOn('job:event', (event: Event) => {
      setLogs((old) => [
        ...old,
        { ...event, time: new Date().toLocaleTimeString('zh-CN', { hour12: false }) },
      ]);
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
    if (!project) {
      setReview(null);
      setReviewPending(false);
      setVideoStates([]);
      return;
    }
    const seq = ++reviewSeq.current;
    const timer = window.setTimeout(() => {
      Review(project)
        .then((result) => {
          if (seq === reviewSeq.current) {
            setReview(result);
            setReviewPending(false);
            setReviewError('');
          }
        })
        .catch((error) => {
          if (seq === reviewSeq.current) {
            setReview(null);
            setReviewPending(false);
            setReviewError(errorText(error));
          }
        });
    }, 220);
    return () => window.clearTimeout(timer);
  }, [project]);

  useEffect(() => {
    if (!review?.project) return;
    let active = true;
    VideoInfos(review.project)
      .then((result) => {
        if (active) setVideoStates(result);
      })
      .catch((error) => {
        if (active) setVideoError(errorText(error));
      });
    return () => {
      active = false;
    };
  }, [review?.project]);

  useEffect(() => {
    if (!autoVideoPending.current || !review?.project) return;
    autoVideoPending.current = false;
    void downloadAllVideos(review.project);
  }, [review?.project]);

  useEffect(() => {
    setVideoLink(videoStates[songIndex]?.url || '');
    setVideoError('');
  }, [songIndex, videoStates[songIndex]?.url]);

  useEffect(() => {
    if (!search.trim()) {
      setChoices([]);
      return;
    }
    let current = true;
    const timer = window.setTimeout(
      () =>
        SearchSongs(search, 8)
          .then((result) => {
            if (current) setChoices(result);
          })
          .catch((error) => {
            if (current) setReviewError(errorText(error));
          }),
      180,
    );
    return () => {
      current = false;
      window.clearTimeout(timer);
    };
  }, [search]);

  useEffect(() => {
    const current = project?.songs[songIndex];
    if (!current?.song_id) {
      setJacket('');
      return;
    }
    const difficulty =
      current.difficulty.name || review?.project?.songs[songIndex]?.difficulty.name || '';
    let active = true;
    setJacket('');
    Jacket(current.song_id, difficulty)
      .then((value) => {
        if (active) setJacket(value);
      })
      .catch(() => {
        if (active) setJacket('');
      });
    return () => {
      active = false;
    };
  }, [
    project?.songs[songIndex]?.song_id,
    project?.songs[songIndex]?.difficulty.name,
    review?.project?.songs[songIndex]?.difficulty.name,
    songIndex,
  ]);

  async function refreshStatus() {
    setStatus(await Status());
  }
  async function testTool(name: string) {
    setChecks((old) => ({ ...old, [name]: { busy: true } }));
    try {
      const result = await CheckTool(name);
      setChecks((old) => ({ ...old, [name]: { result: result.result } }));
    } catch (error) {
      setChecks((old) => ({ ...old, [name]: { error: errorText(error) } }));
    }
  }
  async function chooseTool(name: string) {
    try {
      const path = await PickFile();
      if (!path) return;
      setChecks((old) => ({ ...old, [name]: { busy: true } }));
      const result = await SetToolPath(name, path);
      setChecks((old) => ({ ...old, [name]: { result: result.result } }));
      await refreshStatus();
    } catch (error) {
      setChecks((old) => ({ ...old, [name]: { error: errorText(error) } }));
    }
  }
  async function autoTool(name: string) {
    try {
      setChecks((old) => ({ ...old, [name]: { busy: true } }));
      const result = await SetToolPath(name, '');
      setChecks((old) => ({ ...old, [name]: { result: result.result } }));
      await refreshStatus();
    } catch (error) {
      setChecks((old) => ({ ...old, [name]: { error: errorText(error) } }));
    }
  }
  async function chooseImport() {
    try {
      const path = await PickFile();
      if (path) {
        setImportPath(path);
        if (importMode === 'json') setJsonText('');
      }
    } catch (error) {
      setImportError(errorText(error));
    }
  }
  async function doImport() {
    if (renderState === 'starting' || renderState === 'running') return;
    if (videoBusy) {
      setImportError('请等待录像获取完成');
      return;
    }
    setImportError('');
    setImportBusy(true);
    try {
      const draft = await Import(importMode, importPath, importMode === 'json' ? jsonText : '');
      if (!draft.Project) throw new Error('导入结果没有成绩数据');
      reviewSeq.current++;
      setProject(draft.Project);
      setNotes(draft.Notes || []);
      setAcknowledgedNotes([]);
      setJudgmentTexts({});
      setScoreTexts({});
      setSongIndex(0);
      setReview(null);
      setReviewPending(true);
      setVideoStates([]);
      setVideoLink('');
      setVideoError('');
      setVideoBatch(null);
      setVideoBatchError('');
      setVideoFailures({});
      autoVideoPending.current = true;
      setFinished('');
      setOutput('');
      setRenderState('idle');
      setRenderError('');
      setLogs([]);
      setJobID('');
      setPage(3);
    } catch (error) {
      setImportError(errorText(error));
    } finally {
      setImportBusy(false);
    }
  }
  function editSongAt(index: number, edit: (song: score.Song) => void) {
    reviewSeq.current++;
    setReviewPending(true);
    setProject((current) => {
      if (!current?.songs[index]) return current;
      const copy = structuredClone(current);
      edit(copy.songs[index]);
      return copy;
    });
  }
  function editSong(edit: (song: score.Song) => void) {
    editSongAt(songIndex, edit);
  }
  function jumpProblem(index: number) {
    if (project && index > 0 && index <= project.songs.length) {
      setSongIndex(index - 1);
      setPage(3);
    }
  }
  function toggleNote(index: number) {
    setAcknowledgedNotes((old) =>
      old.includes(index) ? old.filter((id) => id !== index) : [...old, index],
    );
  }
  function stepSong(direction: number) {
    if (!project) return;
    const indexes = onlyProblems
      ? pendingSongIndexes(
          review?.problems || [],
          notes,
          acknowledgedNotes,
          project.songs.length,
        ).map((i) => i - 1)
      : project.songs.map((_, i) => i);
    const next =
      direction > 0
        ? indexes.find((i) => i > songIndex)
        : [...indexes].reverse().find((i) => i < songIndex);
    if (next !== undefined) setSongIndex(next);
  }
  async function pickOutput() {
    try {
      const path = await PickOutput();
      if (path) {
        setOutput(path);
        setRenderError('');
      }
    } catch (error) {
      setRenderError(errorText(error));
    }
  }
  async function applyVideoLink() {
    if (!song) return;
    const index = songIndex;
    setVideoError('');
    try {
      const link = await ApplyVideoLink(song.song_id, currentDiff, videoLink);
      editSongAt(index, (item) => {
        item.video = link ? { url: link, local_path: '', owner_name: '' } : undefined;
      });
      setVideoFailures((old) => ({ ...old, [index + 1]: '' }));
    } catch (error) {
      setVideoError(errorText(error));
    }
  }
  async function pickVideo() {
    if (!song) return;
    const index = songIndex;
    setVideoError('');
    try {
      const path = await PickVideoFile(song.song_id, currentDiff);
      if (!path) return;
      editSongAt(index, (item) => {
        item.video = { url: '', local_path: path, owner_name: '' };
      });
      setVideoFailures((old) => ({ ...old, [index + 1]: '' }));
    } catch (error) {
      setVideoError(errorText(error));
    }
  }
  async function downloadVideo() {
    if (!song || videoBusyRef.current || videoLink !== (videoStates[songIndex]?.url || '')) return;
    const index = songIndex;
    const sourceSong = review?.project?.songs[index] || song;
    videoBusyRef.current = true;
    setVideoBusy(true);
    setVideoError('');
    try {
      const state = await DownloadVideo(sourceSong);
      editSongAt(index, (item) => {
        if (item.song_id !== sourceSong.song_id || (item.difficulty.name && item.difficulty.name !== sourceSong.difficulty.name)) return;
        item.difficulty.name = sourceSong.difficulty.name;
        item.video = { url: state.url, local_path: '', owner_name: state.owner_name };
      });
      setVideoFailures((old) => ({ ...old, [index + 1]: '' }));
    } catch (error) {
      setVideoError(errorText(error));
    } finally {
      videoBusyRef.current = false;
      setVideoBusy(false);
    }
  }
  async function downloadAllVideos(input: score.Project) {
    if (videoBusyRef.current) return;
    videoBusyRef.current = true;
    stopVideoBatch.current = false;
    setVideoStopping(false);
    setVideoBusy(true);
    setVideoBatchError('');
    try {
      const states = await VideoInfos(input);
      setVideoStates(states);
      const targets = videoQueues(states).retryable;
      setVideoBatch({ processed: 0, total: targets.length, failed: 0, running: targets.length > 0 });
      if (targets.length) await CheckTool('yt-dlp');
      let failed = 0;
      for (let i = 0; i < targets.length; i++) {
        if (stopVideoBatch.current) break;
        const index = targets[i].index - 1;
        const sourceSong = input.songs[index];
        try {
          const state = await DownloadVideo(sourceSong);
          setVideoStates((old) =>
            old.map((item) => item.index === index + 1 ? { ...state, index: index + 1 } : item),
          );
          editSongAt(index, (item) => {
            if (item.song_id !== sourceSong.song_id || (item.difficulty.name && item.difficulty.name !== sourceSong.difficulty.name)) return;
            item.difficulty.name = sourceSong.difficulty.name;
            item.video = { url: state.url, local_path: '', owner_name: state.owner_name };
          });
          setVideoFailures((old) => ({ ...old, [index + 1]: '' }));
        } catch (error) {
          if (stopVideoBatch.current) break;
          failed++;
          setVideoFailures((old) => ({ ...old, [index + 1]: errorText(error) }));
        }
        setVideoBatch({ processed: i + 1, total: targets.length, failed, running: true });
      }
    } catch (error) {
      setVideoBatchError(`自动获取失败：${errorText(error)}`);
    } finally {
      setVideoBatch((old) => old && { ...old, running: false });
      videoBusyRef.current = false;
      setVideoBusy(false);
    }
  }
  async function cancelVideoBatch() {
    if (stopVideoBatch.current) return;
    stopVideoBatch.current = true;
    setVideoStopping(true);
    try {
      await CancelVideoDownload();
    } catch (error) {
      setVideoBatchError(errorText(error));
    }
  }
  async function startRender() {
    if (reviewPending || !review?.project || !output) return;
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
        setRenderState((state) => (state === 'starting' ? 'running' : state));
      } catch (error) {
        const message = errorText(error);
        if (!message.includes('输出文件已存在，请确认覆盖')) throw error;
        if (!window.confirm(`输出文件已存在，确认覆盖？\n${output}`)) {
          setRenderState('idle');
          return;
        }
        replace = true;
        const result = await StartRender(review.project, output, replace);
        setJobID(result.job_id);
        outputRef.current = result.output;
        setRenderState((state) => (state === 'starting' ? 'running' : state));
      }
    } catch (error) {
      setRenderError(errorText(error));
      setRenderState('failed');
    }
  }
  async function cancelRender() {
    try {
      await CancelRender(jobID);
    } catch (error) {
      setRenderError(errorText(error));
    }
  }

  const count = project?.songs.length || 0;
  const row = review?.rows?.[songIndex];
  const song = project?.songs[songIndex];
  const ownProblems = (review?.problems || []).filter(
    (problem) => problem.song_index === songIndex + 1,
  );
  const globalProblems = (review?.problems || []).filter((problem) => problem.song_index === 0);
  const ownNotes = notes
    .map((message, index) => ({ message, index }))
    .filter((note) => noteSongIndex(note.message) === songIndex + 1);
  const globalNotes = notes
    .map((message, index) => ({ message, index }))
    .filter((note) => noteSongIndex(note.message) === 0);
  const pendingSongs = pendingSongIndexes(review?.problems || [], notes, acknowledgedNotes, count);
  const availableDiffs = row && row.song_id === song?.song_id ? row.difficulties || [] : [];
  const currentDiff = song?.difficulty.name || (row?.song_id === song?.song_id ? row?.difficulty : '') || '';
  const currentSongVideo = reviewPending ? song?.video : review?.project?.songs[songIndex]?.video || song?.video;
  const currentVideo = videoStates[songIndex];
  const videoLinkChanged = videoLink !== (currentVideo?.url || '');
  const { pending: missingVideos, retryable: retryableVideos, manualCount } = videoQueues(videoStates);
  const doneCount = new Set(
    logs
      .filter((event) => event.stage === 'render' && ['done', 'skipped'].includes(event.status))
      .map((event) => event.song_index),
  ).size;
  const active = renderState === 'starting' || renderState === 'running';

  return (
    <div className="shell">
      <header className="topbar">
        <div className="brand-mark">◈</div>
        <div>
          <strong>回忆视频生成器</strong>
          <small>ARCAEA MEMORY VIDEO MAKER</small>
        </div>
        <span className="top-version">BEST 50 · VIDEO STUDIO</span>
      </header>
      <div className="workspace">
        <aside className="sidebar">
          <div className="sidebar-title">
            制作流程 <span>WORKFLOW</span>
          </div>
          <nav aria-label="制作流程">
            {pages.map((label, index) => (
              <button
                key={label}
                className={`nav-item ${page === index ? 'selected' : ''} ${[2, 4].includes(index) ? 'future' : ''}`}
                onClick={() => setPage(index)}
                aria-current={page === index ? 'page' : undefined}
              >
                <span className="nav-number">{String(index + 1).padStart(2, '0')}</span>
                <span className="nav-label">
                  {label}
                  {[2, 4].includes(index) && <small>待开发</small>}
                </span>
                <span className="nav-dot">{page === index ? '●' : '○'}</span>
              </button>
            ))}
          </nav>
          <div className="sidebar-foot">当前版本 · Arcaea</div>
        </aside>
        <main className="main-area">
          <div className="page-heading">
            <div>
              <span className="eyebrow">STEP {String(page + 1).padStart(2, '0')} / 07</span>
              <h1>{pages[page]}</h1>
            </div>
            <span className="heading-tag">
              {page === 3 && count ? `第 ${songIndex + 1} / ${count} 首` : 'ARCAEA · BEST 50'}
            </span>
          </div>

          {page === 0 && (
            <div className="content-grid">
              <section className="card wide">
                <h2>
                  工具检测 <span>01 / 04</span>
                </h2>
                <p className="muted">自动检测工具；找不到时点「⋯」选择文件。</p>
                <div className="tool-list">
                  {toolNames.map((name) => {
                    const found = status?.tools?.find((tool) => tool.Name === name);
                    const check = checks[name];
                    return (
                      <div className="tool-row" key={name}>
                        <div className="tool-identity">
                          <span className="tool-icon">{name === 'edge' ? 'E' : '⌘'}</span>
                          <div>
                            <strong>{name === 'edge' ? 'Microsoft Edge' : name}</strong>
                            <small>
                              {name === 'yt-dlp'
                                ? '录像获取使用'
                                : name === 'edge'
                                  ? '文字渲染使用'
                                  : '视频处理使用'}
                            </small>
                          </div>
                        </div>
                        <div className="tool-path" title={found?.Path || ''}>
                          {found?.Path || '尚未找到可执行文件'}
                        </div>
                        <div className="tool-actions">
                          <button
                            className="button quiet pick-tool"
                            onClick={() => chooseTool(name)}
                            disabled={check?.busy}
                            title="手动选择工具文件"
                            aria-label={`手动选择${name}工具文件`}
                          >
                            ⋯
                          </button>
                          <button
                            className="button quiet"
                            onClick={() => testTool(name)}
                            disabled={check?.busy}
                            title="实际运行工具，验证是否可用"
                          >
                            验证可用
                          </button>
                          <button
                            className="button quiet"
                            onClick={() => autoTool(name)}
                            disabled={check?.busy}
                            title="清除手动路径，重新定位并验证工具"
                          >
                            自动检测
                          </button>
                        </div>
                        <div
                          className={`tool-result ${check?.error ? 'bad' : check?.result ? 'good' : ''}`}
                        >
                          {check?.busy
                            ? '检测中…'
                            : check?.error || check?.result || (found?.Path ? '待测试' : '未找到')}
                        </div>
                      </div>
                    );
                  })}
                </div>
                {status?.data_error && (
                  <div className="notice bad">数据包：{status.data_error}</div>
                )}
                {status?.tool_error && (
                  <div className="notice bad">工具配置：{status.tool_error}</div>
                )}
              </section>
              <aside className="card side-card">
                <h2>当前状态</h2>
                <div className="status-figure">
                  <strong>
                    {status?.song_count ?? '—'}{' '}
                    <small>/ {status && !status.data_error ? status.song_count : '—'}</small>
                  </strong>
                  <span>已载入 / 数据包共计（首）</span>
                </div>
                <div className="rule" />
                <p>三种工具可在这里一次配置。Edge 用于文字渲染，也需要检查。</p>
                <p>获取 B站录像时需要 yt-dlp。</p>
              </aside>
            </div>
          )}

          {page === 1 && (
            <div className="content-grid">
              <section className="card wide">
                <h2>导入方式</h2>
                <div className="segmented">
                  <button
                    className={importMode === 'image' ? 'active' : ''}
                    onClick={() => {
                      setImportMode('image');
                      setImportPath('');
                      setImportError('');
                    }}
                  >
                    图片导入
                  </button>
                  <button
                    className={importMode === 'json' ? 'active' : ''}
                    onClick={() => {
                      setImportMode('json');
                      setImportPath('');
                      setImportError('');
                    }}
                  >
                    JSON 导入
                  </button>
                </div>
                <div className="file-box">
                  <span className="file-glyph">↑</span>
                  <strong>
                    {importPath
                      ? nameOf(importPath)
                      : importMode === 'image'
                        ? '选择一张 B50 成绩图'
                        : '选择 JSON 文件'}
                  </strong>
                  {importMode === 'json' && <small>也可以在下方直接粘贴 JSON</small>}
                  <button className="button quiet" onClick={chooseImport}>
                    选择文件
                  </button>
                </div>
                {importMode === 'json' && (
                  <label className="field full">
                    粘贴 JSON
                    <textarea
                      value={jsonText}
                      onChange={(event) => {
                        setJsonText(event.target.value);
                        setImportPath('');
                      }}
                      placeholder="在此粘贴成绩 JSON…"
                      rows={7}
                    />
                  </label>
                )}
                <div className="action-line">
                  <button
                    className="button primary"
                    disabled={
                      importBusy ||
                      active ||
                      (!importPath && !(importMode === 'json' && jsonText.trim()))
                    }
                    onClick={doImport}
                  >
                    {importBusy ? '正在导入…' : '导入并校对 →'}
                  </button>
                </div>
                {importError && (
                  <div className="notice bad" role="alert">
                    {importError}
                  </div>
                )}
              </section>
              <aside className="card side-card">
                <h2>导入结果</h2>
                {project ? (
                  <>
                    <div className="status-figure">
                      <strong>{count}</strong>
                      <span>首成绩已导入</span>
                    </div>
                    <div className="rule" />
                    <p>请在「识别信息」校对成绩。</p>
                  </>
                ) : (
                  <p className="empty-aside">尚未导入成绩</p>
                )}
              </aside>
            </div>
          )}

          {(page === 2 || page === 4) && (
            <section className="card placeholder">
              <span className="placeholder-symbol">◇</span>
              <h2>{pages[page]}</h2>
              <p>待开发</p>
            </section>
          )}

          {page === 3 &&
            (song ? (
              <>
                {videoBatch && (videoBatch.running || retryableVideos.length > 0 || Boolean(videoBatchError)) && (
                  <section className="card download-summary">
                    <h2>
                      自动获取录像 <span>{videoBatch.processed} / {videoBatch.total} 首可自动获取</span>
                    </h2>
                    <div className="progress-track">
                      <div
                        style={{
                          width: `${videoBatch.total ? (videoBatch.processed / videoBatch.total) * 100 : 0}%`,
                        }}
                      />
                    </div>
                    <div className="download-footer">
                      <span>
                        {videoBatch.running
                          ? `正在逐首获取完整录像${manualCount ? `；${manualCount} 首需手动处理` : ''}`
                          : stopVideoBatch.current
                            ? `已停止，已处理 ${videoBatch.processed} 首${manualCount ? `；${manualCount} 首需手动处理` : ''}`
                          : videoBatch.total === 0
                            ? manualCount ? `${manualCount} 首需手动处理` : '录像均已就绪'
                            : `本轮已处理，获取失败 ${videoBatch.failed} 首${manualCount ? `；${manualCount} 首需手动处理` : ''}`}
                      </span>
                      {videoBatch.running ? (
                        <button className="button quiet" onClick={cancelVideoBatch} disabled={videoStopping}>
                          {videoStopping ? '停止中…' : '停止获取'}
                        </button>
                      ) : (
                        <button
                          className="button quiet"
                          disabled={reviewPending || videoBusy || retryableVideos.length === 0}
                          onClick={() => review?.project && void downloadAllVideos(review.project)}
                        >
                          重新获取未就绪
                        </button>
                      )}
                    </div>
                    {videoBatchError && (
                      <div className="notice bad" role="alert">{videoBatchError}</div>
                    )}
                  </section>
                )}
                {pendingSongs.length > 0 && (
                  <section className="card pending-summary">
                    <h2>
                      待校对曲目 <span>{pendingSongs.length} 首</span>
                    </h2>
                    <div className="problem-chips">
                      {pendingSongs.map((index) => (
                        <button
                          key={index}
                          className={index === songIndex + 1 ? 'selected' : ''}
                          onClick={() => jumpProblem(index)}
                        >
                          第 {index} 首
                        </button>
                      ))}
                    </div>
                  </section>
                )}
                {!videoBusy && !autoVideoPending.current && missingVideos.length > 0 && (
                  <section className="card pending-summary">
                    <h2>
                      待补录像 <span>{missingVideos.length} 首</span>
                    </h2>
                    <div className="problem-chips">
                      {missingVideos.map((state) => (
                        <button
                          key={state.index}
                          title={videoFailures[state.index]}
                          onClick={() => setSongIndex(state.index - 1)}
                        >
                          第 {state.index} 首
                        </button>
                      ))}
                    </div>
                  </section>
                )}
                <div className="review-toolbar">
                  <label className="field compact">
                    跳转到第{' '}
                    <input
                      type="number"
                      min={1}
                      max={count}
                      value={songIndex + 1}
                      onChange={(event) => {
                        const value = Number(event.target.value);
                        if (value >= 1 && value <= count) setSongIndex(value - 1);
                      }}
                    />
                    {count} 首
                  </label>
                  <label className="checkline">
                    <input
                      type="checkbox"
                      checked={onlyProblems}
                      onChange={(event) => setOnlyProblems(event.target.checked)}
                    />{' '}
                    只看问题行
                  </label>
                  <div className="spacer" />
                  <button className="button quiet" onClick={() => stepSong(-1)}>
                    ← 上一首
                  </button>
                  <button className="button quiet" onClick={() => stepSong(1)}>
                    下一首 →
                  </button>
                </div>
                <div className="content-grid review-grid">
                  <section className="card wide">
                    <h2>
                      成绩信息 <span>#{String(songIndex + 1).padStart(2, '0')}</span>
                    </h2>
                    <div className="song-title">
                      {jacket ? (
                        <img
                          className="jacket-placeholder"
                          src={jacket}
                          alt={row?.title ? `曲绘：${row.title}` : '曲绘'}
                        />
                      ) : (
                        <div className="jacket-placeholder" aria-label="暂无曲绘">
                          ♫
                        </div>
                      )}
                      <div>
                        <span className="eyebrow">SONG PROFILE</span>
                        <h3>{row?.title || song.song_id || '未匹配曲目'}</h3>
                        <p>{row?.artist || '请核对曲目匹配'}</p>
                      </div>
                    </div>
                    <div className="form-grid">
                      <div className="field full">
                        <span>曲目匹配</span>
                        <div className="inline-field">
                          <input
                            value={search}
                            onChange={(event) => setSearch(event.target.value)}
                            placeholder={song.song_id || '搜索歌名或 song_id'}
                          />
                        </div>
                        {choices.length > 0 && (
                          <div className="search-results">
                            {choices.map((choice) => (
                              <button
                                key={choice.song_id}
                                onClick={() => {
                                  editSong((item) => {
                                    item.song_id = choice.song_id;
                                    item.difficulty.name = choice.difficulties[0] || '';
                                    item.difficulty.constant = 0;
                                    item.difficulty.level = '';
                                    item.video = undefined;
                                  });
                                  setSearch('');
                                }}
                              >
                                {choice.title}
                                <small>
                                  {choice.artist} · {choice.song_id}
                                </small>
                              </button>
                            ))}
                          </div>
                        )}
                      </div>
                      <label className="field">
                        难度
                        <select
                          value={availableDiffs.includes(currentDiff) ? currentDiff : ''}
                          disabled={!availableDiffs.length}
                          onChange={(event) =>
                            editSong((item) => {
                              item.difficulty.name = event.target.value;
                              item.difficulty.constant = 0;
                              item.difficulty.level = '';
                              item.video = undefined;
                            })
                          }
                        >
                          <option value="">{review ? '请选择' : '读取谱面中…'}</option>
                          {availableDiffs.map((name) => (
                            <option key={name} value={name}>
                              {name}
                            </option>
                          ))}
                        </select>
                      </label>
                      <label className="field">
                        分数
                        <input
                          type="number"
                          min={0}
                          max={10000000}
                          value={scoreTexts[songIndex] ?? scoreText(song.score.achievement)}
                          onChange={(event) => {
                            setScoreTexts((old) => ({ ...old, [songIndex]: event.target.value }));
                            editSong((item) => {
                              item.score.achievement = Number(event.target.value) / 100000;
                            });
                          }}
                        />
                      </label>
                      <label className="field">
                        通关类型
                        <select
                          value={song.score.clear_type || ''}
                          onChange={(event) =>
                            editSong((item) => {
                              item.score.clear_type = event.target.value;
                            })
                          }
                        >
                          <option value="">未填写</option>
                          {[...new Set([song.score.clear_type, ...clearTypes].filter(Boolean))].map(
                            (name) => (
                              <option key={name} value={name}>
                                {name}
                              </option>
                            ),
                          )}
                        </select>
                      </label>
                      <div className="field">
                        单曲 PTT
                        <div className="readout">
                          {row?.ptt == null ? '待计算' : row.ptt.toFixed(3)}
                        </div>
                      </div>
                      <div className="field">
                        曲库定数
                        <div className="readout">
                          {row?.constant == null ? '未匹配' : row.constant}
                        </div>
                      </div>
                      <div className="field">
                        评级<div className="readout">{row?.rank || '待计算'}</div>
                      </div>
                      <div className="judgment-row full">
                        {(['pure', 'shiny_perfect', 'far', 'lost'] as const).map((key) =>
                          key === 'shiny_perfect' ? (
                            <label className="field" key={key}>
                              PURE(大P数)
                              <input
                                type="number"
                                min={0}
                                max={song.score.pure || undefined}
                                value={
                                  judgmentTexts[songIndex + '.' + key] ?? song.score[key] ?? ''
                                }
                                placeholder={project?.source.type === 'image' ? '未读取' : ''}
                                onChange={(event) => {
                                  setJudgmentTexts((old) => ({
                                    ...old,
                                    [songIndex + '.' + key]: event.target.value,
                                  }));
                                  editSong((item) => {
                                    item.score.shiny_perfect =
                                      event.target.value === ''
                                        ? undefined
                                        : Number(event.target.value);
                                  });
                                }}
                              />
                            </label>
                          ) : (
                            <label className="field" key={key}>
                              {key === 'pure' ? 'Pure' : key.toUpperCase()}
                              <input
                                type="number"
                                min={0}
                                value={
                                  judgmentTexts[songIndex + '.' + key] ??
                                  (song.score.unread_judgments?.includes(key)
                                    ? ''
                                    : song.score[key])
                                }
                                placeholder={project?.source.type === 'image' ? '未读取' : ''}
                                onChange={(event) => {
                                  setJudgmentTexts((old) => ({
                                    ...old,
                                    [songIndex + '.' + key]: event.target.value,
                                  }));
                                  editSong((item) => {
                                    item.score[key] = Number(event.target.value);
                                    const unread = item.score.unread_judgments || [];
                                    item.score.unread_judgments =
                                      event.target.value === ''
                                        ? [...new Set([...unread, key])]
                                        : unread.filter((field) => field !== key);
                                  });
                                }}
                              />
                            </label>
                          ),
                        )}
                      </div>
                      <label className="field full">
                        单曲评语
                        <textarea
                          rows={4}
                          maxLength={500}
                          value={song.user.comment}
                          onChange={(event) =>
                            editSong((item) => {
                              item.user.comment = event.target.value;
                            })
                          }
                          placeholder="写下关于这首歌的回忆…"
                        />
                      </label>
                    </div>
                  </section>
                  <aside className="right-stack">
                    <section className="card video-card">
                      <h2>
                        录像预览{' '}
                        <span>
                          {videoLinkChanged
                            ? '未就绪'
                            : currentVideo?.status === 'ready'
                              ? '已就绪'
                              : currentVideo?.status === 'legacy'
                                ? '旧缓存，请核对'
                                : '未就绪'}
                        </span>
                      </h2>
                      {currentVideo?.preview_url && !videoLinkChanged ? (
                        <video
                          key={currentVideo.preview_url}
                          controls
                          preload="metadata"
                          src={currentVideo.preview_url}
                        />
                      ) : (
                        <div className="video-empty">
                          {currentVideo?.status === 'download'
                            ? '可获取录像'
                            : currentVideo?.status === 'invalid'
                              ? '链接无效，请修改'
                              : '暂无可预览录像'}
                        </div>
                      )}
                    </section>
                    <section className="card video-card">
                      <h2>视频来源</h2>
                      <label className="field">
                        视频链接（B站 / YouTube）
                        <input
                          value={videoLink}
                          onChange={(event) => setVideoLink(event.target.value)}
                          disabled={videoBusy}
                          placeholder="粘贴 B站或 YouTube 视频链接"
                        />
                      </label>
                      {currentVideo?.status === 'missing' && !currentSongVideo?.local_path && currentDiff && (
                        <p className="video-path">曲库暂无该谱面录像，请粘贴链接或选择本地文件。</p>
                      )}
                      {currentSongVideo?.local_path && (
                        <p className="video-path">本地文件：{currentSongVideo.local_path}</p>
                      )}
                      {currentVideo?.owner_name && !videoLinkChanged && (
                        <p className="video-owner">发布者：{currentVideo.owner_name}</p>
                      )}
                      <div className="video-actions">
                        <button
                          className="button quiet"
                          onClick={applyVideoLink}
                          disabled={videoBusy || (!videoLinkChanged && !currentSongVideo?.local_path)}
                        >
                          应用链接
                        </button>
                        <button className="button quiet" onClick={pickVideo} disabled={videoBusy}>
                          选择本地文件
                        </button>
                        <button
                          className="button primary"
                          onClick={downloadVideo}
                          disabled={
                            reviewPending ||
                            videoBusy ||
                            videoLinkChanged ||
                            !currentVideo?.url ||
                            currentVideo.status === 'ready'
                          }
                        >
                          {videoBusy ? '获取中…' : '获取录像'}
                        </button>
                      </div>
                      {(videoError || videoFailures[songIndex + 1]) && (
                        <div className="notice bad" role="alert">
                          {videoError || videoFailures[songIndex + 1]}
                        </div>
                      )}
                    </section>
                    <section className="card">
                      <h2>
                        校对结果{' '}
                        <span>
                          {ownProblems.length +
                            ownNotes.filter((note) => !acknowledgedNotes.includes(note.index))
                              .length}{' '}
                          项待处理
                        </span>
                      </h2>
                      {reviewError && <div className="notice bad">{reviewError}</div>}
                      <div className="problem-list">
                        {ownProblems.map((problem, index) => (
                          <div
                            className={
                              'problem ' +
                              (problem.rule === 'difficulty-not-selected' ? 'warn' : problem.severity)
                            }
                            key={problem.rule + '-' + index}
                          >
                            <b>
                              {problem.rule === 'difficulty-not-selected'
                                ? '警告'
                                : problem.severity.toUpperCase()}
                            </b>
                            <span>{problem.message}</span>
                          </div>
                        ))}
                        {ownNotes.map((note) => (
                          <div
                            className={
                              'problem ' +
                              (acknowledgedNotes.includes(note.index) ? 'done' : 'warn')
                            }
                            key={'ocr-' + note.index}
                          >
                            <b>OCR</b>
                            <span>{note.message.replace(/^#\d+\s*/, '')}</span>
                            <button className="button text" onClick={() => toggleNote(note.index)}>
                              {acknowledgedNotes.includes(note.index) ? '撤销核对' : '标记已核对'}
                            </button>
                          </div>
                        ))}
                        {!ownProblems.length &&
                          !ownNotes.some((note) => !acknowledgedNotes.includes(note.index)) && (
                            <p className="muted">本首暂无待校对问题</p>
                          )}
                      </div>
                    </section>
                    {(globalProblems.length > 0 || globalNotes.length > 0) && (
                      <section className="card">
                        <h2>全局问题</h2>
                        <div className="problem-list">
                          {globalProblems.map((problem, index) => (
                            <div
                              className={'problem ' + problem.severity}
                              key={problem.rule + '-' + index}
                            >
                              <b>{problem.severity.toUpperCase()}</b>
                              <span>{problem.message}</span>
                            </div>
                          ))}
                          {globalNotes.map((note) => (
                            <div
                              className={
                                'problem ' +
                                (acknowledgedNotes.includes(note.index) ? 'done' : 'warn')
                              }
                              key={'ocr-' + note.index}
                            >
                              <b>OCR</b>
                              <span>{note.message}</span>
                              <button
                                className="button text"
                                onClick={() => toggleNote(note.index)}
                              >
                                {acknowledgedNotes.includes(note.index) ? '撤销核对' : '标记已核对'}
                              </button>
                            </div>
                          ))}
                        </div>
                      </section>
                    )}
                  </aside>
                </div>
              </>
            ) : (
              <section className="card placeholder">
                <span className="placeholder-symbol">⌑</span>
                <h2>尚无成绩</h2>
                <p>请先在「导入图片 / JSON」导入一份成绩。</p>
                <button className="button primary" onClick={() => setPage(1)}>
                  前往导入
                </button>
              </section>
            ))}

          {page === 5 && (
            <div className="content-grid">
              <section className="card wide">
                <h2>生成任务</h2>
                <div className="output-picker">
                  <div>
                    <span>输出文件</span>
                    <strong>{output ? nameOf(output) : '尚未选择 MP4 路径'}</strong>
                    <small title={output}>{output || '开始生成前请选择保存位置'}</small>
                  </div>
                  <button className="button quiet" onClick={pickOutput} disabled={active}>
                    选择路径
                  </button>
                </div>
                <div className="progress-head">
                  <strong>
                    {renderState === 'done'
                      ? '已完成'
                      : active
                        ? '正在生成'
                        : renderState === 'failed'
                          ? '生成失败'
                          : '等待开始'}
                  </strong>
                  <span>
                    {doneCount} / {count} 段完成
                  </span>
                </div>
                <div className="progress-track">
                  <div
                    style={{ width: count ? `${Math.min(100, (doneCount / count) * 100)}%` : '0%' }}
                  />
                </div>
                <div className="action-line">
                  <button
                    className="button primary"
                    disabled={reviewPending || !review?.renderable || !output || active || videoBusy}
                    onClick={startRender}
                  >
                    开始生成
                  </button>
                  {active && (
                    <button className="button quiet" disabled={!jobID} onClick={cancelRender}>
                      取消生成
                    </button>
                  )}
                </div>
                {!project && <div className="notice warn">请先导入并校对成绩。</div>}
                {review && !review.renderable && (
                  <div className="notice warn">成绩存在 fatal 问题，请先在「识别信息」校对。</div>
                )}
                {renderError && (
                  <div className="notice bad" role="alert">
                    {renderError}
                  </div>
                )}
              </section>
              <aside className="card log-card">
                <h2>
                  运行记录 <span>{logs.length}</span>
                </h2>
                <div className="log-list" role="log" aria-live="polite">
                  {logs.length ? (
                    logs.map((event, index) => (
                      <div className={`log-entry ${event.status}`} key={index}>
                        <time>{event.time}</time>
                        <span>{eventText(event)}</span>
                      </div>
                    ))
                  ) : (
                    <div className="log-empty">生成开始后，这里会显示实际任务事件。</div>
                  )}
                </div>
              </aside>
            </div>
          )}

          {page === 6 &&
            (finished ? (
              <section className="card completion">
                <div className="completion-icon">✓</div>
                <span className="eyebrow">RENDER COMPLETE</span>
                <h2>视频已生成</h2>
                <p>成片已保存到所选位置。</p>
                <div className="completed-file">
                  <span>MP4 文件</span>
                  <strong>{nameOf(finished)}</strong>
                  <small>{finished}</small>
                </div>
                <div className="completion-actions">
                  <button
                    className="button primary"
                    onClick={() =>
                      RevealOut(finished).catch((error) => setRenderError(errorText(error)))
                    }
                  >
                    打开文件路径
                  </button>
                  <button className="button quiet" onClick={() => setPage(0)}>
                    返回首页
                  </button>
                </div>
                {renderError && <div className="notice bad">{renderError}</div>}
              </section>
            ) : (
              <section className="card placeholder">
                <span className="placeholder-symbol">✓</span>
                <h2>尚无完成的视频</h2>
                <p>生成成功后，成片信息会显示在这里。</p>
              </section>
            ))}
        </main>
      </div>
      {page !== 6 && (
        <footer className="footer">
          <button className="button quiet" disabled={page === 0} onClick={() => setPage(page - 1)}>
            ← 上一步
          </button>
          <span>
            {String(page + 1).padStart(2, '0')} / 07 · {pages[page]}
          </span>
          <button className="button primary" onClick={() => setPage(page + 1)}>
            下一步 →
          </button>
        </footer>
      )}
    </div>
  );
}

export default App;
