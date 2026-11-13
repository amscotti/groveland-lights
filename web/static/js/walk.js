// @ts-check
// Walk-the-town client (T-205): geolocation readout, low-accuracy save gate,
// draggable/keyboard pin pad (±25 m), manual fallback, photo preview,
// htmx error-swap config. CSP: no eval/inline/console.

const EARTH_M_PER_DEG = 111194.9; // mean-earth metres per degree of latitude
const ACCURACY_GATE_M = 30;
const PAD_SPAN_M = 50, PAD_HALF_UNITS = 100; // the pad spans ±25 m across the viewBox
const BBOX_MESSAGE = 'That position is outside Groveland town bounds';

/** @type {Record<string, [number, number]>} metre nudges per arrow key. */
const NUDGE_STEPS = { ArrowUp: [1, 0], ArrowDown: [-1, 0], ArrowRight: [0, 1], ArrowLeft: [0, -1] };

/** @type {{baseLat: number|null, baseLon: number|null, accuracyM: number|null, offLatM: number, offLonM: number, lowAccAccepted: boolean, manual: boolean}} */
const state = { baseLat: null, baseLon: null, accuracyM: null, offLatM: 0, offLonM: 0, lowAccAccepted: false, manual: false };

/** @type {(id: string) => HTMLElement | null} */
const el = (id) => document.getElementById(id);
const fmt6 = (/** @type {number} */ v) => v.toFixed(6);
const posText = (/** @type {number} */ lat, /** @type {number} */ lon) => `${fmt6(lat)}, ${fmt6(lon)}`;
const degLat = (/** @type {number} */ m) => m / EARTH_M_PER_DEG;
const degLon = (/** @type {number} */ m, /** @type {number} */ lat) => m / (EARTH_M_PER_DEG * Math.max(Math.cos((lat * Math.PI) / 180), 1e-6));
const clampOff = (/** @type {number} */ m) => Math.min(PAD_SPAN_M / 2, Math.max(-PAD_SPAN_M / 2, m));

/** @param {string} id @param {string} text */
function setText(id, text) {
  const node = el(id);
  if (node) node.textContent = text;
}

/** @param {string} id @param {string} value */
function setHiddenValue(id, value) {
  const input = el(id);
  if (input instanceof HTMLInputElement) input.value = value;
}

/** @returns {{lat: number, lon: number} | null} geolocation fix plus pin offset. */
function effective() {
  if (state.baseLat === null || state.baseLon === null) return null;
  return { lat: state.baseLat + degLat(state.offLatM), lon: state.baseLon + degLon(state.offLonM, state.baseLat) };
}

/** The typed manual position, or null until both fields parse. */
function manualPoint() {
  const latInput = el('manual-lat');
  const lonInput = el('manual-lon');
  if (!(latInput instanceof HTMLInputElement) || !(lonInput instanceof HTMLInputElement)) return null;
  const lat = Number(latInput.value), lon = Number(lonInput.value);
  if (!Number.isFinite(lat) || !Number.isFinite(lon) || latInput.value === '' || lonInput.value === '') return null;
  return { lat, lon };
}

function parseBBox() {
  const section = el('walk');
  const parts = (section instanceof HTMLElement ? (section.dataset.bbox ?? '') : '').split(',').map(Number);
  if (parts.length !== 4 || parts.some((n) => !Number.isFinite(n))) return null;
  return { minLat: parts[0], minLon: parts[1], maxLat: parts[2], maxLon: parts[3] };
}

function outsideBBox(/** @type {{lat: number, lon: number} | null} */ pt) {
  const box = parseBBox();
  return !!pt && !!box &&
    !(pt.lat >= box.minLat && pt.lat <= box.maxLat && pt.lon >= box.minLon && pt.lon <= box.maxLon);
}

/** @param {string} message */
function setAlert(message) {
  const alert = el('walk-alert');
  if (!(alert instanceof HTMLElement)) return;
  alert.textContent = message;
  alert.hidden = message === '';
}

/** Sync every readout, the pin, the hidden inputs and the save gate. */
function render(/** @type {{lat: number, lon: number} | null} */ pt) {
  if (pt) {
    setText('walk-pos', posText(pt.lat, pt.lon));
    setText('pin-pos', posText(pt.lat, pt.lon));
    setHiddenValue('hidden-lat', String(pt.lat));
    setHiddenValue('hidden-lon', String(pt.lon));
  }
  const pin = el('pin');
  if (!state.manual && pin instanceof SVGElement) {
    const x = PAD_HALF_UNITS + (state.offLonM / PAD_SPAN_M) * 2 * PAD_HALF_UNITS;
    const y = PAD_HALF_UNITS - (state.offLatM / PAD_SPAN_M) * 2 * PAD_HALF_UNITS;
    pin.setAttribute('transform', `translate(${x} ${y})`);
  }
  const accuracy = state.manual ? 'manual position' : state.accuracyM === null ? 'accuracy pending…' : `accuracy ${Math.round(state.accuracyM)} m`;
  setText('walk-accuracy', accuracy);
  setHiddenValue('hidden-accuracy', state.accuracyM === null ? '' : String(Math.round(state.accuracyM)));
  setAlert(outsideBBox(pt) ? BBOX_MESSAGE : '');
  const add = el('add-light');
  const low = el('save-lowacc');
  const needChoice = state.accuracyM !== null && state.accuracyM > ACCURACY_GATE_M && !state.lowAccAccepted;
  if (add instanceof HTMLButtonElement) add.disabled = !pt || needChoice || outsideBBox(pt);
  if (low instanceof HTMLElement) low.hidden = !needChoice;
}

function update() {
  render(state.manual ? manualPoint() : effective());
}

/** Geolocation denied or unavailable: fall back to typed coordinates. */
function enterManual() {
  if (state.manual) return;
  state.manual = true;
  const pad = el('pinpad');
  const box = el('manual-pos');
  if (pad) pad.setAttribute('hidden', ''); // svg: not HTMLElement, so no .hidden
  if (box instanceof HTMLElement) box.hidden = false;
  for (const id of ['manual-lat', 'manual-lon']) el(id)?.addEventListener('input', update);
  update();
}

function startWatch() {
  if (!('geolocation' in navigator)) {
    enterManual();
    return;
  }
  navigator.geolocation.watchPosition((pos) => {
    if (state.manual) return; // an explicit manual position wins
    state.baseLat = pos.coords.latitude;
    state.baseLon = pos.coords.longitude;
    state.accuracyM = pos.coords.accuracy;
    update();
  }, (err) => {
    // Denied/unavailable falls back to manual input — but only before any
    // fix: permission overrides (setGeolocation) can surface a transient
    // error mid-watch, and the next callback refreshes the readout.
    if ((err.code === 1 || err.code === 2) && state.baseLat === null) enterManual();
  }, { enableHighAccuracy: true, maximumAge: 2000, timeout: 10000 });
}

/** Pointer drag over the pad (px → metres via PAD_SPAN_M) + arrow-key nudges. */
function bindPin() {
  const pin = el('pin');
  if (!(pin instanceof SVGElement)) return;
  let dragging = false;
  let start = { x: 0, y: 0, offLatM: 0, offLonM: 0 };
  pin.addEventListener('pointerdown', (e) => {
    dragging = true;
    start = { x: e.clientX, y: e.clientY, offLatM: state.offLatM, offLonM: state.offLonM };
    pin.setPointerCapture(e.pointerId);
    e.preventDefault();
  });
  pin.addEventListener('pointermove', (e) => {
    if (!dragging) return;
    const pad = el('pinpad');
    if (!(pad instanceof SVGElement)) return;
    const rect = pad.getBoundingClientRect();
    if (rect.width === 0 || rect.height === 0) return;
    state.offLonM = clampOff(start.offLonM + ((e.clientX - start.x) * PAD_SPAN_M) / rect.width);
    state.offLatM = clampOff(start.offLatM - ((e.clientY - start.y) * PAD_SPAN_M) / rect.height);
    update();
  });
  for (const event of ['pointerup', 'pointercancel']) pin.addEventListener(event, () => { dragging = false; });
  pin.addEventListener('keydown', (e) => {
    const step = NUDGE_STEPS[e.key];
    if (!step) return;
    [state.offLatM, state.offLonM] = [clampOff(state.offLatM + step[0]), clampOff(state.offLonM + step[1])];
    e.preventDefault();
    update();
  });
}

/** In-form preview img; the object URL honours EXIF orientation intrinsically. */
function bindPhotoPreview() {
  const input = el('walk-photo');
  const slot = el('photo-preview-slot');
  if (!(input instanceof HTMLInputElement) || !(slot instanceof HTMLElement)) return;
  input.addEventListener('change', () => {
    slot.replaceChildren();
    const file = input.files?.[0];
    if (!file) return;
    const img = document.createElement('img');
    img.id = 'photo-preview';
    img.alt = 'Photo preview';
    img.src = URL.createObjectURL(file);
    slot.append(img);
  });
}

function init() {
  // htmx 2.0.11 must swap 4xx bodies (the 422 alert / 409 dup panel) into the
  // form's #dup-slot target: base.html's htmx-config meta carries allowEval,
  // but the vendored default refuses to swap 4xx — override before init.
  const h = window.htmx;
  if (h) {
    h.config.responseHandling = [
      { code: '204', swap: false }, { code: '[23]..', swap: true }, { code: '[45]..', swap: true, error: false },
    ];
  }
  bindPin();
  bindPhotoPreview();
  el('add-light')?.addEventListener('click', () => {
    const form = el('add-form');
    if (form instanceof HTMLElement) { form.hidden = false; el('walk-label')?.focus(); }
  });
  el('save-lowacc')?.addEventListener('click', () => { state.lowAccAccepted = true; update(); });
  startWatch();
  update();
}
init(); export {};

