import { useEffect, useRef, useState, type RefObject } from 'react';
import { ReleasePreview } from '../wailsjs/go/app/App';
import { connectMedia } from './media';

const users = new Map<string, number>();
export function retainPreview(url: string) {
  users.set(url, (users.get(url) || 0) + 1);
  return () => {
    users.set(url, Math.max(0, (users.get(url) || 0) - 1));
    queueMicrotask(() => {
      if (users.get(url) === 0) {
        users.delete(url);
        void ReleasePreview(url).catch(() => {});
      }
    });
  };
}

export function PreviewMedia({ src, kind, className }: { src: string; kind: 'video' | 'audio'; className: string }) {
  const ref = useRef<HTMLVideoElement & HTMLAudioElement>(null);
  useEffect(() => {
    const media = ref.current;
    media?.setAttribute('src', src);
    const release = retainPreview(src);
    return () => {
      media?.pause();
      media?.removeAttribute('src');
      media?.load();
      release();
    };
  }, [src]);
  return kind === 'video' ? <video key={src} ref={ref} className={className} controls src={src} />
    : <audio key={src} ref={ref} className={className} controls src={src} />;
}

type Props = {
  src: string; audioSrc?: string; start: number | null; end: number | null; invalid: boolean;
  videoRef: RefObject<HTMLVideoElement | null>; scrubbing: RefObject<number | null>;
  onTime: (seconds: number) => void; onPlaying: (playing: boolean) => void; onError: (error: string) => void;
};

export default function VideoPreview({ src, audioSrc, start, end, invalid, videoRef, scrubbing, onTime, onPlaying, onError }: Props) {
  const audioRef = useRef<HTMLAudioElement>(null);
  const read = () => ({ start, end, invalid, onTime, onPlaying, onError, scrubbing: scrubbing.current !== null });
  const waiting = useMediaPair(videoRef, audioRef, src, audioSrc, read);
  return <>
    <video ref={videoRef} preload="metadata" src={src} />
    {audioSrc && <audio ref={audioRef} preload="metadata" src={audioSrc} />}
    {waiting && <p role="status">正在缓冲音视频…</p>}
  </>;
}

function useMediaPair(videoRef: Props['videoRef'], audioRef: RefObject<HTMLAudioElement | null>, src: string, audioSrc: string | undefined,
  read: () => Pick<Props, 'start' | 'end' | 'invalid' | 'onTime' | 'onPlaying' | 'onError'> & { scrubbing: boolean }) {
  const [waiting, setWaiting] = useState(false);
  const latestRead = useRef(read);
  latestRead.current = read;
  useEffect(() => {
    const video = videoRef.current;
    const audio = audioRef.current;
    if (!video) return;
    video.setAttribute('src', src);
    if (audio && audioSrc) audio.setAttribute('src', audioSrc);
    setWaiting(false);
    const releases = [retainPreview(src), ...(audioSrc ? [retainPreview(audioSrc)] : [])];
    const disconnect = connectMedia(video, audioSrc ? audio : null, () => latestRead.current(), setWaiting);
    return () => {
      disconnect();
      for (const media of [video, ...(audioSrc && audio ? [audio] : [])]) { media.removeAttribute('src'); media.load(); }
      releases.forEach((release) => release());
    };
  }, [src, audioSrc]);
  return waiting;
}
