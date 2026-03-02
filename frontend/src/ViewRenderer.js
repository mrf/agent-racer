import { RaceCanvas } from './canvas/RaceCanvas.js';

const VIEW_TYPES = {};

export function registerView(type, factory) {
  VIEW_TYPES[type] = factory;
}

export function createView(type, canvas, engine) {
  const factory = VIEW_TYPES[type];
  if (!factory) throw new Error('Unknown view type: ' + type);
  return factory(canvas, engine);
}

export function getViewTypes() {
  return Object.keys(VIEW_TYPES);
}

// Register built-in views
registerView('race', (canvas, engine) => {
  const view = new RaceCanvas(canvas);
  if (engine) view.setEngine(engine);
  return view;
});
