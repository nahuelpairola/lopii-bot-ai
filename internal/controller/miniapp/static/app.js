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

// htmx events bubble to document — listen there, NOT on document.body (this
// script is in <head>, where document.body is still null).
document.addEventListener('htmx:afterSwap', (evt) => {
  initCharts(evt.detail.target);
  markActiveTab();
  syncBackButton();
});
