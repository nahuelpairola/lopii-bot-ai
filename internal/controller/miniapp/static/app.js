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

const ROLE_INCOME_COLOR = '#1baf7a';

function colorForRole(role) {
  if (role === 'income') return ROLE_INCOME_COLOR;
  const accent = getComputedStyle(document.documentElement)
    .getPropertyValue('--pico-primary').trim();
  return accent || '#2a78d6';
}

let chartLib = null;

function loadChartLib() {
  if (window.Chart) return Promise.resolve();
  if (!chartLib) {
    chartLib = new Promise((resolve, reject) => {
      const src = document.body.dataset.chartSrc;
      if (!src) {
        reject(new Error('sin data-chart-src'));
        return;
      }
      const tag = document.createElement('script');
      tag.src = src;
      tag.onload = resolve;
      tag.onerror = reject;
      document.head.appendChild(tag);
    });
  }
  return chartLib;
}

function initCharts(root) {
  const canvases = (root || document).querySelectorAll('canvas[data-chart-type]');
  if (!canvases.length) return;
  loadChartLib()
    .then(() => {
      applyChartTheme();
      renderCharts(canvases);
    })
    .catch(() => {
      canvases.forEach((canvas) => {
        const box = canvas.closest('.chart-box');
        if (box) box.hidden = true;
      });
    });
}

function renderCharts(canvases) {
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
          datasets: data.datasets.map((d) => ({
            ...d,
            backgroundColor: d.backgroundColor || colorForRole(d.role),
          })),
        },
        options: { responsive: true, maintainAspectRatio: false, animation, plugins: { legend: { display: true } } },
      }));
    } else if (chartType === 'bar-single') {
      const box = canvas.parentElement;
      if (box && box.classList.contains('chart-box')) {
        box.style.height = Math.max(220, data.labels.length * 28 + 48) + 'px';
      }
      chartRegistry.set(canvasId, new Chart(canvas, {
        type: 'bar',
        data: { labels: data.labels, datasets: [{ data: data.values, backgroundColor: colorForRole(data.role) }] },
        options: {
          indexAxis: 'y',
          responsive: true,
          maintainAspectRatio: false,
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
          datasets: data.datasets.map((d) => {
            const color = d.backgroundColor || colorForRole(d.role);
            return { ...d, backgroundColor: color, borderColor: color };
          }),
        },
        options: { responsive: true, maintainAspectRatio: false, animation, plugins: { legend: { display: true } } },
      }));
    }
  });
}

document.addEventListener('htmx:configRequest', (evt) => {
  try {
    if (window.Telegram && window.Telegram.WebApp && window.Telegram.WebApp.initData) {
      evt.detail.headers['X-Telegram-Init-Data'] = window.Telegram.WebApp.initData;
    }
  } catch (e) {}
});

const ROUTE_STATUS_ID = 'route-status';

function announceRoute() {
  const status = document.getElementById(ROUTE_STATUS_ID);
  if (!status) return;
  const heading = document.querySelector('#content h1');
  const tab = document.querySelector('.tab-link[aria-current]');
  const name = (heading && heading.textContent.trim()) || (tab && tab.textContent.trim());
  if (!name || name === status.textContent) return;
  status.textContent = name;
}

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

document.addEventListener('htmx:responseError', (evt) => {
  const content = document.getElementById('content');
  if (!content) return;
  if (evt.detail.xhr.status === 401) {
    content.innerHTML =
      '<article><p>Sesión vencida. Volvé a abrir la app desde el botón del chat.</p></article>';
    return;
  }
  content.innerHTML =
    '<article><p>No se pudo cargar. Probá de nuevo en un momento.</p></article>';
});

document.addEventListener('htmx:sendError', () => {
  const content = document.getElementById('content');
  if (content) {
    content.innerHTML =
      '<article><p>Sin conexión. Probá de nuevo cuando vuelva.</p></article>';
  }
});

function applyTelegramTheme() {
  try {
    const scheme = window.Telegram && window.Telegram.WebApp && window.Telegram.WebApp.colorScheme;
    if (scheme) document.documentElement.setAttribute('data-theme', scheme);
  } catch (e) {}
}

function applyTelegramChrome() {
  try {
    const wa = window.Telegram && window.Telegram.WebApp;
    if (!wa) return;
    const cs = getComputedStyle(document.documentElement);
    const page = cs.getPropertyValue('--pico-background-color').trim();
    if (page && wa.setHeaderColor) wa.setHeaderColor(page);
    if (page && wa.setBottomBarColor) wa.setBottomBarColor(page);
  } catch (e) {}
}

function applyChartTheme() {
  if (!window.Chart) return;
  const cs = getComputedStyle(document.documentElement);
  Chart.defaults.color = cs.getPropertyValue('--pico-color').trim();
  Chart.defaults.borderColor = cs.getPropertyValue('--pico-muted-border-color').trim();
}

function syncBackButton() {
  try {
    const bb = window.Telegram && window.Telegram.WebApp && window.Telegram.WebApp.BackButton;
    if (!bb) return;
    const params = new URLSearchParams(location.search);
    if (params.has('category') || params.has('expand') || params.has('account')) {
      bb.show();
    } else {
      bb.hide();
    }
  } catch (e) {}
}

document.addEventListener('DOMContentLoaded', () => {
  applyTelegramTheme();
  applyTelegramChrome();
  try {
    if (window.Telegram && window.Telegram.WebApp) {
      window.Telegram.WebApp.ready();
      window.Telegram.WebApp.expand();
      window.Telegram.WebApp.BackButton.onClick(() => history.back());
      window.Telegram.WebApp.onEvent('themeChanged', () => {
        applyTelegramTheme();
        applyTelegramChrome();
        initCharts(document);
      });
    }
  } catch (e) {}
  initCharts(document);
  markActiveTab();
  syncBackButton();
});

const MOV_FILTER_ID = 'mov-filter';
const MOV_FILTER_EMPTY_ID = 'mov-filter-empty';
const MOV_FILTER_MIN = 3;

function normChars(s) {
  let out = '';
  for (let i = 0; i < s.length; i++) {
    const n = s[i].normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLowerCase();
    out += n.length ? n[0] : s[i];
  }
  return out;
}

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

document.addEventListener('input', (evt) => {
  if (evt.target.id !== MOV_FILTER_ID) return;
  const raw = evt.target.value.trim();
  const q = raw.length >= MOV_FILTER_MIN ? normChars(raw) : '';
  let shown = 0;
  document.querySelectorAll('.mov-row').forEach((row) => {
    const hit = q === '' || normChars(row.querySelector('.mov-text').textContent).includes(q);
    row.style.display = hit ? '' : 'none';
    if (hit) shown++;
    row.querySelectorAll('.mov-title, .mov-meta').forEach((el) => markMatches(el, hit ? q : ''));
  });
  document.querySelectorAll('.mov-group').forEach((group) => {
    const anyVisible = [...group.querySelectorAll('.mov-row')]
      .some((row) => row.style.display !== 'none');
    group.style.display = anyVisible ? '' : 'none';
  });
  const empty = document.getElementById(MOV_FILTER_EMPTY_ID);
  if (empty) empty.textContent = shown > 0 ? '' : (empty.dataset.msg || '');
});

document.addEventListener('htmx:afterSwap', (evt) => {
  initCharts(evt.detail.target);
  markActiveTab();
  announceRoute();
  syncBackButton();
});
