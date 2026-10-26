export function formatClipTime(seconds: number): string {
  const milliseconds = Math.round(seconds * 1000);
  const minutes = Math.floor(milliseconds / 60000);
  const wholeSeconds = Math.floor((milliseconds % 60000) / 1000);
  const fraction = milliseconds % 1000;
  return `${minutes}:${String(wholeSeconds).padStart(2, '0')}${fraction ? `.${String(fraction).padStart(3, '0').replace(/0+$/, '')}` : ''}`;
}

export function parseClipTime(value: string): number | null {
  const match = /^(\d+):([0-5]?\d)(?:\.(\d{1,3}))?$/.exec(value.trim());
  if (!match) return null;
  const milliseconds = Number(match[1]) * 60000 + Number(match[2]) * 1000 + Number((match[3] || '').padEnd(3, '0'));
  return Number.isSafeInteger(milliseconds) ? milliseconds / 1000 : null;
}

export function clipRangeError(start: number | null, end: number | null, videoLength: number): string {
  if (start === null || end === null) return '请输入分:秒格式的时间，例如 1:23';
  if (start > end) return '开始时间不能晚于结束时间';
  if (start === end) return '片段时长不能为 0';
  if (start < 0 || end > videoLength + 0.05) return '片段超出录像时长';
  if (end - start < 1) return '片段时长不能少于 1 秒';
  return '';
}
