// 仅用于诊断：真实解码，显式注入暂停时的残差；不代表原偶发故障复现。
export async function diagnosePlayback(connectMedia, edge, modes) {
  const precision = edge === 'precision';
  const diagnostics = [];
  const delay = (ms) => new Promise((done) => setTimeout(done, ms));
  const until = async (check) => {
    for (let i = 0; i < 300; i++) { if (check()) return; await delay(20); }
    throw Error('诊断等待超时');
  };
  const passed = [];
  for (const mode of modes) {
    const video = document.createElement('video');
    const audio = document.createElement('audio');
    document.body.append(video, audio);
    let scenario = 'observed-repeated', target = null, error = '';
    const times = [];
    const state = (media) => ({ time: media.currentTime, paused: media.paused, seeking: media.seeking, readyState: media.readyState, ended: media.ended });
    const record = (event, extra = {}) => diagnostics.push({ mode, scenario, event, ms: performance.now(), target,
      video: state(video), audio: state(audio), error, ...extra });
    const controls = { start: 1, end: 2, invalid: false, scrubbing: false,
      onTime(value) { times.push(value); }, onPlaying(value) { record('onPlaying', { value }); }, onError(value) { error = value; } };
    const listeners = [];
    for (const [name, media] of [['video', video], ['audio', audio]]) {
      for (const event of ['play', 'playing', 'pause', 'seeking', 'seeked', 'timeupdate', 'waiting', 'canplay', 'ended']) {
        media.addEventListener(event, () => record(`${name}:${event}:before`));
        listeners.push([name, media, event]);
      }
    }
    const stop = connectMedia(video, audio, () => controls, (waiting) => record('onWaiting', { waiting }));
    // 原生事件可在监听器间运行微任务，用注册顺序观察生产处理前后。
    for (const [name, media, event] of listeners) media.addEventListener(event, () => record(`${name}:${event}:after`));
    try {
      video.src = `/${mode}/video.mp4`; audio.src = `/${mode}/audio.wav`;
      await until(() => video.readyState >= 3 && audio.readyState >= 3);
      if (!precision) {
      await video.play(); await until(() => video.currentTime > 1.2);
      record('before-pause'); video.pause(); record('after-pause-call');
      // 保留原 repeated 的目标、120ms拖动等待和非seeking等待，只有额外观测。
      for (target of [1, 1.9, 1.1, 2, 1.4, 1.8]) {
        controls.scrubbing = true; const count = times.length;
        video.dispatchEvent(new Event('timeupdate')); await delay(120);
        record('before-target', { syntheticTimeupdate: true, staleProgressSuppressed: times.length === count });
        controls.scrubbing = false; video.currentTime = target; record('after-target-set');
        await until(() => !video.seeking && !audio.seeking);
        const within40ms = Math.abs(video.currentTime - target) < 0.04 && Math.abs(audio.currentTime - target) < 0.04;
        record('original-wait-returned', { within40ms });
        if (!within40ms) break;
      }
      }
      // 每个case先在scrubbing中建立前置位置，恢复后由真实seek事件进入未修改的生产逻辑。
      const cases = precision ? [
        ...[0.072, -0.072, 0.041, -0.041, 0.079, -0.079, 0.081, -0.081].map((offset) => ({ offset, pause: 'already-paused' })),
        { offset: 0.072, pause: 'queued-before-setup' },
        { offset: -0.072, pause: 'settled-before-setup' },
        { offset: 0.072, pause: 'same-position', same: true },
        { offset: -0.072, pause: 'newest-target', sequence: [1.4, 1.6, 1.2] },
        { offset: 0.072, pause: 'buffer-overlap', waiting: true },
        { offset: 0.072, pause: 'end-point', target: 2 },
        { offset: -0.072, pause: 'start-point', target: 1 },
      ] : [
        ...[0.02, 0.039, 0.041, 0.072, 0.079, 0.081].map((offset) => ({ offset, pause: 'already-paused' })),
        { offset: 0.072, pause: 'queued-before-setup' },
        { offset: 0.072, pause: 'settled-before-setup' },
      ];
      for (const spec of cases) {
        scenario = `injected-${spec.offset}-${spec.pause}`;
        if (['queued-before-setup', 'settled-before-setup'].includes(spec.pause)) {
          controls.scrubbing = false;
          await video.play(); await until(() => !audio.paused && video.currentTime > 1.2);
          record('before-pause'); video.pause(); record('after-pause-call');
          if (spec.pause === 'settled-before-setup') await until(() => video.paused && audio.paused);
        }
        controls.scrubbing = true;
        video.pause(); await until(() => video.paused && audio.paused && !video.seeking && !audio.seeking);
        target = spec.target ?? (precision ? 1.4 : 1);
        video.currentTime = spec.same ? target : 1.8; await until(() => !video.seeking && !audio.seeking);
        audio.currentTime = target + spec.offset; await until(() => !audio.seeking);
        record('injected-precondition', { injected: true, requestedOffset: spec.offset });
        if (Math.abs(audio.currentTime - target - spec.offset) > 0.002) throw Error('未能建立注入前置状态');
        const progressCount = times.length;
        video.dispatchEvent(new Event('timeupdate'));
        if (precision && times.length !== progressCount) throw Error('拖动时发布了旧进度');
        const seekCount = () => diagnostics.filter((event) => event.scenario === scenario && event.event === 'audio:seeking:before').length;
        const seeksBefore = seekCount();
        if (spec.waiting) audio.dispatchEvent(new Event('waiting'));
        controls.scrubbing = false;
        for (const value of spec.sequence || [target]) { target = value; video.currentTime = target; }
        record('after-target-set');
        await until(() => !video.seeking && !audio.seeking);
        record('original-wait-returned', { injected: true, within40ms: Math.abs(audio.currentTime - target) < 0.04 });
        if (precision && (Math.abs(video.currentTime - target) >= 0.04 || Math.abs(audio.currentTime - target) >= 0.04)) throw Error(`精度回归失败：${scenario}，video=${video.currentTime}，audio=${audio.currentTime}，target=${target}`);
        const seeksAtCompletion = seekCount();
        if (spec.waiting) audio.dispatchEvent(new Event('canplay'));
        await delay(220);
        record('after-extra-220ms', { injected: true, within40ms: Math.abs(audio.currentTime - target) < 0.04 });
        if (precision) {
          const writes = seeksAtCompletion - seeksBefore;
          record('precision-assertion', { injected: true, audioSeeks: writes, latestTarget: target });
          if (writes > (spec.sequence?.length || 1) || seekCount() !== seeksAtCompletion) throw Error('音频seek回环');
          if (Math.abs(video.currentTime - target) >= 0.04 || Math.abs(audio.currentTime - target) >= 0.04) throw Error('旧事件覆盖了定位目标');
        }
        if (!video.paused || !audio.paused || error) throw Error('注入场景不满足暂停/无错误前提');
      }
      passed.push(mode);
    } catch (failure) {
      record('diagnostic-error');
      return { passed, diagnostics, error: failure.stack };
    } finally {
      stop();
      for (const media of [video, audio]) { media.removeAttribute('src'); media.load(); media.remove(); }
    }
  }
  return { passed, diagnostics };
}
