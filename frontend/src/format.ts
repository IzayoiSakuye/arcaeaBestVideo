export type Event = {
  stage: string;
  song_index: number;
  song_id: string;
  status: string;
  progress: number;
  message: string;
  error?: string;
};

export function nameOf(path: string) { return path.split(/[\\/]/).pop() || path; }
export function scoreText(achievement: number) { return String(Math.round(achievement * 100000)); }

export function eventText(event: Event) {
  if (event.error) return event.error;
  if (event.message) return event.message;
  if (event.stage === 'concat') return '正在拼接视频';
  if (event.stage === 'done') return '视频已生成';
  if (event.status === 'running') return `正在处理第 ${event.song_index} 首 · ${event.song_id}`;
  return `第 ${event.song_index} 首 ${event.status === 'skipped' ? '使用缓存' : '已完成'} · ${event.song_id}`;
}
