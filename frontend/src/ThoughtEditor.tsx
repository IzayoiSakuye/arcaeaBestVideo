import { useEffect, useRef, useState } from 'react';
import { PickThoughtAudio, PickThoughtImage, RenderThought, ThoughtAudio, ThoughtImage } from '../wailsjs/go/app/App';
import { score, type app } from '../wailsjs/go/models';
import { clipRangeError, formatClipTime, parseClipTime } from './clip';
import { nameOf } from './format';
import { sameThought, type ThoughtDraft } from './thought';

type Props = {
  draft: ThoughtDraft;
  saved: ThoughtDraft | null;
  options: app.ThoughtAudioOption[];
  optionsLoading: boolean;
  optionsError: string;
  onChange: (thought: ThoughtDraft) => void;
  onSave: (thought: ThoughtDraft) => void;
};

function errorText(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

export default function ThoughtEditor({ draft, saved, options, optionsLoading, optionsError, onChange, onSave }: Props) {
  const [media, setMedia] = useState<app.ThoughtMedia | null>(null);
  const [imageURLs, setImageURLs] = useState<Record<string, string>>({});
  const [imageRatios, setImageRatios] = useState<Record<string, number>>({});
  const [audioLoading, setAudioLoading] = useState(false);
  const [imageLoading, setImageLoading] = useState(false);
  const [audioStart, setAudioStart] = useState('0:00');
  const [audioEnd, setAudioEnd] = useState('0:00');
  const [silentDuration, setSilentDuration] = useState('0:10');
  const [previewTime, setPreviewTime] = useState(0);
  const [previewURL, setPreviewURL] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [overflow, setOverflow] = useState(false);
  const [stageWidth, setStageWidth] = useState(960);
  const [frameReady, setFrameReady] = useState(false);
  const stageRef = useRef<HTMLDivElement>(null);
  const frameRef = useRef<HTMLIFrameElement>(null);
  const dragRef = useRef<{ page: number; image: number; x: number; y: number; clientX: number; clientY: number } | null>(null);
  const draftVersion = useRef(0);

  useEffect(() => {
    if (draft.audio.kind === 'silent' || draft.audio.kind.startsWith('op')) {
      if (draft.audio.kind === 'silent') setMedia(null);
      setSilentDuration(formatClipTime(draft.duration_ms / 1000));
      if (draft.audio.kind === 'silent') return;
    }
    const option = options.find((item) => item.kind === draft.audio.kind && item.available);
    if (option) setMedia(option);
    else if (draft.audio.kind === 'custom' && draft.audio.path) {
      setAudioLoading(true);
      ThoughtAudio(draft.audio.path).then(setMedia).catch((e) => setError(errorText(e))).finally(() => setAudioLoading(false));
    }
    setAudioStart(formatClipTime(draft.audio.start_ms / 1000));
    setAudioEnd(formatClipTime(draft.audio.end_ms / 1000));
  }, [draft.audio.kind, draft.audio.path, options]);
  const imagePaths = draft.pages.flatMap((page) => page.images.map((image) => image.path)).join('\u0000');
  useEffect(() => {
    for (const path of imagePaths.split('\u0000').filter(Boolean)) {
      if (!imageURLs[path]) ThoughtImage(path).then((item) => setImageURLs((old) => ({ ...old, [path]: item.preview_url }))).catch((e) => setError(errorText(e)));
    }
  }, [imagePaths]);
  useEffect(() => {
    if (!stageRef.current) return;
    const observer = new ResizeObserver(([entry]) => setStageWidth(entry.contentRect.width));
    observer.observe(stageRef.current);
    return () => observer.disconnect();
  }, []);
  useEffect(() => {
    for (const [path, url] of Object.entries(imageURLs)) {
      if (imageRatios[path]) continue;
      const img = new Image();
      img.onload = () => setImageRatios((old) => ({ ...old, [path]: img.naturalHeight / img.naturalWidth }));
      img.src = url;
    }
  }, [imageURLs]);

  const seconds = draft.duration_ms / 1000;
  const activePage = Math.max(0, Math.min(draft.pages.length - 1, Math.floor(previewTime / Math.max(seconds / draft.pages.length, 0.001))));
  useEffect(() => {
    frameRef.current?.contentWindow?.postMessage({ type: 'thought-preview', text: draft.pages[activePage]?.text || '',
      images: draft.pages[activePage]?.images.map((image) => ({ source: imageURLs[image.path], x: image.x, y: image.y, width: image.width }))
        .filter((image) => image.source) || [] }, '*');
  }, [draft, activePage, imageURLs, frameReady]);
  useEffect(() => {
    const node = frameRef.current?.contentDocument?.getElementById('text');
    if (!node) return;
    let tooLong = false;
    for (const page of draft.pages) {
      node.textContent = page.text;
      if (node.scrollHeight > node.clientHeight + 1) tooLong = true;
    }
    node.textContent = draft.pages[activePage]?.text || '';
    setOverflow(tooLong);
  }, [draft.pages, activePage, frameReady]);
  const start = parseClipTime(audioStart);
  const end = parseClipTime(audioEnd);
  const audioError = draft.audio.kind !== 'silent' && media
    ? clipRangeError(start, end, media.duration_ms / 1000) : '';
  const reversedRange = start !== null && end !== null && start > end;
  const railStart = Math.min(start ?? draft.audio.start_ms / 1000, end ?? draft.audio.end_ms / 1000);
  const railEnd = Math.max(start ?? draft.audio.start_ms / 1000, end ?? draft.audio.end_ms / 1000);
  const durationValue = parseClipTime(silentDuration);
  const durationError = (draft.audio.kind === 'silent' || draft.audio.kind.startsWith('op')) && (durationValue === null || durationValue < 1 || durationValue > 600)
    ? '片段时长须为 0:01 到 10:00' : '';
  const formError = !draft.name.trim() ? '请填写片段名称'
    : draft.pages.some((page) => !page.text.trim()) ? '每段感想都要填'
    : draft.pages.length > 20 ? '最多添加 20 段文字'
    : !['silent', 'custom', ...Array.from({ length: 7 }, (_, i) => `op${i + 1}`)].includes(draft.audio.kind) ? '请选择背景音乐'
    : draft.audio.kind !== 'silent' && !media ? '请选择可用的音频文件'
    : audioError || durationError || (seconds / draft.pages.length < 0.5 ? '每段文字至少需要 0.5 秒' : '') ||
      (seconds > 600 ? '片段时长不能超过 10 分钟' : '') || (overflow ? '有文字超出文字框，请缩短' : '');

  function currentThought(): ThoughtDraft {
    if (draft.audio.kind === 'silent') return { ...draft, duration_ms: Math.round((durationValue || 0) * 1000) };
    const startMs = Math.round((start || 0) * 1000);
    const endMs = Math.round((end || 0) * 1000);
    return { ...draft, duration_ms: draft.audio.kind.startsWith('op') ? Math.round((durationValue || 0) * 1000) : endMs - startMs,
      audio: { ...draft.audio, start_ms: startMs, end_ms: endMs } };
  }

  function change(next: ThoughtDraft) {
    draftVersion.current++;
    setPreviewURL('');
    setError('');
    onChange(next);
  }
  function editPage(index: number, edit: (page: ThoughtDraft['pages'][number]) => void) {
    const pages = structuredClone(draft.pages);
    edit(pages[index]);
    change({ ...draft, pages });
  }
  function updateText(index: number, value: string) {
    editPage(index, (page) => { page.text = value; });
  }
  function updateRange(startSeconds: number, endSeconds: number) {
    const startMs = Math.round(startSeconds * 1000);
    const endMs = Math.round(endSeconds * 1000);
    setAudioStart(formatClipTime(startSeconds));
    setAudioEnd(formatClipTime(endSeconds));
    change({ ...draft, duration_ms: draft.audio.kind.startsWith('op') ? draft.duration_ms : Math.max(0, endMs - startMs), audio: { ...draft.audio, start_ms: startMs, end_ms: endMs } });
  }
  async function chooseAudio() {
    setAudioLoading(true);
    try {
      const item = await PickThoughtAudio();
      if (!item.path) return;
      const endMs = Math.min(item.duration_ms, 30000);
      setMedia(item);
      setAudioStart('0:00');
      setAudioEnd(formatClipTime(endMs / 1000));
      change({ ...draft, duration_ms: endMs, audio: { kind: 'custom', path: item.path, start_ms: 0, end_ms: endMs } });
    } catch (e) { setError(errorText(e)); }
    finally { setAudioLoading(false); }
  }
  async function chooseImage(pageIndex: number) {
    setImageLoading(true);
    try {
      const item = await PickThoughtImage();
      if (!item.path) return;
      setImageURLs((old) => ({ ...old, [item.path]: item.preview_url }));
      editPage(pageIndex, (page) => { page.images.push({ path: item.path, x: 1250, y: 420, width: 360 }); });
      setPreviewTime((pageIndex + 0.5) * seconds / draft.pages.length);
    } catch (e) { setError(errorText(e)); }
    finally { setImageLoading(false); }
  }
  async function render() {
    if (formError) { setError(formError); return; }
    setBusy(true);
    setError('');
    const version = draftVersion.current;
    try {
      const thought = currentThought();
      const result = await RenderThought(new score.Thought(thought));
      if (version === draftVersion.current) setPreviewURL(result.preview_url);
    } catch (e) { setError(errorText(e)); }
    finally { setBusy(false); }
  }
  function dragImage(event: React.PointerEvent<HTMLDivElement>) {
    if (!dragRef.current || !stageRef.current) return;
    const scale = 1920 / stageRef.current.clientWidth;
    const x = Math.max(0, Math.min(1919, Math.round(dragRef.current.x + (event.clientX - dragRef.current.clientX) * scale)));
    const y = Math.max(0, Math.min(1079, Math.round(dragRef.current.y + (event.clientY - dragRef.current.clientY) * scale)));
    const { page, image } = dragRef.current;
    editPage(page, (item) => { item.images[image].x = x; item.images[image].y = y; });
  }

  const savedCurrent = saved !== null && sameThought(saved, currentThought());

  return <div className="thought-grid">
    <section className="card thought-editor">
      <h2>感想片段</h2>
      <label className="field full">片段名称
        <input value={draft.name} onChange={(e) => change({ ...draft, name: e.target.value })}  />
      </label>
      <h3>感想文字</h3>
      <p className="muted">依次显示各段文字，片段时长平分。</p>
      {draft.pages.map((page, i) => <div className="thought-page" key={i}>
        <div className="thought-text-row">
          <label className="field full">第 {i + 1} 段 · {formatClipTime((i * seconds) / draft.pages.length)}–{formatClipTime(((i + 1) * seconds) / draft.pages.length)}
            <textarea rows={3} value={page.text} onChange={(e) => updateText(i, e.target.value)} placeholder="言って" />
          </label>
          {draft.pages.length > 1 && <button className="button quiet" onClick={() => change({ ...draft, pages: draft.pages.filter((_, at) => at !== i) })}>删除段落</button>}
        </div>
        <div className="thought-actions"><button className="button quiet" disabled={imageLoading || busy} onClick={() => chooseImage(i)}>{imageLoading ? '选择中…' : '添加图片'}</button></div>
        {page.images.map((image, imageIndex) => <div className="thought-image-row" key={imageIndex}>
          <span title={image.path}>{nameOf(image.path)}</span>
          <label>宽度 <input type="range" min="40" max="1200" value={image.width} onChange={(e) => editPage(i, (item) => { item.images[imageIndex].width = Number(e.target.value); })} /></label>
          <button className="button quiet" onClick={() => editPage(i, (item) => { item.images.splice(imageIndex, 1); })}>移除</button>
        </div>)}
      </div>)}
      <button className="button quiet" disabled={draft.pages.length >= 20} onClick={() => change({ ...draft, pages: [...draft.pages, { text: '', images: [] }] })}>添加一段</button>
      <h3>背景音乐</h3>
      <label className="field full">音频来源
        <select value={draft.audio.kind} disabled={optionsLoading || busy} onChange={(e) => {
          const kind = e.target.value;
          if (kind === 'silent') { setMedia(null); change({ ...draft, audio: { kind, start_ms: 0, end_ms: 0 } }); return; }
          const option = options.find((item) => item.kind === kind && item.available);
          if (option) { const endMs = Math.min(option.duration_ms, 30000); setMedia(option); setAudioStart('0:00'); setAudioEnd(formatClipTime(endMs / 1000)); setSilentDuration(formatClipTime(endMs / 1000)); change({ ...draft, duration_ms: endMs, audio: { kind, start_ms: 0, end_ms: endMs } }); }
          else { setMedia(null); change({ ...draft, audio: { kind, start_ms: 0, end_ms: 0 } }); }
        }}>
          <option value="silent">静音</option>
          {Array.from({ length: 7 }, (_, i) => <option key={i} value={`op${i + 1}`} disabled={!options.find((o) => o.kind === `op${i + 1}`)?.available}>OP{i + 1}{options.find((o) => o.kind === `op${i + 1}`)?.available ? '' : '（待提供）'}</option>)}
          <option value="custom">自定义音频</option>
        </select>
      </label>
      {optionsLoading && <p className="muted">正在读取BGM信息…</p>}
      {optionsError && <div className="notice warn">xBGM读取失败：{optionsError}</div>}
      {draft.audio.kind === 'custom' && <button className="button quiet" disabled={audioLoading || busy} onClick={chooseAudio}>{audioLoading ? '正在读取音频…' : '选择音频文件'}</button>}
      {media && draft.audio.kind !== 'silent' && <>
        <audio className="thought-audio" controls src={media.preview_url} />
        <div className="clip-range" aria-label="音频范围">
          <div className="clip-range-rail"><div className={`clip-range-selected ${reversedRange ? 'invalid' : ''}`} style={{ left: `${Math.max(0, railStart * 1000 / media.duration_ms) * 100}%`, width: `${Math.max(0, (railEnd - railStart) * 1000 / media.duration_ms) * 100}%` }} /></div>
          <input className="clip-range-start" type="range" min="0" max={media.duration_ms / 1000} step="0.1" value={draft.audio.start_ms / 1000} aria-label="开始时间" onChange={(e) => updateRange(Number(e.target.value), draft.audio.end_ms / 1000)} />
          <input className="clip-range-end" type="range" min="0" max={media.duration_ms / 1000} step="0.1" value={draft.audio.end_ms / 1000} aria-label="结束时间" onChange={(e) => updateRange(draft.audio.start_ms / 1000, Number(e.target.value))} />
        </div>
        <div className="clip-times">
          <label className="field">开始时间<input value={audioStart} onChange={(e) => setAudioStart(e.target.value)} onBlur={() => { const value = parseClipTime(audioStart); if (value !== null) updateRange(value, draft.audio.end_ms / 1000); }} /></label>
          <label className="field">结束时间<input value={audioEnd} onChange={(e) => setAudioEnd(e.target.value)} onBlur={() => { const value = parseClipTime(audioEnd); if (value !== null) updateRange(draft.audio.start_ms / 1000, value); }} /></label>
        </div>
      </>}
      {(draft.audio.kind === 'silent' || draft.audio.kind.startsWith('op')) && <label className="field full">片段时长
        <input value={silentDuration} onChange={(e) => setSilentDuration(e.target.value)} onBlur={() => { const value = parseClipTime(silentDuration); if (value !== null) change({ ...draft, duration_ms: Math.round(value * 1000) }); }} />
      </label>}
      <p className="muted">片段时长：{formatClipTime(seconds)}{draft.audio.kind.startsWith('op') ? '（音频循环播放）' : draft.audio.kind === 'custom' ? '（跟随音频截取范围）' : ''}</p>
      {formError && <div className="notice warn">{formError}</div>}
      {error && <div className="notice bad" role="alert">{error}</div>}
      <div className={`thought-save-status ${savedCurrent ? 'saved' : ''}`} role="status">
        {savedCurrent ? `已保存到当前作品：${saved!.name} · ${saved!.pages.length} 段 · ${formatClipTime(saved!.duration_ms / 1000)}`
          : saved ? '有未保存修改' : '尚未保存'}
      </div>
      <div className="thought-actions"><button className="button primary" disabled={!!formError || busy || audioLoading || imageLoading || optionsLoading || savedCurrent} onClick={() => onSave(currentThought())}>{savedCurrent ? '已保存' : saved ? '更新片段' : '保存片段'}</button>
        <button className="button quiet" disabled={!!formError || busy || audioLoading || imageLoading || optionsLoading} onClick={render}>{busy ? '正在生成预览…' : '生成预览'}</button></div>
    </section>
    <aside className="card thought-preview">
      <h2>画面预览</h2>
      <div className="thought-stage" ref={stageRef}>
        <iframe className="thought-frame" title="感想画面" src="/thought-template" ref={frameRef}
          style={{ transform: `scale(${stageWidth / 1920})` }} onLoad={() => setFrameReady(true)} />
        {draft.pages[activePage]?.images.map((image, index) => imageURLs[image.path] && <div className="thought-decoration" key={`${activePage}-${index}`} role="img" aria-label={`第 ${activePage + 1} 段第 ${index + 1} 张装饰图，可拖动`}
          style={{ left: `${image.x / 19.2}%`, top: `${image.y / 10.8}%`, width: `${image.width / 19.2}%`, aspectRatio: `1 / ${imageRatios[image.path] || 1}` }}
          onPointerDown={(e) => { dragRef.current = { page: activePage, image: index, x: image.x, y: image.y, clientX: e.clientX, clientY: e.clientY }; e.currentTarget.setPointerCapture(e.pointerId); }}
          onPointerMove={dragImage} onPointerUp={() => { dragRef.current = null; }} onPointerCancel={() => { dragRef.current = null; }} />)}
      </div>
      <label className="field full">查看时间：{formatClipTime(previewTime)} / {formatClipTime(seconds)}
        <input type="range" min="0" max={Math.max(seconds - 0.01, 0)} step="0.1" value={Math.min(previewTime, seconds)} onChange={(e) => setPreviewTime(Number(e.target.value))} />
      </label>
      {previewURL && <><h3>生成结果</h3><video className="thought-video" controls src={previewURL} /></>}
    </aside>
  </div>;
}
