export function videoQueues<T extends { status: string; url: string }>(states: T[]) {
  const pending = states.filter((state) => state.status !== 'ready');
  const retryable = pending.filter(
    (state) => (state.status === 'download' || state.status === 'legacy') && state.url !== '',
  );
  return { pending, retryable, manualCount: pending.length - retryable.length };
}

export function videoSourceKey(songs: { song_id: string; difficulty: { name: string }; video?: { url?: string; local_path?: string; owner_name?: string } }[]) {
  return JSON.stringify(songs.map((song) => [song.song_id, song.difficulty.name, song.video?.url, song.video?.local_path, song.video?.owner_name]));
}

export function songKey(state: { song_id: string; difficulty: string }) {
  return `${state.song_id}\u0000${state.difficulty}`;
}

export function mergeVideoSnapshot<T extends { song_id: string; difficulty: string; request_id: string }>(snapshot: T[], live: T[]): T[] {
  return snapshot.map((state) => live.find((item) => songKey(item) === songKey(state) && (item.request_id === state.request_id || (item.request_id.length === state.request_id.length ? item.request_id > state.request_id : item.request_id.length > state.request_id.length))) || state);
}

export function usedSongIndexes(project: { songs: unknown[]; play_order?: { kind: string; song_index?: number }[] }) {
  return project.play_order === undefined ? project.songs.map((_, i) => i + 1)
    : project.play_order.filter((item) => item.kind === 'song').map((item) => item.song_index || 0);
}

export function clipSaved(clip: { start_ms: number; duration_ms: number; confirmed_key?: string } | undefined, start: number | null, end: number | null) {
  return Boolean(clip?.confirmed_key && start !== null && end !== null && clip.start_ms === Math.round(start * 1000) && clip.duration_ms === Math.round((end - start) * 1000));
}

export const videoStatusText: Record<string, string> = {
  preparing: '准备预览中', queued: '排队中', downloading: '下载素材中', preview: '可预览', ready: '素材就绪',
  failed: '失败', cancelled: '已取消', missing: '缺少来源', invalid: '来源无效', download: '待准备', legacy: '旧缓存，须重新确认',
};
