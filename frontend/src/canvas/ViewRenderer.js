import { RaceCanvas } from './RaceCanvas.js';
import { FootraceCanvas } from './FootraceCanvas.js';

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

function viewFactory(ViewClass) {
  return function (canvas, engine) {
    const view = new ViewClass(canvas);
    if (engine) view.setEngine(engine);
    return view;
  };
}

// Built-in views
registerView('race', viewFactory(RaceCanvas));
registerView('footrace', viewFactory(FootraceCanvas));
