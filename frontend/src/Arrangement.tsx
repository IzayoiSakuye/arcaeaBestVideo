import { useState, type DragEvent } from 'react';
import type { app, score } from '../wailsjs/go/models';

type Item = score.PlayItem;
type Props = { project: score.Project; rows: app.ReviewRow[]; onChange: (order: Item[]) => void };

export default function Arrangement({ project, rows, onChange }: Props) {
  const [dropAt, setDropAt] = useState<number | null>(null);
  const [dragged, setDragged] = useState<string | null>(null);
  const defaultOrder = () => project.songs.map((_, i) => ({ kind: 'song', song_index: project.songs.length - i }));
  const order: Item[] = project.play_order ?? defaultOrder();
  const usedSongs = new Set(order.filter((item) => item.kind === 'song').map((item) => item.song_index));
  const usedThoughts = new Set(order.filter((item) => item.kind === 'thought').map((item) => item.thought_id));
  const title = (item: Item) => item.kind === 'song'
    ? `#${item.song_index} ${rows[(item.song_index || 1) - 1]?.title || project.songs[(item.song_index || 1) - 1]?.song_id || '待校对曲目'}`
    : project.thoughts?.find((thought) => thought.id === item.thought_id)?.name || '感想片段';
  function insert(raw: string, at: number) {
    try {
      const source = JSON.parse(raw) as { from?: number; item?: Item };
      if (typeof source.from === 'number') {
        if (source.from < 0 || source.from >= order.length) return;
        const next = [...order];
        const [item] = next.splice(source.from, 1);
        next.splice(source.from < at ? at - 1 : at, 0, item);
        onChange(next);
      } else if (source.item) {
        if (source.item.kind === 'song' && usedSongs.has(source.item.song_index)) return;
        if (source.item.kind === 'thought' && usedThoughts.has(source.item.thought_id)) return;
        const next = [...order];
        next.splice(at, 0, source.item);
        onChange(next);
      }
    } catch { /* 其他拖放内容不属于编排项目。 */ }
  }
  function drag(event: DragEvent, value: object, label: string) {
    event.dataTransfer.setData('application/x-play-item', JSON.stringify(value));
    event.dataTransfer.effectAllowed = 'move';
    setDragged(label);
    const card = document.createElement('div');
    card.className = 'arrange-drag-card';
    card.textContent = label;
    document.body.appendChild(card);
    event.dataTransfer.setDragImage(card, 20, 20);
    window.setTimeout(() => card.remove(), 0);
  }
  const finishDrag = () => { setDragged(null); setDropAt(null); };
  const drop = (event: DragEvent, at: number) => { event.preventDefault(); event.stopPropagation(); insert(event.dataTransfer.getData('application/x-play-item'), at); finishDrag(); };
  function allow(event: DragEvent, at: number) {
    if (!event.dataTransfer.types.includes('application/x-play-item')) return;
    event.preventDefault();
    event.stopPropagation();
    event.dataTransfer.dropEffect = 'move';
    setDropAt(at);
  }
  function moveTo(index: number, value: string) {
    const position = Number(value);
    if (!Number.isInteger(position) || position < 1 || position > order.length || position === index + 1) return;
    const next = [...order];
    const [item] = next.splice(index, 1);
    next.splice(position - 1, 0, item);
    onChange(next);
  }
  const remove = (index: number) => onChange(order.filter((_, i) => i !== index));
  return <section className="arrangement">
    <div className="card arrange-main" onDragOver={(event) => allow(event, order.length)} onDrop={(event) => drop(event, order.length)}><div className="arrange-heading"><h2>最终播放顺序 <span>{order.length} 段</span></h2><button className="button quiet" onClick={() => onChange(defaultOrder())}>重置顺序</button></div><p className="muted">拖动或输入序号调整顺序。</p>
      {order.map((item, index) => <div className={`arrange-row${dropAt === index ? ' arrange-drop-before' : ''}${dropAt === index + 1 ? ' arrange-drop-after' : ''}`} key={`${item.kind}-${item.song_index || item.thought_id}-${index}`} draggable onDragStart={(event) => drag(event, { from: index }, title(item))} onDragEnd={finishDrag} onDragOver={(event) => { const after = event.clientY > event.currentTarget.getBoundingClientRect().top + event.currentTarget.clientHeight / 2; allow(event, index + Number(after)); }} onDrop={(event) => { const after = event.clientY > event.currentTarget.getBoundingClientRect().top + event.currentTarget.clientHeight / 2; drop(event, index + Number(after)); }}>
        <input className="arrange-number" type="number" min="1" max={order.length} defaultValue={index + 1} aria-label={`${title(item)}的播放序号`} onBlur={(event) => { moveTo(index, event.currentTarget.value); event.currentTarget.value = String(index + 1); }} onKeyDown={(event) => { if (event.key === 'Enter') event.currentTarget.blur(); }} />
        <span className="arrange-handle" aria-hidden="true">⋮⋮</span><span className="arrange-title">{title(item)}</span>
        <button className="button quiet" aria-label={`上移 ${title(item)}`} disabled={index === 0} onClick={() => { const next = [...order]; [next[index-1], next[index]] = [next[index], next[index-1]]; onChange(next); }}>↑</button>
        <button className="button quiet" aria-label={`下移 ${title(item)}`} disabled={index === order.length-1} onClick={() => { const next = [...order]; [next[index+1], next[index]] = [next[index], next[index+1]]; onChange(next); }}>↓</button>
        <button className="button quiet" onClick={() => remove(index)}>移除</button>
      </div>)}
      {dragged && <div className="arrange-drop-tail" onDragOver={(event) => allow(event, order.length)} onDrop={(event) => drop(event, order.length)} />}
    </div>
    <aside className="card arrange-side"><h2>可加入的片段</h2>
      <h3>感想</h3>{project.thoughts?.filter((thought) => !usedThoughts.has(thought.id)).map((thought) => <div className="arrange-source" key={thought.id} draggable onDragStart={(event) => drag(event, { item: { kind: 'thought', thought_id: thought.id } }, thought.name)} onDragEnd={finishDrag}><span>{thought.name}</span><button className="button quiet" onClick={() => onChange([...order, { kind: 'thought', thought_id: thought.id }])}>加入末尾</button></div>)}
      <h3>歌曲</h3>{project.songs.map((_, i) => i + 1).filter((index) => !usedSongs.has(index)).map((index) => <div className="arrange-source" key={index} draggable onDragStart={(event) => drag(event, { item: { kind: 'song', song_index: index } }, title({ kind: 'song', song_index: index }))} onDragEnd={finishDrag}><span>{title({ kind: 'song', song_index: index })}</span><button className="button quiet" onClick={() => onChange([...order, { kind: 'song', song_index: index }])}>加入末尾</button></div>)}
    </aside>
  </section>;
}
