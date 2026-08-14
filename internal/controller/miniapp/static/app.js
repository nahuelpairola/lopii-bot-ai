// chartRegistry tracks live Chart.js instances by canvas id, so a re-init
// (initial load or htmx:afterSwap) always destroys the previous instance
// before creating a new one — this is the hard requirement from the spec's
// canvas-lifecycle rule.
const chartRegistry = new Map();

function readDataIsland(id) {
  const el = document.getElementById(id);
  if (!el) return null;
  return JSON.parse(el.textContent);
}

function reducedMotion() {
  return window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

function destroyChart(canvasId) {
  const existing = chartRegistry.get(canvasId);
  if (existing) {
    existing.destroy();
    chartRegistry.delete(canvasId);
  }
}

// initCharts finds every <canvas data-chart-type data-chart-data-id> in the
// current DOM, destroys any prior instance for that canvas, and draws a
// fresh Chart.js chart from its paired JSON data island.
function initCharts(root) {
  const canvases = (root || document).querySelectorAll('canvas[data-chart-type]');
  canvases.forEach((canvas) => {
    const canvasId = canvas.id;
    destroyChart(canvasId);

    const data = readDataIsland(canvas.dataset.chartDataId);
    if (!data) return;

    const chartType = canvas.dataset.chartType;
    const animation = reducedMotion() ? false : undefined;

    if (chartType === 'bar-grouped') {
      chartRegistry.set(canvasId, new Chart(canvas, {
        type: 'bar',
        data: {
          labels: data.labels,
          datasets: data.datasets,
        },
        options: { responsive: true, animation, plugins: { legend: { display: true } } },
      }));
    } else if (chartType === 'bar-single') {
      chartRegistry.set(canvasId, new Chart(canvas, {
        type: 'bar',
        data: { labels: data.labels, datasets: [{ data: data.values, backgroundColor: data.color }] },
        // autoSkip viene prendido por default y sobre el eje de categorías
        // saltea etiquetas cuando las ve apretadas: en un gráfico horizontal
        // eso deja una barra sí y una no sin nombre. Acá cada barra ES una
        // categoría, así que ninguna etiqueta es opcional.
        options: {
          indexAxis: 'y',
          responsive: true,
          animation,
          plugins: { legend: { display: false } },
          scales: { y: { ticks: { autoSkip: false } } },
        },
      }));
    } else if (chartType === 'line-multi') {
      chartRegistry.set(canvasId, new Chart(canvas, {
        type: 'line',
        data: {
          labels: data.labels,
          // El trazo de una línea lo pinta borderColor, no backgroundColor, y
          // fill viene en false: sin esto la línea sale en el default de
          // Chart.js (rgba(0,0,0,0.1)) y el color del slot sólo se ve en la
          // leyenda. Sin fill a propósito: varias cuentas con relleno
          // superpuesto es barro.
          datasets: data.datasets.map((d) => ({ ...d, borderColor: d.backgroundColor })),
        },
        options: { responsive: true, animation, plugins: { legend: { display: true } } },
      }));
    }
  });
}

// Telegram delivers initData only to client JS (in the launch URL hash) —
// it never reaches the server on the first full-page navigation. So we
// inject it as a header on every htmx request; the server's authInitData
// middleware verifies THAT (the shell itself loads unauthenticated).
document.addEventListener('htmx:configRequest', (evt) => {
  try {
    if (window.Telegram && window.Telegram.WebApp && window.Telegram.WebApp.initData) {
      evt.detail.headers['X-Telegram-Init-Data'] = window.Telegram.WebApp.initData;
    }
  } catch (e) { /* opened outside Telegram — request goes unauthenticated, server 401s */ }
});

// The tab bar is rendered once, in the shell, and htmx only swaps #content —
// so the active tab has to be re-marked client-side after every swap.
function markActiveTab() {
  const active = location.pathname.replace(/^\/app\//, '').split('/')[0];
  document.querySelectorAll('.tab-link').forEach((link) => {
    if (link.dataset.tab === active) {
      link.setAttribute('aria-current', 'page');
    } else {
      link.removeAttribute('aria-current');
    }
  });
}

// initData expires after 24h. A swap that comes back 401 has to say so, not
// leave a half-broken partial on screen.
document.addEventListener('htmx:responseError', (evt) => {
  if (evt.detail.xhr.status !== 401) return;
  const content = document.getElementById('content');
  if (content) {
    content.innerHTML =
      '<article><p>Sesión vencida. Volvé a abrir la app desde el botón del chat.</p></article>';
  }
});

// Telegram's own light/dark setting, not the OS one: pico reads data-theme.
function applyTelegramTheme() {
  try {
    const scheme = window.Telegram && window.Telegram.WebApp && window.Telegram.WebApp.colorScheme;
    if (scheme) document.documentElement.setAttribute('data-theme', scheme);
  } catch (e) { /* not inside Telegram */ }
}

// Chart.js no sabe nada de temas: los ticks y la leyenda salen en #666 fijo y
// la grilla en rgba(0,0,0,0.1), o sea gris oscuro sobre fondo oscuro. Pico ya
// resolvió el par por tema, así que se leen de ahí en vez de hardcodear hex.
function applyChartTheme() {
  const cs = getComputedStyle(document.documentElement);
  Chart.defaults.color = cs.getPropertyValue('--pico-color').trim();
  Chart.defaults.borderColor = cs.getPropertyValue('--pico-muted-border-color').trim();
}

// The native back button beats a link for backing out of a drill. htmx pushes
// the URL, so history.back() restores the previous partial.
function syncBackButton() {
  try {
    const bb = window.Telegram && window.Telegram.WebApp && window.Telegram.WebApp.BackButton;
    if (!bb) return;
    const params = new URLSearchParams(location.search);
    if (params.has('category') || params.has('expand')) {
      bb.show();
    } else {
      bb.hide();
    }
  } catch (e) { /* not inside Telegram */ }
}

document.addEventListener('DOMContentLoaded', () => {
  applyTelegramTheme();
  applyChartTheme();
  try {
    if (window.Telegram && window.Telegram.WebApp) {
      window.Telegram.WebApp.ready();
      window.Telegram.WebApp.expand();
      window.Telegram.WebApp.BackButton.onClick(() => history.back());
      window.Telegram.WebApp.onEvent('themeChanged', () => {
        applyTelegramTheme();
        applyChartTheme();
        initCharts(document);
      });
    }
  } catch (e) { /* not inside Telegram */ }
  initCharts(document);
  markActiveTab();
  syncBackButton();
});

// --- Buscador de la hoja de cuenta ---------------------------------------
// Filtra las filas YA renderizadas: el período elegido y el tope de 50, nada
// más. Buscar en toda la cuenta sería ?q= + ILIKE en ListForAccount; hoy no
// hace falta, el bot ya contesta eso por chat.
// Los ids salen de templates.MovFilterID / MovFilterEmptyID: si cambian allá,
// cambian acá.
const MOV_FILTER_ID = 'mov-filter';
const MOV_FILTER_EMPTY_ID = 'mov-filter-empty';
// Menos de esto no filtra: con una o dos letras matchea media lista y el
// resaltado es puro ruido.
const MOV_FILTER_MIN = 3;

// Un carácter entra, un carácter sale: así los índices del resultado siguen
// apuntando al texto original, que es lo que hace que el <mark> caiga justo.
// Normalizar el string entero de una no sirve — NFD lo alarga ("í" son dos
// unidades) y el resaltado quedaría corrido.
function normChars(s) {
  let out = '';
  for (let i = 0; i < s.length; i++) {
    const n = s[i].normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLowerCase();
    out += n.length ? n[0] : s[i];
  }
  return out;
}

// Reescribe el nodo envolviendo cada aparición en <mark>. Sin innerHTML: el
// texto es del usuario. El original se guarda una vez en dataset.raw, así
// tipear letra por letra no lo va comiendo.
function markMatches(el, q) {
  if (el.dataset.raw === undefined) el.dataset.raw = el.textContent;
  const text = el.dataset.raw;
  const hay = normChars(text);
  el.textContent = '';
  let i = 0;
  for (let at = q ? hay.indexOf(q) : -1; at >= 0; at = hay.indexOf(q, i)) {
    el.append(text.slice(i, at));
    const m = document.createElement('mark');
    m.textContent = text.slice(at, at + q.length);
    el.append(m);
    i = at + q.length;
  }
  el.append(text.slice(i));
}

// El listener va delegado en document: htmx reemplaza #content entero en cada
// navegación, y uno atado al input muere en el primer swap.
document.addEventListener('input', (evt) => {
  if (evt.target.id !== MOV_FILTER_ID) return;
  const raw = evt.target.value.trim();
  const q = raw.length >= MOV_FILTER_MIN ? normChars(raw) : '';
  let shown = 0;
  document.querySelectorAll('.mov-row').forEach((row) => {
    const hit = q === '' || normChars(row.querySelector('.mov-text').textContent).includes(q);
    // display y no el atributo hidden: .mov-row trae display:flex, que le gana
    // al display:none que el navegador le da a [hidden].
    row.style.display = hit ? '' : 'none';
    if (hit) shown++;
    // El monto queda afuera de la búsqueda y del resaltado a propósito: "500"
    // matchearía fechas, montos y cualquier descripción con un número.
    row.querySelectorAll('.mov-title, .mov-meta').forEach((el) => markMatches(el, hit ? q : ''));
  });
  const empty = document.getElementById(MOV_FILTER_EMPTY_ID);
  if (empty) empty.hidden = shown > 0;
});

// htmx events bubble to document — listen there, NOT on document.body (this
// script is in <head>, where document.body is still null).
document.addEventListener('htmx:afterSwap', (evt) => {
  initCharts(evt.detail.target);
  markActiveTab();
  syncBackButton();
});
