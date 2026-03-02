// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from 'vitest';
import { initAmbientAudio } from './ambientAudio.js';

function makeEngine() {
  return { startAmbient: vi.fn() };
}

function listenerTypes(spy) {
  return spy.mock.calls.map(c => c[0]);
}

describe('initAmbientAudio', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('listener registration', () => {
    it('registers click and keydown listeners on document', () => {
      const addSpy = vi.spyOn(document, 'addEventListener');
      initAmbientAudio(makeEngine());

      expect(listenerTypes(addSpy)).toContain('click');
      expect(listenerTypes(addSpy)).toContain('keydown');
    });

    it('removes both listeners after first interaction', () => {
      const removeSpy = vi.spyOn(document, 'removeEventListener');
      initAmbientAudio(makeEngine());

      document.dispatchEvent(new Event('click'));

      expect(listenerTypes(removeSpy)).toContain('click');
      expect(listenerTypes(removeSpy)).toContain('keydown');
    });
  });

  describe('one-shot behavior', () => {
    it('does not call engine.startAmbient before user interaction', () => {
      const engine = makeEngine();
      initAmbientAudio(engine);

      expect(engine.startAmbient).not.toHaveBeenCalled();
    });

    it('calls engine.startAmbient on first click', () => {
      const engine = makeEngine();
      initAmbientAudio(engine);

      document.dispatchEvent(new Event('click'));

      expect(engine.startAmbient).toHaveBeenCalledTimes(1);
    });

    it('calls engine.startAmbient on first keydown', () => {
      const engine = makeEngine();
      initAmbientAudio(engine);

      document.dispatchEvent(new Event('keydown'));

      expect(engine.startAmbient).toHaveBeenCalledTimes(1);
    });

    it('only starts ambient once across multiple interactions', () => {
      const engine = makeEngine();
      initAmbientAudio(engine);

      document.dispatchEvent(new Event('click'));
      document.dispatchEvent(new Event('keydown'));
      document.dispatchEvent(new Event('click'));

      expect(engine.startAmbient).toHaveBeenCalledTimes(1);
    });
  });

  describe('tryStart API', () => {
    it('returns an object with tryStart method', () => {
      const result = initAmbientAudio(makeEngine());

      expect(typeof result.tryStart).toBe('function');
    });

    it('allows programmatic start via returned tryStart', () => {
      const engine = makeEngine();
      const { tryStart } = initAmbientAudio(engine);

      tryStart();

      expect(engine.startAmbient).toHaveBeenCalledTimes(1);
    });

    it('programmatic tryStart is also idempotent', () => {
      const engine = makeEngine();
      const { tryStart } = initAmbientAudio(engine);

      tryStart();
      tryStart();

      expect(engine.startAmbient).toHaveBeenCalledTimes(1);
    });

    it('click after programmatic tryStart does not start again', () => {
      const engine = makeEngine();
      const { tryStart } = initAmbientAudio(engine);

      tryStart();
      document.dispatchEvent(new Event('click'));

      expect(engine.startAmbient).toHaveBeenCalledTimes(1);
    });
  });

  describe('independent instances', () => {
    it('separate initAmbientAudio calls are independent', () => {
      const engine1 = makeEngine();
      const engine2 = makeEngine();

      initAmbientAudio(engine1);
      initAmbientAudio(engine2);

      document.dispatchEvent(new Event('click'));

      expect(engine1.startAmbient).toHaveBeenCalledTimes(1);
      expect(engine2.startAmbient).toHaveBeenCalledTimes(1);
    });
  });
});
