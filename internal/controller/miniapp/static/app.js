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

// Los gráficos ya no reciben color desde Go: reciben un rol. Chart.js no lee
// variables CSS, así que el color se resuelve acá, del mismo computed style
// del que applyChartTheme saca los ticks y la grilla. Eso es lo que hace que
// el gasto siga al acento del tema del usuario.
//
// El ingreso se queda en su verde: Telegram tiene color destructivo y no tiene
// uno positivo, la misma asimetría que el par del Neto en app.css.
const ROLE_INCOME_COLOR = '#1baf7a';

function colorForRole(role) {
  if (role === 'income') return ROLE_INCOME_COLOR;
  const accent = getComputedStyle(document.documentElement)
    .getPropertyValue('--pico-primary').trim();
  return accent || '#2a78d6';
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
          datasets: data.datasets.map((d) => ({
            ...d,
            backgroundColor: d.backgroundColor || colorForRole(d.role),
          })),
        },
        options: { responsive: true, animation, plugins: { legend: { display: true } } },
      }));
    } else if (chartType === 'bar-single') {
      chartRegistry.set(canvasId, new Chart(canvas, {
        type: 'bar',
        data: { labels: data.labels, datasets: [{ data: data.values, backgroundColor: colorForRole(data.role) }] },
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
          datasets: data.datasets.map((d) => {
            const color = d.backgroundColor || colorForRole(d.role);
            return { ...d, backgroundColor: color, borderColor: color };
          }),
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

// initData vence a las 24h: un 401 tiene su propio texto porque tiene su propia
// salida (volver a abrir desde el chat). Todo lo demás —500, timeout, la mala
// conexión que PRODUCT.md nombra como parte de la escena de uso— antes no decía
// NADA: la opacidad volvía sola y el tap se leía como ignorado, no como fallado.
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

// Una conexión caída no dispara responseError: la request no llega a tener
// respuesta. Sin este handler ese caso —el más probable en el celular— es el
// único que sigue mudo.
document.addEventListener('htmx:sendError', () => {
  const content = document.getElementById('content');
  if (content) {
    content.innerHTML =
      '<article><p>Sin conexión. Probá de nuevo cuando vuelva.</p></article>';
  }
});

// Telegram's own light/dark setting, not the OS one: pico reads data-theme.
function applyTelegramTheme() {
  try {
    const scheme = window.Telegram && window.Telegram.WebApp && window.Telegram.WebApp.colorScheme;
    if (scheme) document.documentElement.setAttribute('data-theme', scheme);
  } catch (e) { /* not inside Telegram */ }
}

// Telegram dibuja su propia barra arriba del webview y otra abajo. Sin esto la
// app es un rectángulo de otro color adentro de esa chrome, que es lo que más
// la delata como "una web adentro de Telegram" en vez de parte del cliente.
//
// setHeaderColor es Bot API 6.1+; setBottomBarColor es 7.10+, bastante más
// nuevo que el piso que podemos asumir, así que va con guarda de feature.
// Los dos aceptan #RRGGBB, y los sacamos del computed style para que sean
// exactamente los mismos valores que ya está usando el CSS.
function applyTelegramChrome() {
  try {
    const wa = window.Telegram && window.Telegram.WebApp;
    if (!wa) return;
    const cs = getComputedStyle(document.documentElement);
    const page = cs.getPropertyValue('--pico-background-color').trim();
    if (page && wa.setHeaderColor) wa.setHeaderColor(page);
    if (page && wa.setBottomBarColor) wa.setBottomBarColor(page);
  } catch (e) { /* fuera de Telegram, o cliente viejo sin estas APIs */ }
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
    // 'account' faltaba: la hoja de cuenta era la única pantalla profunda sin
    // botón Atrás nativo, y es justo la vista que reconcilia el saldo — la única
    // donde se ve una transferencia o una compra en USD.
    if (params.has('category') || params.has('expand') || params.has('account')) {
      bb.show();
    } else {
      bb.hide();
    }
  } catch (e) { /* not inside Telegram */ }
}

document.addEventListener('DOMContentLoaded', () => {
  applyTelegramTheme();
  applyChartTheme();
  applyTelegramChrome();
  try {
    if (window.Telegram && window.Telegram.WebApp) {
      window.Telegram.WebApp.ready();
      window.Telegram.WebApp.expand();
      window.Telegram.WebApp.BackButton.onClick(() => history.back());
      window.Telegram.WebApp.onEvent('themeChanged', () => {
        applyTelegramTheme();
        applyChartTheme();
        applyTelegramChrome();
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
