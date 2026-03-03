export const TILE_TYPES = [
  { id: 'straight-h',  label: '\u2501\u2501', desc: 'Straight H' },
  { id: 'straight-v',  label: '\u2503',  desc: 'Straight V' },
  { id: 'curve-ne',    label: '\u2570',  desc: 'Curve NE'   },
  { id: 'curve-nw',    label: '\u256f',  desc: 'Curve NW'   },
  { id: 'curve-se',    label: '\u256d',  desc: 'Curve SE'   },
  { id: 'curve-sw',    label: '\u256e',  desc: 'Curve SW'   },
  { id: 'chicane',     label: '~',  desc: 'Chicane'     },
  { id: 'pit-entry',   label: 'P\u2193', desc: 'Pit Entry'  },
  { id: 'pit-exit',    label: 'P\u2191', desc: 'Pit Exit'   },
  { id: 'grandstand',  label: '\u2586',  desc: 'Grandstand' },
  { id: 'tree',        label: '\u2663',  desc: 'Tree'       },
  { id: 'barrier',     label: '\u25ac',  desc: 'Barrier'    },
  { id: 'start-line',  label: 'S',  desc: 'Start Line' },
  { id: 'finish-line', label: 'F',  desc: 'Finish Line'},
  { id: '',            label: '\u232b',  desc: 'Erase'      },
];

export class TilePalette {
  constructor() {
    this.selectedTile = 'straight-h';
    this.el = null;
    this.onSelect = null;
  }

  mount(container) {
    this.el = document.createElement('div');
    this.el.id = 'tile-palette';
    Object.assign(this.el.style, {
      position: 'fixed', left: '10px', top: '50%',
      transform: 'translateY(-50%)',
      background: 'rgba(20,20,30,0.95)', border: '1px solid #444',
      borderRadius: '8px', padding: '8px', display: 'flex',
      flexDirection: 'column', gap: '3px', zIndex: '1000', minWidth: '56px',
    });

    const title = document.createElement('div');
    title.textContent = 'TILES';
    Object.assign(title.style, {
      color: '#888', font: 'bold 10px Courier New',
      textAlign: 'center', marginBottom: '4px',
    });
    this.el.appendChild(title);

    for (let i = 0; i < TILE_TYPES.length; i++) {
      const tile = TILE_TYPES[i];
      const btn = document.createElement('button');
      btn.dataset.tileId = tile.id;
      btn.title = tile.desc;
      btn.textContent = tile.label;
      Object.assign(btn.style, {
        background: '#2a2a3a', border: '1px solid #555', color: '#ccc',
        font: '13px monospace', padding: '3px 6px', cursor: 'pointer',
        borderRadius: '3px', textAlign: 'center', width: '44px',
      });
      btn.addEventListener('click', () => this.select(tile.id));
      this.el.appendChild(btn);
    }

    container.appendChild(this.el);
    this.refresh();
  }

  select(tileId) {
    this.selectedTile = tileId;
    this.refresh();
    if (this.onSelect) this.onSelect(tileId);
  }

  refresh() {
    if (!this.el) return;
    const btns = this.el.querySelectorAll('button');
    btns.forEach(btn => {
      const active = btn.dataset.tileId === this.selectedTile;
      btn.style.background = active ? '#4a4a6a' : '#2a2a3a';
      btn.style.borderColor = active ? '#a0a0ff' : '#555';
      btn.style.color = active ? '#fff' : '#ccc';
    });
  }

  show() { if (this.el) this.el.style.display = 'flex'; }
  hide() { if (this.el) this.el.style.display = 'none'; }

  unmount() {
    if (this.el) { this.el.remove(); this.el = null; }
  }
}
