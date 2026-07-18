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
        options: { indexAxis: 'y', responsive: true, animation, plugins: { legend: { display: false } } },
      }));
    } else if (chartType === 'line-multi') {
      chartRegistry.set(canvasId, new Chart(canvas, {
        type: 'line',
        data: { labels: data.labels, datasets: data.datasets },
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

document.addEventListener('DOMContentLoaded', () => {
  try {
    if (window.Telegram && window.Telegram.WebApp) {
      window.Telegram.WebApp.ready();
      window.Telegram.WebApp.expand();
    }
  } catch (e) { /* not inside Telegram */ }
  initCharts(document);
});

// htmx events bubble to document — listen there, NOT on document.body (this
// script is in <head>, where document.body is still null).
document.addEventListener('htmx:afterSwap', (evt) => initCharts(evt.detail.target));
