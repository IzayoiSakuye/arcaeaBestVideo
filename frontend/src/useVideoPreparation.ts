import { useEffect, useRef, useState } from 'react';
import { CancelVideoPreparation, ConfirmVideoRange, PrepareVideo, RetryVideoPreparation, VideoInfos } from '../wailsjs/go/app/App';
import { EventsOn } from '../wailsjs/runtime';
import type { app, score } from '../wailsjs/go/models';
import { songKey, videoSourceKey, mergeVideoSnapshot } from './video';

export function useVideoPreparation(project: score.Project | null, identity: string, epoch: number) {
  const key = `${epoch}:${identity}:${videoSourceKey(project?.songs || [])}:${project?.songs.map((song) => song.clip?.confirmed_key || '').join(',')}`;
  const session = useRef({ key, states: [] as app.VideoState[], pending: new Map<string, app.VideoState[]>(), alive: true });
  if (session.current.key !== key) session.current = { key, states: [], pending: new Map(), alive: true };
  const [, update] = useState(0);
  const [error, setError] = useState('');
  function publish(current: typeof session.current) {
    if (current === session.current && current.alive) update((value) => value + 1);
  }
  useEffect(() => {
    const current = session.current;
    current.alive = true;
    setError('');
    const queryEvents: app.VideoState[] = [];
    let querying = true;
    const off = EventsOn('video:event', (event: app.VideoState) => {
      if (current !== session.current || !current.alive) return;
      if (querying) queryEvents.push(event);
      const id = songKey(event);
      const buffer = current.pending.get(id);
      if (buffer) buffer.push(event);
      const index = current.states.findIndex((state) => songKey(state) === id && state.request_id === event.request_id && state.url === event.url);
      if (index < 0) return;
      current.states[index] = { ...event, index: current.states[index].index };
      publish(current);
    });
    if (project) VideoInfos(project).then((states) => {
      if (current !== session.current || !current.alive) return;
      querying = false;
      // 查询期间同任务的最新事件包含完整状态，只沿用查询中的歌曲索引。
      const snapshot = states.map((state) => ({ ...(queryEvents.filter((event) => event.request_id === state.request_id && songKey(event) === songKey(state) && event.url === state.url).at(-1) || state), index: state.index }));
      current.states = mergeVideoSnapshot(snapshot, current.states);
      publish(current);
    }).catch((reason) => { if (current === session.current && current.alive) setError(String(reason)); });
    return () => { current.alive = false; off(); };
  }, [key]);

  async function run<T extends app.VideoState | app.VideoSelection>(song: score.Song, call: () => Promise<T>, accept: (result: T) => boolean = () => true): Promise<T | null> {
    const current = session.current;
    const id = songKey({ song_id: song.song_id, difficulty: song.difficulty.name });
    if (current.pending.has(id)) return null;
    const events: app.VideoState[] = [];
    current.pending.set(id, events);
    setError('');
    publish(current);
    try {
      const result = await call();
      if (current !== session.current || !current.alive) return null;
      if (!accept(result)) {
        current.states = current.states.map((state) => songKey(state) === id ? { ...state, confirmed: false, material_ready: false } : state);
        setError('编辑已改变，请重新保存当前片段');
        return null;
      }
      const value: app.VideoState | app.VideoSelection = result;
      const state: app.VideoState = 'state' in value ? value.state : value;
      const latest = events.filter((event) => event.request_id === state.request_id && event.url === state.url).at(-1) || state;
      const index = project?.songs.findIndex((item) => item.song_id === song.song_id && item.difficulty.name === song.difficulty.name) ?? -1;
      if (index >= 0) {
        current.states = current.states.filter((item) => songKey(item) !== id);
        current.states.push({ ...latest, index: index + 1 });
      }
      return result;
    } catch (reason) {
      if (current === session.current && current.alive) {
        setError(String(reason));
        const failed = events.filter((event) => songKey(event) === id && (!song.video?.url || event.url === song.video.url) && ['failed', 'cancelled'].includes(event.status)).at(-1);
        if (failed) {
          const index = project?.songs.findIndex((item) => item.song_id === song.song_id && item.difficulty.name === song.difficulty.name) ?? -1;
          current.states = current.states.filter((item) => songKey(item) !== id);
          if (index >= 0) current.states.push({ ...failed, index: index + 1 });
        }
      }
      return null;
    } finally {
      current.pending.delete(id);
      publish(current);
    }
  }
  return {
    states: [...session.current.states].sort((a, b) => a.index - b.index), error,
    busy: session.current.pending.size > 0,
    prepare: (song: score.Song) => run(song, () => PrepareVideo(song)),
    confirm: (song: score.Song, clip: score.ClipRange, accept: () => boolean) => run(song, () => ConfirmVideoRange(song, clip), accept),
    retry: (song: score.Song, state: app.VideoState) => run(song, () => RetryVideoPreparation(state.song_id, state.difficulty, state.request_id)),
    cancel: async (state: app.VideoState) => {
      const current = session.current;
      try { await CancelVideoPreparation(state.song_id, state.difficulty, state.request_id); }
      catch (reason) { if (current === session.current && current.alive) setError(String(reason)); }
    },
  };
}
