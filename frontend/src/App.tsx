import { useEffect, useRef, useState } from 'react';
import {
  ApplyVideoLink,
  CancelRender,
  CheckTool,
  ClearCache,
  CheckResourceUpdate,
  CheckOPAudioUpdate,
  Import,
  Jacket,
  PickFile,
  PickOutput,
  PickVideoFile,
  ThoughtAudioOptions,
  RevealOut,
  Review,
  SearchSongs,
  SetToolPath,
  StartRender,
  Status,
  UpdateResources,
  UpdateOPAudio,
  CacheInfo,
  Version,
} from '../wailsjs/go/app/App';
import { EventsOn } from '../wailsjs/runtime';
import { score, type app } from '../wailsjs/go/models';
import { eventText, nameOf, scoreText, type Event } from './format';
import { noteSongIndex, pendingSongIndexes } from './issues';
import { clipSaved, usedSongIndexes, videoSourceKey, videoStatusText } from './video';
import { useVideoPreparation } from './useVideoPreparation';
import VideoPreview from './PreviewMedia';
import { clipRangeError, formatClipTime, parseClipTime } from './clip';
import ThoughtEditor from './ThoughtEditor';
import Arrangement from './Arrangement';
import { emptyThought, sameThought, thoughtFromProject, type ThoughtDraft } from './thought';

const pages = [
  '导入图片 / JSON',
  '填写感想',
  '识别信息',
  '编排顺序',
  '生成视频',
  '完成与导出',
  '设置',
];
const toolNames = ['ffmpeg', 'ffprobe', 'yt-dlp', 'edge'];
const clearTypes = ['TL', 'NC', 'HC', 'EC', 'FR', 'PM'];
type LogEntry = Event & { time: string };
type ResourceProgress = { downloaded: number; total: number };
type OPAudioStatus = { audio_version: string; files: { kind: string; size: number; installed: boolean; update_available: boolean }[] };
type OPAudioProgress = ResourceProgress & { kind: string };

function errorText(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

function App() {
  const [page, setPage] = useState(0);
  const [status, setStatus] = useState<app.AppStatus | null>(null);
  const [version, setVersion] = useState('');
  const [cacheMessage, setCacheMessage] = useState('');
  const [cacheBusy, setCacheBusy] = useState(false);
  const [resourceInfo, setResourceInfo] = useState<app.ResourceUpdateStatus | null>(null);
  const [resourceMessage, setResourceMessage] = useState('正在检查资源更新…');
  const [resourceBusy, setResourceBusy] = useState(false);
  const [resourceProgress, setResourceProgress] = useState<ResourceProgress | null>(null);
  const [opInfo, setOpInfo] = useState<OPAudioStatus | null>(null);
  const [opMessage, setOpMessage] = useState('');
  const [opBusy, setOpBusy] = useState('');
  const [opProgress, setOpProgress] = useState<OPAudioProgress | null>(null);
  const [checkingResources, setCheckingResources] = useState(false);
  const [updateFinished, setUpdateFinished] = useState(false);
  const resourceDialog = useRef<HTMLDialogElement>(null);
  const promptedResourceVersion = useRef('');
  const [configSaved, setConfigSaved] = useState(false);
  const clearCacheDialog = useRef<HTMLDialogElement>(null);
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
  const [jacket, setJacket] = useState({ key: '', url: '' });
  const jacketCache = useRef(new Map<string, string>());
  const [project, setProject] = useState<score.Project | null>(null);
  const [thoughtDraft, setThoughtDraft] = useState<ThoughtDraft>(emptyThought);
  const [thoughtID, setThoughtID] = useState<string | null>(null);
  const [thoughtEditorVersion, setThoughtEditorVersion] = useState(0);
  const [bgmOptions, setBgmOptions] = useState<app.ThoughtAudioOption[]>([]);
  const [bgmLoading, setBgmLoading] = useState(true);
  const [bgmError, setBgmError] = useState('');
  const [review, setReview] = useState<app.ReviewResult | null>(null);
  const [reviewPending, setReviewPending] = useState(false);
  const [reviewError, setReviewError] = useState('');
  const [projectEpoch, setProjectEpoch] = useState(0);
  const lifecycle = useRef(0);
  const sourceChangeSeq = useRef(0);
  const [videoLinkDrafts, setVideoLinkDrafts] = useState<Record<string, string>>({});
  const [videoError, setVideoError] = useState('');
  const [clipSaving, setClipSaving] = useState(false);
  const [switchTarget, setSwitchTarget] = useState<number | null>(null);
  const switchDialog = useRef<HTMLDialogElement>(null);
  const [cacheInfo, setCacheInfo] = useState<app.CacheState | null>(null);
  const [songIndex, setSongIndex] = useState(0);
  const [songJumpDraft, setSongJumpDraft] = useState<string | null>(null);
  const [clipDrafts, setClipDrafts] = useState<Record<string, { start: string; end: string }>>({});
  const draftSong = project?.songs[songIndex];
  const draftKey = `${projectEpoch}:${songIndex}:${videoSourceKey(draftSong ? [draftSong] : [])}`;
  const clipStartText = clipDrafts[draftKey]?.start ?? formatClipTime((draftSong?.clip?.start_ms || 0) / 1000);
  const clipEndText = clipDrafts[draftKey]?.end ?? formatClipTime(((draftSong?.clip?.start_ms || 0) + (draftSong?.clip?.duration_ms || 10000)) / 1000);
  const setClipStartText = (start: string) => setClipDrafts((old) => ({ ...old, [draftKey]: { start, end: clipEndText } }));
  const setClipEndText = (end: string) => setClipDrafts((old) => ({ ...old, [draftKey]: { start: clipStartText, end } }));
  const latestEdit = useRef({ project, draftKey, clipStartText, clipEndText });
  latestEdit.current = { project, draftKey, clipStartText, clipEndText };
  const [previewTime, setPreviewTime] = useState(0);
  const previewScrub = useRef<number | null>(null);
  const [previewPlaying, setPreviewPlaying] = useState(false);
  const previewRef = useRef<HTMLVideoElement>(null);
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
  const [renderTotal, setRenderTotal] = useState(0);
  const [finished, setFinished] = useState('');
  const outputRef = useRef('');
  const reviewSeq = useRef(0);
  const videoKey = videoSourceKey(project?.songs || []);
  const videos = useVideoPreparation(review?.project || null, videoKey, projectEpoch);
  const videoStates = videos.states;
  const currentVideoURL = videoStates.find((state) => state.index === songIndex + 1)?.url || '';
  const videoLink = videoLinkDrafts[draftKey] ?? currentVideoURL;
  const setVideoLink = (value: string) => setVideoLinkDrafts((old) => ({ ...old, [draftKey]: value }));
  const videoBusy = videos.busy;
  const attempted = useRef(new Set<string>());

  useEffect(() => {
    Status()
      .then((value) => {
        setStatus(value);
        if (localStorage.getItem('environment-ready') !== '1') setPage(6);
      })
      .catch((error) => setImportError(`环境状态读取失败：${errorText(error)}`));
    Version().then(setVersion).catch(() => setVersion('未知'));
    void checkAllResources();
    ThoughtAudioOptions()
      .then(setBgmOptions)
      .catch((error) => setBgmError(errorText(error)))
      .finally(() => setBgmLoading(false));
    const offJob = EventsOn('job:event', (event: Event) => {
      setLogs((old) => [
        ...old,
        { ...event, time: new Date().toLocaleTimeString('zh-CN', { hour12: false }) },
      ]);
      if (event.stage === 'done' && event.status === 'done') {
        setFinished(outputRef.current);
        setRenderState('done');
        setPage(5);
      } else if (event.status === 'cancelled') {
        setRenderState('idle');
        setPage(4);
      } else if (event.status === 'failed') {
        setRenderError(event.error || event.message || '生成失败');
        setRenderState('failed');
        setPage(4);
      }
    });
    const offResource = EventsOn('resource:progress', (value: ResourceProgress) => setResourceProgress(value));
    const offOP = EventsOn('op:progress', (value: OPAudioProgress) => setOpProgress(value));
    return () => { offJob(); offResource(); offOP(); };
  }, []);

  useEffect(() => {
    const audioAvailable = opInfo?.files.some((file) => file.update_available);
    const key = `${resourceInfo?.latest_version || ''}/${opInfo?.audio_version || ''}`;
    if (page !== 0 || !status || resourceBusy || checkingResources || updateFinished ||
      (!resourceInfo?.update_available && !audioAvailable) || promptedResourceVersion.current === key) return;
    promptedResourceVersion.current = key;
    resourceDialog.current?.showModal();
  }, [page, status, resourceInfo, opInfo, resourceBusy, checkingResources, updateFinished]);

  useEffect(() => {
    if (!project) {
      setReview(null);
      setReviewPending(false);
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
  }, [project?.songs]);

  useEffect(() => {
    setVideoError('');
  }, [songIndex, projectEpoch, videoStates.find((state) => state.index === songIndex + 1)?.url]);

  useEffect(() => {
    if (reviewPending || !review?.project) return;
    for (const state of videoStates) {
      const selected = review.project.songs[state.index - 1];
      if (!selected) continue;
      const youtube = state.url.startsWith('https://www.youtube.com/');
      const editing = page === 2 && state.index === songIndex + 1;
      if ((!youtube && !editing) || state.request_id || ['failed', 'cancelled', 'invalid', 'missing'].includes(state.status)) continue;
      const key = `${projectEpoch}:${videoKey}:${state.index}:${state.url}`;
      if (attempted.current.has(key)) continue;
      attempted.current.add(key);
      void videos.prepare(selected).catch(() => {});
    }
  }, [page, songIndex, reviewPending, review?.project, videoStates.map((state) => `${state.index}:${state.url}:${state.request_id}`).join('|')]);

  useEffect(() => { setPreviewTime(0); setPreviewPlaying(false); previewScrub.current = null; }, [songIndex, page, projectEpoch]);
  useEffect(() => { setSongJumpDraft(null); }, [songIndex, projectEpoch]);

  useEffect(() => {
    if (page !== 6) return;
    let current = true;
    const refresh = () => CacheInfo().then((value) => { if (current) setCacheInfo(value); }).catch((error) => { if (current) setCacheMessage(errorText(error)); });
    void refresh();
    const timer = window.setInterval(() => { void refresh(); }, 2000);
    return () => { current = false; window.clearInterval(timer); };
  }, [page, renderState]);

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
      setJacket({ key: '', url: '' });
      return;
    }
    const difficulty =
      current.difficulty.name || review?.project?.songs[songIndex]?.difficulty.name || '';
    const key = `${current.song_id}\u0000${difficulty}`;
    const cached = jacketCache.current.get(key);
    if (cached) {
      setJacket({ key, url: cached });
      return;
    }
    let active = true;
    setJacket({ key, url: '' });
    Jacket(current.song_id, difficulty)
      .then((value) => {
        if (active) {
          jacketCache.current.set(key, value);
          setJacket({ key, url: value });
        }
      })
      .catch(() => {
        if (active) setJacket({ key, url: '' });
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
  async function checkAllResources() {
    setCheckingResources(true);
    const [data, audio] = await Promise.allSettled([CheckResourceUpdate(), CheckOPAudioUpdate()]);
    if (data.status === 'fulfilled') {
      setResourceInfo((old) => old?.restart_required ? old : data.value);
    }
    if (audio.status === 'fulfilled') {
      setOpInfo(audio.value);
    }
    const available = (data.status === 'fulfilled' && data.value.update_available) ||
      (audio.status === 'fulfilled' && audio.value.files.some((file) => file.update_available));
    const errors = [data.status === 'rejected' ? errorText(data.reason) : '',
      audio.status === 'rejected' ? errorText(audio.reason) : ''].filter(Boolean);
    setResourceMessage(errors.length ? `检查更新失败：${errors.join('；')}`
      : available ? '发现可用资源更新' : '资源已是最新版本');
    setCheckingResources(false);
  }
  async function updateResources() {
    setResourceBusy(true);
    setUpdateFinished(false);
    setResourceProgress(null);
    setOpProgress(null);
    setResourceMessage('正在检查更新…');
    const failures: string[] = [];
    let restartRequired = false;
    try {
      const result = await UpdateResources();
      setResourceInfo(result);
      restartRequired = result.restart_required;
    } catch (error) {
      failures.push(`曲库：${errorText(error)}`);
    }
    setResourceProgress(null);
    setOpMessage('正在检查 OP 更新…');
    try {
      const audio = await CheckOPAudioUpdate();
      setOpInfo(audio);
      const pending = audio.files.filter((file) => file.update_available);
      for (const [index, file] of pending.entries()) {
        setOpBusy(file.kind);
        setOpProgress(null);
        setOpMessage(`正在更新 ${file.kind.toUpperCase()}（${index + 1}/${pending.length}）`);
        try {
          await UpdateOPAudio(file.kind);
          setOpInfo((old) => old && { ...old, files: old.files.map((item) => item.kind === file.kind ? { ...item, installed: true, update_available: false } : item) });
        } catch (error) {
          failures.push(`${file.kind.toUpperCase()}：${errorText(error)}`);
        }
      }
      setBgmOptions(await ThoughtAudioOptions());
      setBgmError('');
      setOpMessage(pending.length ? `OP 更新完成：${pending.length - failures.filter((item) => item.startsWith('OP')).length}/${pending.length}` : 'OP 音频已是最新版本');
    } catch (error) {
      failures.push(`OP：${errorText(error)}`);
      setOpMessage(`OP 更新失败：${errorText(error)}`);
    }
    setOpBusy('');
    setResourceBusy(false);
    setUpdateFinished(true);
    setResourceMessage(failures.length ? `资源更新失败：${failures.join('；')}；已安装资源可继续使用`
      : restartRequired ? '资源更新完成，重启应用后生效' : '资源更新完成');
  }
  async function clearCache(includeVideos: boolean) {
    try {
      setCacheBusy(true);
      setCacheMessage('清理中…');
      await ClearCache(includeVideos);
      setCacheInfo(await CacheInfo());
      setProjectEpoch(++lifecycle.current);
      setCacheMessage(includeVideos ? '渲染缓存和录像缓存已清理' : '渲染缓存已清理，录像已保留');
    } catch (error) {
      setCacheMessage(errorText(error));
    } finally {
      setCacheBusy(false);
    }
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
      const path = await PickFile(false);
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
      const path = await PickFile(importMode === 'image');
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
    setImportError('');
    setImportBusy(true);
    try {
      const draft = await Import(importMode, importPath, importMode === 'json' ? jsonText : '');
      if (!draft.Project) throw new Error('导入结果没有成绩数据');
      const importedThoughts = draft.Project.thoughts?.length ? draft.Project.thoughts : draft.Project.thought ? [draft.Project.thought] : [];
      const thoughts = importedThoughts.map((thought) => new score.Thought({ ...thought, id: thought.id || crypto.randomUUID() }));
      const importedProject = { ...draft.Project, thoughts, thought: undefined } as score.Project;
      reviewSeq.current++;
      lifecycle.current++;
      setProjectEpoch(lifecycle.current);
      setClipDrafts({});
      setClipSaving(false);
      setSwitchTarget(null);
      switchDialog.current?.close();
      setProject(importedProject);
      setThoughtID(thoughts[0]?.id || null);
      setThoughtDraft(thoughtFromProject(thoughts[0]) || emptyThought());
      setThoughtEditorVersion((version) => version + 1);
      setNotes(draft.Notes || []);
      setAcknowledgedNotes([]);
      setJudgmentTexts({});
      setScoreTexts({});
      setSongIndex(0);
      setReview(null);
      setReviewPending(true);
      setVideoLinkDrafts({});
      setVideoError('');
      setFinished('');
      setOutput('');
      setRenderState('idle');
      setRenderError('');
      setLogs([]);
      setRenderTotal(0);
      setJobID('');
      setPage(1);
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
      const before = videoSourceKey([copy.songs[index]]);
      const previousClip = JSON.stringify(copy.songs[index].clip);
      edit(copy.songs[index]);
      if (before !== videoSourceKey([copy.songs[index]]) && JSON.stringify(copy.songs[index].clip) === previousClip) copy.songs[index].clip = undefined;
      return copy;
    });
  }
  function editSong(edit: (song: score.Song) => void) {
    editSongAt(songIndex, edit);
  }
  function addBlankSong() {
    if (active) return;
    const blank = { index: (project?.songs.length || 0) + 1, song_id: '',
      difficulty: { name: '', level: '', constant: 0 },
      score: { achievement: 0, rank: '', clear_type: '', pure: 0, far: 0, lost: 0 },
      user: { comment: '' } } as score.Song;
    setProject((current) => {
      if (!current) return new score.Project({
        schema_version: 1, game: 'arcaea', region: '',
        player: { name: '', rating: 0, rating_label: '' },
        snapshot_at: new Date().toISOString(), source: { type: 'manual', provider: 'user', confidence: 1 },
        defaults: { clips: { start_ms: 0, duration_ms: 10000 } },
        songs: [blank], thoughts: [], play_order: [{ kind: 'song', song_index: 1 }],
      });
      return { ...current, songs: [...current.songs, blank],
        play_order: current.play_order ? [...current.play_order, { kind: 'song', song_index: blank.index }] : undefined,
      } as score.Project;
    });
    setSongIndex(blank.index - 1);
    setSearch('');
    setPage(2);
  }
  function saveThought(thought: ThoughtDraft) {
    const id = thought.id || crypto.randomUUID();
    const saved = new score.Thought({ ...thought, id });
    setProject((current) => {
      if (!current) return current;
      const thoughts = current.thoughts || [];
      const index = thoughts.findIndex((item) => item.id === id);
      const next = [...thoughts];
      if (index < 0) next.push(saved);
      else next[index] = saved;
      return { ...current, thoughts: next, thought: undefined } as score.Project;
    });
    setThoughtID(id);
    setThoughtDraft(thoughtFromProject(saved) || emptyThought());
  }
  function leaveThoughtDraft() {
    const saved = project?.thoughts?.find((item) => item.id === thoughtID);
    const current = thoughtFromProject(saved);
    const blank = emptyThought();
    if (sameThought(thoughtDraft, current || blank)) return true;
    return window.confirm('当前感想片段有未保存修改，确定放弃这些修改？');
  }
  function selectThought(id: string) {
    if (id === thoughtID || !leaveThoughtDraft()) return;
    const thought = project?.thoughts?.find((item) => item.id === id);
    if (!thought) return;
    setThoughtID(id);
    setThoughtDraft(thoughtFromProject(thought) || emptyThought());
    setThoughtEditorVersion((version) => version + 1);
  }
  function newThought() {
    if (!leaveThoughtDraft()) return;
    setThoughtID(null);
    setThoughtDraft(emptyThought());
    setThoughtEditorVersion((version) => version + 1);
  }
  function deleteThought(id: string) {
    const thought = project?.thoughts?.find((item) => item.id === id);
    if (!thought || !window.confirm(`确定删除感想片段「${thought.name}」？`)) return;
    const remaining = project?.thoughts?.filter((item) => item.id !== id) || [];
    setProject((current) => current ? { ...current, thoughts: remaining, play_order: current.play_order?.filter((item) => item.kind !== 'thought' || item.thought_id !== id) } as score.Project : current);
    if (thoughtID === id) {
      setThoughtID(null);
      setThoughtDraft(emptyThought());
      setThoughtEditorVersion((version) => version + 1);
    }
  }
  function jumpProblem(index: number) {
    if (project && index > 0 && index <= project.songs.length) {
      requestSong(index - 1);
      setPage(2);
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
    if (next !== undefined) requestSong(next);
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
    const epoch = lifecycle.current;
    const operation = ++sourceChangeSeq.current;
    const source = videoSourceKey([song]);
    setVideoError('');
    try {
      const link = await ApplyVideoLink(song.song_id, currentDiff, videoLink);
      if (epoch !== lifecycle.current || operation !== sourceChangeSeq.current) return;
      editSongAt(index, (item) => {
        if (videoSourceKey([item]) !== source) return;
        item.video = link ? { url: link, local_path: '', owner_name: '' } : undefined;
      });
    } catch (error) {
      if (epoch === lifecycle.current && operation === sourceChangeSeq.current && videoSourceKey(latestEdit.current.project?.songs[index] ? [latestEdit.current.project.songs[index]] : []) === source) setVideoError(errorText(error));
    }
  }
  async function pickVideo() {
    if (!song) return;
    const index = songIndex;
    const epoch = lifecycle.current;
    const operation = ++sourceChangeSeq.current;
    const source = videoSourceKey([song]);
    setVideoError('');
    try {
      const path = await PickVideoFile(song.song_id, currentDiff);
      if (!path) return;
      if (epoch !== lifecycle.current || operation !== sourceChangeSeq.current) return;
      editSongAt(index, (item) => {
        if (videoSourceKey([item]) !== source) return;
        item.video = { url: '', local_path: path, owner_name: '' };
      });
    } catch (error) {
      if (epoch === lifecycle.current && operation === sourceChangeSeq.current && videoSourceKey(latestEdit.current.project?.songs[index] ? [latestEdit.current.project.songs[index]] : []) === source) setVideoError(errorText(error));
    }
  }
  async function downloadVideo() {
    const selected = review?.project?.songs[songIndex];
    if (!selected || reviewPending || videoLinkChanged) return;
    try { await videos.prepare(selected); } catch (error) { setVideoError(errorText(error)); }
  }
  async function changeVideoTask(state: app.VideoState, retry: boolean) {
    const selected = review?.project?.songs[state.index - 1];
    if (!selected) return;
    try {
      if (retry) await videos.retry(selected, state);
      else await videos.cancel(state);
    } catch (error) { setVideoError(errorText(error)); }
  }
  async function startRender() {
    if (reviewPending || !review?.project || !output || materialBlockers.length) return;
    setRenderError('');
    setLogs([]);
    setRenderTotal(project?.play_order?.length ?? review.project.songs.length);
    setRenderState('starting');
    setFinished('');
    outputRef.current = output;
    try {
      let replace = false;
      try {
        const result = await StartRender({ ...review.project, thoughts: project?.thoughts, play_order: project?.play_order, thought: undefined } as score.Project, output, replace);
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
        const result = await StartRender({ ...review.project, thoughts: project?.thoughts, play_order: project?.play_order, thought: undefined } as score.Project, output, replace);
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
  const jacketKey = `${song?.song_id || ''}\u0000${currentDiff}`;
  const jacketURL = jacketCache.current.get(jacketKey) || (jacket.key === jacketKey ? jacket.url : '');
  const currentSongVideo = reviewPending ? song?.video : review?.project?.songs[songIndex]?.video || song?.video;
  const currentVideo = videoStates.find((state) => state.index === songIndex + 1);
  const videoLength = (currentVideo?.duration_ms || 0) / 1000;
  const videoLinkChanged = videoLink !== (currentVideo?.url || '');
  const clipStart = parseClipTime(clipStartText);
  const clipEnd = parseClipTime(clipEndText);
  const rangeStart = Math.min(clipStart ?? 0, videoLength);
  const rangeEnd = Math.min(clipEnd ?? 0, videoLength);
  const clipError = videoLength > 0 ? clipRangeError(clipStart, clipEnd, videoLength) : '请先准备录像预览';
  const clipDuration = clipStart !== null && clipEnd !== null ? Math.max(0, clipEnd - clipStart) : 0;
  const savedClip = !videoLinkChanged && clipSaved(song?.clip, clipStart, clipEnd) && Boolean(currentVideo?.confirmed);
  const materialBlockers = project ? usedSongIndexes(project).filter((index) => {
    const state = videoStates.find((item) => item.index === index);
    const item = project.songs[index - 1];
    const key = `${projectEpoch}:${index - 1}:${videoSourceKey(item ? [item] : [])}`;
    const draft = clipDrafts[key];
    return !item?.clip?.confirmed_key || !state?.confirmed || !state.material_ready || Boolean(draft && !clipSaved(item.clip, parseClipTime(draft.start), parseClipTime(draft.end))) || (videoLinkDrafts[key] !== undefined && videoLinkDrafts[key] !== (state?.url || ''));
  }) : [];
  async function saveClipRange() {
    const selected = review?.project?.songs[songIndex];
    if (!selected || clipSaving || reviewPending || videoLinkChanged || clipError || clipStart === null || clipEnd === null) return false;
    const epoch = lifecycle.current;
    const captured = latestEdit.current;
    const original = captured.project?.songs[songIndex];
    if (!original) return false;
    const index = songIndex;
    setClipSaving(true);
    setVideoError('');
    const stillCurrent = () => {
      const latest = latestEdit.current;
      return epoch === lifecycle.current && captured.draftKey === latest.draftKey && captured.clipStartText === latest.clipStartText && captured.clipEndText === latest.clipEndText && Boolean(latest.project?.songs[index]) && videoSourceKey([latest.project!.songs[index]]) === videoSourceKey([original]);
    };
    try {
      const result = await videos.confirm(selected, new score.ClipRange({ start_ms: Math.round(clipStart * 1000), duration_ms: Math.round((clipEnd - clipStart) * 1000) }), stillCurrent);
      if (!result || !stillCurrent()) return false;
      editSongAt(index, (item) => {
        if (videoSourceKey([item]) !== videoSourceKey([original])) return;
        item.video = result.song.video;
        item.difficulty.name = result.song.difficulty.name;
        item.clip = result.song.clip;
      });
      return true;
    } catch (error) { if (epoch === lifecycle.current) setVideoError(errorText(error)); return false; }
    finally { if (epoch === lifecycle.current) setClipSaving(false); }
  }
  const clipChanged = clipStart !== (song?.clip?.start_ms || 0) / 1000 || clipEnd !== ((song?.clip?.start_ms || 0) + (song?.clip?.duration_ms || 10000)) / 1000;
  const canApplyClip = Boolean(currentVideo?.preview_ready) && !videoLinkChanged;
  function switchSong(index: number) {
    if (index === count) addBlankSong();
    else { setSongIndex(index); setSearch(''); }
  }
  function requestSong(index: number) {
    if (index === songIndex || index < 0 || index > count || clipSaving) return;
    if (song && !savedClip && (currentVideo?.preview_ready || clipChanged || videoLinkChanged)) {
      setSwitchTarget(index); switchDialog.current?.showModal();
    } else switchSong(index);
  }
  function commitSongJump() {
    const value = songJumpDraft;
    setSongJumpDraft(null);
    if (value !== null && /^\d+$/.test(value) && Number(value) >= 1 && Number(value) <= count) requestSong(Number(value) - 1);
  }
  function finishPreviewScrub() {
    const elapsed = previewScrub.current;
    if (elapsed === null) return;
    previewScrub.current = null;
    if (previewRef.current && clipStart !== null) previewRef.current.currentTime = clipStart + elapsed;
  }
  function appliedVideo(state: app.VideoState) {
    const item = project?.songs[state.index - 1];
    const key = `${projectEpoch}:${state.index - 1}:${videoSourceKey(item ? [item] : [])}`;
    const draft = clipDrafts[key];
    return Boolean(item?.clip?.confirmed_key && state.confirmed && (!draft || clipSaved(item.clip, parseClipTime(draft.start), parseClipTime(draft.end))) && (videoLinkDrafts[key] === undefined || videoLinkDrafts[key] === state.url));
  }
  const unappliedVideos = videoStates.filter((state) => !appliedVideo(state));
  const appliedPendingVideos = videoStates.filter((state) => appliedVideo(state) && !state.material_ready);
  const doneCount = logs.filter((event) => event.stage === 'render' && ['done', 'skipped'].includes(event.status)).length;
  const active = renderState === 'starting' || renderState === 'running';

  return (
    <div className="shell">
      <header className="topbar">
          <nav aria-label="制作流程">
            {pages.slice(0, 6).map((label, index) => (
              <button
                key={label}
                className={`nav-item ${page === index ? 'selected' : ''}`}
                onClick={() => setPage(index)}
                aria-current={page === index ? 'page' : undefined}
              >
                <span className="nav-label">{label}</span>
              </button>
            ))}
            <button className={`nav-item settings-link ${page === 6 ? 'selected' : ''}`} onClick={() => setPage(6)} aria-current={page === 6 ? 'page' : undefined}>
              <span aria-hidden="true">⚙</span><span className="nav-label">设置</span>
            </button>
          </nav>
      </header>
      <div className="workspace">
        <main className="main-area">
          <div className="page-heading">
            <div>
              <h1>{pages[page]}</h1>
            </div>
            {page === 2 && count > 0 && <span className="heading-tag">第 {songIndex + 1} / {count} 首</span>}
          </div>

          {page === 6 && (
            <div className="content-grid">
              <section className="card wide">
                <h2>环境设置</h2>
                <div className="tool-list">
                  {toolNames.map((name) => {
                    const found = status?.tools?.find((tool) => tool.Name === name);
                    const check = checks[name];
                    return (
                      <div className="tool-row" key={name}>
                        <div className="tool-identity">
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
                <button className="button primary" onClick={() => {
                  localStorage.setItem('environment-ready', '1');
                  setConfigSaved(true);
                }}>
                  保存配置
                </button>
                {configSaved && <div className="notice">配置已保存，下次启动将直接进入导入页。</div>}
                {status && status.tools.some((tool) => !tool.Path) && <div className="notice warn">请先补齐未找到的工具路径。</div>}
              </section>
              <aside className="card side-card">
                <h2>关于与资源</h2>
                <p>作者：IzayoiSakuye</p>
                <p>版本：{version || '读取中…'}</p>
                <div className="status-figure">
                  <strong>
                    {status?.song_count ?? '—'}{' '}
                    <small>/ {status && !status.data_error ? status.song_count : '—'}</small>
                  </strong>
                  <span>已载入 / 数据包共计（首）</span>
                </div>
                <div className="rule" />
                <h2>资源更新</h2>
                <button className="button quiet" disabled={resourceBusy || !!opBusy || checkingResources} onClick={checkAllResources}>
                  {checkingResources ? '检查中…' : '检查更新'}
                </button>
                <button className="button quiet" disabled={resourceBusy || checkingResources} onClick={updateResources}>
                  {resourceBusy ? '更新中…' : '更新资源'}
                </button>
                {resourceBusy && resourceProgress && !opBusy && <p>下载中：{(resourceProgress.downloaded / 1048576).toFixed(1)} / {(resourceProgress.total / 1048576).toFixed(1)} MB</p>}
                {resourceBusy && opBusy && <p>{opMessage}{opProgress?.kind === opBusy ? `：${(opProgress.downloaded / 1048576).toFixed(1)} / ${(opProgress.total / 1048576).toFixed(1)} MB` : ''}</p>}
                {resourceMessage && <div className="notice" role="status">{resourceMessage}</div>}
                <div className="rule" />
                <p>清理可重新生成的渲染缓存，不影响已下载录像。</p>
                <button className="button quiet" disabled={cacheBusy || active || !cacheInfo || cacheInfo.rendering || cacheInfo.downloading || cacheInfo.in_use || cacheInfo.preview_count > 0} onClick={() => clearCache(false)}>清理渲染缓存</button>
                <button className="button quiet" disabled={cacheBusy || active || !cacheInfo || cacheInfo.rendering || cacheInfo.downloading || cacheInfo.in_use || cacheInfo.preview_count > 0} onClick={() => clearCacheDialog.current?.showModal()}>清理全部缓存</button>
                {(active || cacheInfo?.rendering || cacheInfo?.downloading || cacheInfo?.in_use || Boolean(cacheInfo?.preview_count)) && <p className="muted">生成、下载或预览正在占用缓存，请结束后清理。</p>}
                {cacheMessage && <div className="notice">{cacheMessage}</div>}
              </aside>
            </div>
          )}

          {page === 0 && (
            <div className="content-grid">
              <section className="card wide">
                <h2>导入方式</h2>
                {status?.data_error && <div className="notice warn">资源未就绪：{status.data_error} <button className="button quiet" onClick={() => setPage(6)}>前往设置更新资源</button></div>}
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
                        ? '选择 B50 成绩图'
                        : '选择 JSON 文件'}
                  </strong>
                  {importMode === 'json'}
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
                      placeholder="在此粘贴成绩 JSON"
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
                    {importBusy ? '正在导入…' : '导入'}
                  </button>
                  <button className="button quiet" disabled={importBusy || active} onClick={addBlankSong}>手动新增歌曲</button>
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
                  </>
                ) : (
                  <p className="empty-aside">尚未导入成绩</p>
                )}
              </aside>
            </div>
          )}

          {page === 1 && (project ? (
            <>
              <section className="card thought-library">
                <div className="thought-library-head"><h2>已保存的感想片段</h2><button className="button quiet" onClick={newThought}>新建片段</button></div>
                {!!project.thoughts?.length && <div className="thought-library-list">{project.thoughts.map((thought) => <div className={`thought-library-item ${thought.id === thoughtID ? 'selected' : ''}`} key={thought.id}>
                  <button className="button text" onClick={() => selectThought(thought.id || '')}>{thought.name} · {Math.round(thought.duration_ms / 1000)} 秒</button>
                  <button className="thought-library-delete" onClick={() => deleteThought(thought.id || '')} aria-label={`删除 ${thought.name}`}>删除</button>
                </div>)}</div>}
              </section>
              <ThoughtEditor key={thoughtEditorVersion} draft={thoughtDraft} saved={thoughtFromProject(project.thoughts?.find((item) => item.id === thoughtID))} options={bgmOptions} optionsLoading={bgmLoading} optionsError={bgmError} onChange={setThoughtDraft} onSave={saveThought} />
            </>
          ) : (
            <section className="card placeholder"><h2>请先导入成绩</h2><button className="button primary" onClick={() => setPage(0)}>前往导入</button></section>
          ))}

          {page === 3 && (project ? <Arrangement project={project} rows={review?.rows || []} onChange={(play_order) => setProject((current) => current ? { ...current, play_order } as score.Project : current)} /> : <section className="card placeholder"><h2>请先导入成绩</h2></section>)}

          {page === 2 &&
            (song ? (
              <>
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
                <div className="review-toolbar">
                  <label className="field compact">
                    跳转到第{' '}
                    <input
                      type="number"
                      min={1}
                      max={count}
                      value={songJumpDraft ?? String(songIndex + 1)}
                      onChange={(event) => setSongJumpDraft(event.target.value)}
                      onBlur={commitSongJump}
                      onKeyDown={(event) => { if (event.key === 'Enter') { event.preventDefault(); event.currentTarget.blur(); } }}
                    />
                    {count} 首
                  </label>
                  <label className="checkline">
                    <input
                      type="checkbox"
                      checked={onlyProblems}
                      onChange={(event) => setOnlyProblems(event.target.checked)}
                    />{' '}
                    只看有问题的曲目
                  </label>
                  <div className="spacer" />
                  <button className="button quiet" onClick={() => stepSong(-1)}>
                    ← 上一首
                  </button>
                  <button className="button quiet" disabled={clipSaving || (songIndex === count - 1 && active)} onClick={() => songIndex === count - 1 ? requestSong(count) : stepSong(1)}>
                    {songIndex === count - 1 ? '新增歌曲 →' : '下一首 →'}
                  </button>
                </div>
                <div className="content-grid review-grid">
                  <section className="card wide">
                    <h2>
                      成绩信息 <span>#{String(songIndex + 1).padStart(2, '0')}</span>
                    </h2>
                    <div className="song-title">
                      {jacketURL ? (
                        <img
                          className="jacket-placeholder"
                          src={jacketURL}
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
                            placeholder={song.song_id || '搜索歌名或id'}
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
                      <label className="field">单曲 PTT
                        <input type="number" min={0} max={20} step="0.0001"
                          value={song.score.ptt ?? ''} placeholder={row?.ptt?.toFixed(4) || '未计算'}
                          onChange={(event) => editSong((item) => { item.score.ptt = event.target.value === '' ? undefined : Number(event.target.value); })} />
                      </label>
                      <label className="field">定数
                        <input type="number" min={0} max={20} step="0.1"
                          value={song.difficulty.constant || ''} placeholder={row?.constant?.toString() || '未匹配'}
                          onChange={(event) => editSong((item) => { item.difficulty.constant = Number(event.target.value); })} />
                      </label>
                      <label className="field">评级
                        <select value={song.score.rank || ''} onChange={(event) => editSong((item) => { item.score.rank = event.target.value; })}>
                          <option value="">{row?.rank ? `自动计算：${row.rank}` : '未填写'}</option>
                          {['EX+', 'EX', 'AA', 'A', 'B', 'C', 'D'].map((rank) => <option key={rank} value={rank}>{rank}</option>)}
                        </select>
                      </label>
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
                          placeholder="言って"
                        />
                        <small className="comment-hint">AI酱提醒您：字体大小默认 44px，约 36 字/行；文字在面板内垂直居中，行数过多时支持自动缩小字号，过长仍可能超出画面。回车会保留换行。</small>
                      </label>
                    </div>
                  </section>
                  <aside className="right-stack">
                    <section className="card video-card">
                      <h2>
                        录像预览{' '}
                        <span>
                          {videoLinkChanged ? '链接未应用' : currentVideo?.status === 'ready' && !currentVideo.material_ready ? '可预览，待确认' : videoStatusText[currentVideo?.status || 'missing']}
                        </span>
                      </h2>
                      {currentVideo?.preview_ready && currentVideo.preview_url && !videoLinkChanged ? (
                        <VideoPreview key={`${projectEpoch}:${songIndex}:${currentVideo.preview_url}:${currentVideo.preview_audio_url || ''}`}
                          videoRef={previewRef} src={currentVideo.preview_url} audioSrc={currentVideo.preview_audio_url}
                          start={clipStart} end={clipEnd} invalid={Boolean(clipError)} scrubbing={previewScrub} onTime={setPreviewTime}
                          onPlaying={setPreviewPlaying} onError={setVideoError} />
                      ) : <div className="video-empty">{currentVideo?.status === 'ready' && !currentVideo.material_ready ? '可预览，待确认' : videoStatusText[currentVideo?.status || 'missing']}</div>}
                      {videoLength > 0 && !videoLinkChanged && (
                        <div className="clip-picker">
                          <div className="clip-playback">
                            <button className="button quiet" disabled={Boolean(clipError)} onClick={() => {
                              const video = previewRef.current;
                              if (!video) return;
                              if (video.paused) void video.play().catch(() => {});
                              else video.pause();
                            }}>{previewPlaying ? '暂停预览' : '播放片段'}</button>
                            <input aria-label="片段预览进度" type="range" min={0} max={clipDuration} step={0.1}
                              disabled={Boolean(clipError)}
                              value={Math.min(previewTime, clipDuration)}
                              onPointerDown={(event) => { previewScrub.current = previewTime; event.currentTarget.setPointerCapture(event.pointerId); }}
                              onPointerUp={finishPreviewScrub} onPointerCancel={finishPreviewScrub} onLostPointerCapture={finishPreviewScrub}
                              onChange={(event) => {
                                const elapsed = Number(event.target.value);
                                setPreviewTime(elapsed);
                                if (previewScrub.current !== null) previewScrub.current = elapsed;
                                else if (previewRef.current && clipStart !== null) previewRef.current.currentTime = clipStart + elapsed;
                              }} />
                            <span>{formatClipTime(Math.min(previewTime, clipDuration))} / {formatClipTime(clipDuration)}</span>
                          </div>
                          <div className="clip-range">
                            <div className="clip-range-rail">
                              <div className={`clip-range-selected${clipError ? ' invalid' : ''}`} style={{
                                left: `${Math.min(rangeStart, rangeEnd) / videoLength * 100}%`,
                                width: `${Math.abs(rangeEnd - rangeStart) / videoLength * 100}%`,
                              }} />
                            </div>
                            <input aria-label="拖动开始时间" title="开始时间" className="clip-range-start"
                              type="range" min={0} max={videoLength} step={0.1}
                              value={rangeStart}
                              onChange={(event) => {
                                const start = Number(event.target.value);
                                setClipStartText(formatClipTime(start));
                                setPreviewTime(0);
                                if (previewRef.current) previewRef.current.currentTime = start;
                              }} />
                            <input aria-label="拖动结束时间" title="结束时间" className="clip-range-end"
                              type="range" min={0} max={videoLength} step={0.1}
                              value={rangeEnd}
                              onChange={(event) => {
                                setClipEndText(formatClipTime(Number(event.target.value)));
                                setPreviewTime(0);
                                if (previewRef.current && clipStart !== null) previewRef.current.currentTime = clipStart;
                              }} />
                          </div>
                          <div className="clip-times">
                            <label className="field">开始时间
                              <input type="text" inputMode="decimal" value={clipStartText} placeholder="0:00"
                                onChange={(event) => {
                                  const value = event.target.value;
                                  setClipStartText(value);
                                  setPreviewTime(0);
                                  const start = parseClipTime(value);
                                  if (previewRef.current && start !== null && start >= 0 && start < videoLength) previewRef.current.currentTime = start;
                                }} onBlur={() => {
                                  const start = parseClipTime(clipStartText);
                                  if (start !== null) setClipStartText(formatClipTime(start));
                                }} />
                            </label>
                            <label className="field">结束时间
                              <input type="text" inputMode="decimal" value={clipEndText} placeholder="0:10"
                                onChange={(event) => {
                                  setClipEndText(event.target.value);
                                  setPreviewTime(0);
                                  if (previewRef.current && clipStart !== null) previewRef.current.currentTime = clipStart;
                                }} onBlur={() => {
                                  const end = parseClipTime(clipEndText);
                                  if (end !== null) setClipEndText(formatClipTime(end));
                                }} />
                            </label>
                          </div>
                          <button className="button primary" disabled={clipSaving || reviewPending || active || Boolean(clipError) || savedClip} onClick={() => { void saveClipRange(); }}>{clipSaving ? '应用中…' : '应用片段'}</button>
                          {clipError && <div className="clip-footer"><span className="clip-error" role="alert">{clipError}</span></div>}
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
                          disabled={clipSaving || active}
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
                          disabled={clipSaving || active || (!videoLinkChanged && !currentSongVideo?.local_path)}
                        >
                          应用链接
                        </button>
                        <button className="button quiet" onClick={pickVideo} disabled={clipSaving || active}>
                          选择本地文件
                        </button>
                        <button
                          className="button primary"
                          onClick={downloadVideo}
                          disabled={
                            reviewPending ||
                            videoBusy ||
                            videoLinkChanged ||
                            !currentDiff ||
                            Boolean(currentVideo?.preview_ready)
                          }
                        >
                          {videoBusy ? '准备中…' : '准备预览'}
                        </button>
                      </div>
                      {(videoError || videos.error || currentVideo?.error) && (
                        <div className="notice bad" role="alert">
                          {videoError || videos.error || currentVideo?.error}
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
                            <p className="muted">本首暂无问题</p>
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
                                {acknowledgedNotes.includes(note.index) ? '撤销核对' : '已核对'}
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
                <p>请先在「导入图片 / JSON」选项卡导入成绩。</p>
                <button className="button primary" onClick={() => setPage(0)}>
                  前往导入
                </button>
              </section>
            ))}

          {page === 4 && (
            <div className="content-grid">
              <section className="card wide">
                <h2>生成任务</h2>
                <div className="output-picker">
                  <div>
                    <span>输出文件</span>
                    <strong>{output ? nameOf(output) : '尚未选择保存位置'}</strong>
                    <small title={output}>{output || ''}</small>
                  </div>
                  <button className="button quiet" onClick={pickOutput} disabled={active}>
                    选择路径
                  </button>
                </div>
                <div className="progress-head">
                  <strong>
                    {renderState === 'done'
                      ? '已完成，可前往设置清理缓存'
                      : active
                        ? '正在生成'
                        : renderState === 'failed'
                          ? '生成失败'
                          : '等待开始'}
                  </strong>
                  <span>
                    {doneCount} / {renderTotal || project?.play_order?.length || count} 段完成
                  </span>
                </div>
                <div className="progress-track">
                  <div
                    style={{ width: renderTotal ? `${Math.min(100, (doneCount / renderTotal) * 100)}%` : '0%' }}
                  />
                </div>
                <div className="action-line">
                  <button
                    className="button primary"
                    disabled={reviewPending || !review?.renderable || !output || active || materialBlockers.length > 0}
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
                  <div className="notice warn">成绩存在问题，请在「识别信息」选项卡校对。</div>
                )}
                {renderError && (
                  <div className="notice bad" role="alert">
                    {renderError}
                  </div>
                )}
              </section>
              <aside className="right-stack">
                {videoStates.length > 0 && <section className="card material-summary">
                  <h2>素材准备 <span>{videoStates.filter((state) => state.material_ready).length} / {project?.songs.length || 0} 首就绪</span></h2>
                  {unappliedVideos.length > 0 && <p className="muted">未应用片段：{unappliedVideos.length} 首</p>}
                  {unappliedVideos.map((state) => <div className="material-row" key={`${state.song_id}:${state.difficulty}`}>
                    <strong>#{state.index} {review?.rows[state.index - 1]?.title || state.song_id} · {state.difficulty}</strong>
                    <span>{state.status === 'ready' && !state.material_ready ? '可预览' : videoStatusText[state.status] || state.status}</span>
                    {['preparing', 'queued', 'downloading'].includes(state.status) && <progress aria-label={`第 ${state.index} 首素材进度`} max="1" value={state.progress < 0 ? undefined : state.progress} />}
                    {state.error && <span className="clip-error">{state.error}</span>}
                    {['failed', 'cancelled'].includes(state.status) && state.request_id && <button className="button quiet" disabled={active} onClick={() => { void changeVideoTask(state, true); }}>重试</button>}
                    {['preparing', 'queued', 'downloading'].includes(state.status) && state.request_id && <button className="button quiet" disabled={active} onClick={() => { void changeVideoTask(state, false); }}>取消</button>}
                    <button className="button text" onClick={() => jumpProblem(state.index)}>去编辑</button>
                  </div>)}
                  {!unappliedVideos.length && <p className="muted">所有片段已应用。</p>}
                  {appliedPendingVideos.map((state) => <div className="material-status" key={`${state.song_id}:${state.difficulty}`}>
                    <span>已应用 #{state.index} · {videoStatusText[state.status] || state.status}{state.error ? `：${state.error}` : ''}</span>
                    {['preparing', 'queued', 'downloading'].includes(state.status) && <progress aria-label={`第 ${state.index} 首素材进度`} max="1" value={state.progress < 0 ? undefined : state.progress} />}
                    {['failed', 'cancelled'].includes(state.status) && state.request_id && <button className="button quiet" disabled={active} onClick={() => { void changeVideoTask(state, true); }}>重试</button>}
                    {['preparing', 'queued', 'downloading'].includes(state.status) && state.request_id && <button className="button quiet" disabled={active} onClick={() => { void changeVideoTask(state, false); }}>取消</button>}
                    <button className="button text" onClick={() => jumpProblem(state.index)}>去编辑</button>
                  </div>)}
                </section>}
                {(videoError || videos.error) && <p className="notice bad" role="alert">{videoError || videos.error}</p>}
                <section className="card log-card">
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
                      <div className="log-empty">这里可以看到生成进度喵</div>
                    )}
                  </div>
                </section>
              </aside>
            </div>
          )}

          {page === 5 &&
            (finished ? (
              <section className="card completion">
                <div className="completion-icon">✓</div>
                <span className="eyebrow">RENDER COMPLETE</span>
                <h2>视频已生成</h2>
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
              </section>
            ))}
        </main>
      </div>
      <dialog ref={switchDialog} className="confirm-dialog" aria-labelledby="unsaved-clip-title" onCancel={(event) => { if (clipSaving) event.preventDefault(); else setSwitchTarget(null); }}>
        <h2 id="unsaved-clip-title">当前片段尚未应用</h2><p>{canApplyClip ? '当前片段有未应用的范围，请应用后切换歌曲。' : '当前修改尚未应用。可以保留草稿并切换，返回后继续编辑。'}</p>
        {(videoError || videos.error) && <p className="notice bad" role="alert">{videoError || videos.error}</p>}
        <div className="action-line"><button className="button quiet" disabled={clipSaving} onClick={() => { setSwitchTarget(null); switchDialog.current?.close(); }}>继续编辑</button>
          <button className="button primary" disabled={clipSaving || (canApplyClip && (reviewPending || Boolean(clipError)))} onClick={async () => {
            if (!canApplyClip || await saveClipRange()) { const next = switchTarget; setSwitchTarget(null); switchDialog.current?.close(); if (next !== null) switchSong(next); }
          }}>{clipSaving ? '应用中…' : canApplyClip ? '应用并切换' : '保留草稿并切换'}</button></div>
      </dialog>
      <dialog ref={clearCacheDialog} className="confirm-dialog" aria-labelledby="clear-cache-title">
        <h2 id="clear-cache-title">清理全部缓存？</h2>
        <p>将清理渲染缓存和下载的录像素材。用户选择的本地文件受后端保护；清理后需要重新准备素材。</p>
        <div className="confirm-actions">
          <button className="button quiet" autoFocus onClick={() => clearCacheDialog.current?.close()}>取消</button>
          <button className="button primary" onClick={() => {
            clearCacheDialog.current?.close();
            void clearCache(true);
          }}>确认清理</button>
        </div>
      </dialog>
      <dialog ref={resourceDialog} className="confirm-dialog" aria-labelledby="resource-update-title" onCancel={(event) => {
        if (resourceBusy) event.preventDefault();
      }}>
        <h2 id="resource-update-title">{updateFinished ? '资源更新结束' : '发现资源更新'}</h2>
        <p>{resourceInfo?.restart_required
          ? '新资源已安装，重启应用后生效。'
          : `曲库：${resourceInfo?.update_available ? `发现版本 ${resourceInfo.latest_version}` : '已是最新'}；OP：${opInfo?.files.filter((file) => file.update_available).length || 0} 首待更新。`}</p>
        {resourceBusy && !resourceProgress && !opBusy && <p role="status">正在检查更新…</p>}
        {resourceBusy && resourceProgress && !opBusy && (
          <div className="resource-download" role="status">
            <span>{resourceProgress.total > 0 && resourceProgress.downloaded >= resourceProgress.total
              ? '下载完成，正在安装…'
              : `下载中：${(resourceProgress.downloaded / 1048576).toFixed(1)} / ${(resourceProgress.total / 1048576).toFixed(1)} MB`}</span>
            <div className="progress-track"><div style={{ width: `${resourceProgress.total > 0 ? Math.min(100, resourceProgress.downloaded / resourceProgress.total * 100) : 0}%` }} /></div>
          </div>
        )}
        {opBusy && <div className="resource-download" role="status">
          <span>{opMessage}{opProgress?.kind === opBusy ? `：${(opProgress.downloaded / 1048576).toFixed(1)} / ${(opProgress.total / 1048576).toFixed(1)} MB` : ''}</span>
          {opProgress?.kind === opBusy && <div className="progress-track"><div style={{ width: `${opProgress.total > 0 ? Math.min(100, opProgress.downloaded / opProgress.total * 100) : 0}%` }} /></div>}
        </div>}
        {resourceMessage.startsWith('资源更新失败：') && <div className="notice bad" role="alert">{resourceMessage}</div>}
        <div className="confirm-actions">
          <button className="button quiet" disabled={resourceBusy} onClick={() => resourceDialog.current?.close()}>
            {updateFinished ? '关闭' : '稍后'}
          </button>
          {!updateFinished && <button className="button primary" disabled={resourceBusy} onClick={updateResources}>
            {resourceBusy ? '更新中…' : '更新资源'}
          </button>}
        </div>
      </dialog>
      {page !== 5 && page !== 6 && (
        <footer className="footer">
          <button className="button quiet" disabled={page === 0} onClick={() => setPage(page - 1)}>
            ← 上一步
          </button>
          <button className="button primary" onClick={() => setPage(page + 1)}>
            下一步 →
          </button>
        </footer>
      )}
    </div>
  );
}

export default App;
