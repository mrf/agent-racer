import { BaseCanvas } from './BaseCanvas.js';
import { FootraceTrack } from './FootraceTrack.js';
import { Dashboard } from './Dashboard.js';
import { WeatherSystem } from './Weather.js';
import { Character } from '../entities/Character.js';
import { syncEngineForEntity } from '../audio/engineSync.js';

// Character hit area — smaller than car, centered on character body.
const HIT_HX = 20;
const HIT_HY = 25;

function isInsideHitbox(dx, dy) {
  return dx >= -HIT_HX && dx <= HIT_HX && dy >= -HIT_HY && dy <= HIT_HY;
}

// Hamster bounding box
const HAMSTER_HIT_HX = 10;
const HAMSTER_HIT_HY = 7.5;

function isInsideHamsterHitbox(dx, dy) {
  return dx >= -HAMSTER_HIT_HX && dx <= HAMSTER_HIT_HX && dy >= -HAMSTER_HIT_HY && dy <= HAMSTER_HIT_HY;
}

export class FootraceCanvas extends BaseCanvas {
  constructor(canvas, engine = null) {
    super(canvas, {
      engine,
      track: new FootraceTrack(),
      dashboard: new Dashboard(),
      weather: new WeatherSystem(),
      entityMapProperty: 'characters',
      createEntity: (state) => new Character(state),
      entityHitType: 'character',
      backgroundColor: '#1a2e1a',
      emptyStateColor: '#8a8',
      emptyStateText: 'No active sessions detected',
      emptyStateSubtext: 'Start a session to see them run',
      isInsideEntityHitbox: isInsideHitbox,
      isInsideHamsterHitbox,
    });
  }

  onComplete(sessionId) {
    const ch = this.characters.get(sessionId);
    if (ch) {
      ch.confettiEmitted = false;
    }
    this.flashAlpha = 0.3;
  }

  onError(sessionId) {
    const ch = this.characters.get(sessionId);
    if (ch) {
      ch.stumbleEmitted = false;
      ch.starsEmitted = false;
      ch.errorTimer = 0;
      ch.errorStage = 0;
    }
    this.shakeIntensity = 6;
    this.shakeDuration = 0.3;
    this.shakeTimer = 0;
  }

  update() {
    const dt = this.dt;
    const zoneLayout = this._buildZoneLayout();
    this._positionTrackEntities(zoneLayout, (character) => {
      syncEngineForEntity(this.engine, character.id, character.state, 'track');
    });
    this._positionPitEntities(zoneLayout, (character) => {
      syncEngineForEntity(this.engine, character.id, character.state, 'pit');
    });
    this._positionParkingLotEntities(zoneLayout, (character) => {
      syncEngineForEntity(this.engine, character.id, character.state, 'parkingLot');
    });
    this._updateSharedFrameState(dt);
  }

  drawEntityBloom(gc, character, x, y) {
    if (character.glowIntensity > 0.02) {
      this._drawGlow(gc, x, y, 25, undefined, character.glowIntensity * 2);
    }
  }
}
