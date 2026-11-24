export type MediaControls = {
  start: number | null; end: number | null; invalid: boolean; muted: boolean; volume: number;
  onTime: (seconds: number) => void; onPlaying: (playing: boolean) => void; onError: (message: string) => void;
};

export function connectMedia(video: HTMLMediaElement, audio: HTMLMediaElement | null, read: () => MediaControls, onWaiting: (waiting: boolean) => void) {
  let alive = true;
  let wanted = false;
  let buffering = false;
  let starting = false;
  let internalPauses = 0;
  const pause = () => { if (!video.paused) { internalPauses++; video.pause(); } audio?.pause(); read().onPlaying(false); };
  const volume = () => { video.muted = Boolean(audio) || read().muted; video.volume = read().volume; if (audio) { audio.volume = read().volume; audio.muted = read().muted; } };
  const position = () => {
    const { start, end, invalid } = read();
    if (start === null || end === null || invalid) { wanted = false; pause(); return; }
    if (video.currentTime < start || video.currentTime >= end - 0.03) video.currentTime = start;
    if (audio && Math.abs(audio.currentTime - video.currentTime) > 0.15) audio.currentTime = video.currentTime;
    read().onTime(Math.max(0, video.currentTime - start));
  };
  const fail = () => { wanted = false; buffering = false; onWaiting(false); pause(); read().onError('预览播放失败，请重新准备录像或检查网络'); };
  const resume = async () => {
    if (!alive || !wanted || starting || video.seeking || (audio?.seeking)) return;
    if (video.readyState < 3 || (audio && audio.readyState < 3)) { buffering = true; onWaiting(true); pause(); return; }
    starting = true;
    buffering = false;
    onWaiting(false);
    position();
    volume();
    try {
      await Promise.all([video.play(), ...(audio ? [audio.play()] : [])]);
      if (!alive || !wanted) pause();
    } catch { if (alive && wanted && !buffering) fail(); }
    finally { starting = false; }
  };
  const play = () => { wanted = true; position(); volume(); void resume(); };
  const paused = () => { if (internalPauses > 0) { internalPauses--; return; } wanted = false; buffering = false; onWaiting(false); audio?.pause(); read().onPlaying(false); };
  const wait = () => { if (!wanted) return; buffering = true; onWaiting(true); pause(); };
  const seek = () => { if (audio && Math.abs(audio.currentTime - video.currentTime) > 0.08) audio.currentTime = video.currentTime; wait(); };
  const playing = () => { read().onPlaying(true); if (audio?.paused && wanted) void resume(); };
  const ended = () => { if (!wanted) return; buffering = true; position(); void resume(); };
  const listeners: [HTMLMediaElement, string, EventListener][] = [];
  const on = (node: HTMLMediaElement, name: string, fn: () => void) => { node.addEventListener(name, fn); listeners.push([node, name, fn]); };
  on(video, 'play', play); on(video, 'pause', paused); on(video, 'playing', playing);
  on(video, 'seeking', seek); on(video, 'timeupdate', position); on(video, 'ended', ended);
  for (const media of [video, ...(audio ? [audio] : [])]) {
    on(media, 'waiting', wait); on(media, 'stalled', wait); on(media, 'canplay', () => { void resume(); });
    on(media, 'seeked', () => { void resume(); }); on(media, 'error', fail);
  }
  // 不依赖 timeupdate 的低频率来限制音画漂移。
  const timer = globalThis.setInterval(() => { if (wanted && !buffering) position(); }, 100);
  return () => {
    alive = false; wanted = false; globalThis.clearInterval(timer);
    listeners.forEach(([node, name, fn]) => node.removeEventListener(name, fn));
    pause();

  };
}
