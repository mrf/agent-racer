export function initAmbientAudio(engine) {
  let started = false;

  function tryStart() {
    if (started) return;
    started = true;
    document.removeEventListener('click', tryStart);
    document.removeEventListener('keydown', tryStart);
    engine.startAmbient();
  }

  document.addEventListener('click', tryStart);
  document.addEventListener('keydown', tryStart);

  return { tryStart };
}
