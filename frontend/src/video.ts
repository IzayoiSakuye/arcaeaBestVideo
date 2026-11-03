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
