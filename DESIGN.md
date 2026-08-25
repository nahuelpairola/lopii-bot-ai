---
name: Lopii Mini App
description: A Telegram Mini App that reads like an account statement — flat, tabular, and painted in the user's own Telegram theme.
colors:
  theme-accent: "var(--tg-theme-accent-text-color, var(--tg-theme-link-color, #0172ad))"
  theme-accent-dark: "var(--tg-theme-accent-text-color, var(--tg-theme-link-color, #01aaff))"
  theme-button: "var(--tg-theme-button-color, #0172ad)"
  theme-button-text: "var(--tg-theme-button-text-color, #ffffff)"
  statement-ink: "var(--tg-theme-text-color, #373c44)"
  statement-ink-dark: "var(--tg-theme-text-color, #c2c7d0)"
  margin-grey: "var(--tg-theme-hint-color, #646b79)"
  margin-grey-dark: "var(--tg-theme-hint-color, #7b8495)"
  hairline: "var(--tg-theme-section-separator-color, var(--tg-theme-hint-color, rgb(231, 234, 239.5)))"
  hairline-dark: "var(--tg-theme-section-separator-color, var(--tg-theme-hint-color, #202632))"
  paper: "var(--tg-theme-secondary-bg-color, #ffffff)"
  paper-dark: "var(--tg-theme-secondary-bg-color, rgb(19, 22.5, 30.5))"
  card-paper: "var(--tg-theme-section-bg-color, rgb(251, 251.5, 252.25))"
  card-paper-dark: "var(--tg-theme-section-bg-color, rgb(26, 30.5, 40.25))"
  positive-green: "color-mix(in srgb, #0ca30c 70%, var(--pico-color))"
  negative-red: "color-mix(in srgb, var(--tg-theme-destructive-text-color, #d03b3b) 70%, var(--pico-color))"
  ledger-green: "#1baf7a"
  slot-1: "#2a78d6"
  slot-2: "#1baf7a"
  slot-3: "#eda100"
  slot-4: "#008300"
  slot-5: "#4a3aa7"
  slot-6: "#e34948"
  slot-7: "#e87ba4"
  slot-8: "#eb6834"
typography:
  kpi:
    fontFamily: "system-ui, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif"
    fontSize: "1.75rem"
    fontWeight: 600
    lineHeight: 1.2
    fontFeature: "tabular-nums"
  body:
    fontFamily: "system-ui, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif"
    fontSize: "1rem"
    fontWeight: 400
    lineHeight: 1.5
  table:
    fontFamily: "system-ui, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif"
    fontSize: "0.85rem"
    fontWeight: 400
  label:
    fontFamily: "system-ui, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif"
    fontSize: "0.8rem"
    fontWeight: 400
  micro:
    fontFamily: "system-ui, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 400
  cursor:
    fontFamily: "system-ui, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif"
    fontSize: "1.5rem"
    fontWeight: 400
rounded:
  sm: "0.25rem"
  pill: "1rem"
spacing:
  cell: "0.35rem 0.4rem"
  row: "0.5rem"
  base: "0.75rem"
  touch: "44px"
components:
  kpi-card:
    backgroundColor: "{colors.card-paper}"
    textColor: "{colors.statement-ink}"
    typography: "{typography.kpi}"
    rounded: "{rounded.sm}"
    padding: "{spacing.base}"
  chip:
    backgroundColor: "transparent"
    textColor: "{colors.margin-grey}"
    typography: "{typography.label}"
    rounded: "{rounded.pill}"
    padding: "0.25rem 0.6rem"
  chip-active:
    backgroundColor: "{colors.theme-button}"
    textColor: "{colors.theme-button-text}"
    typography: "{typography.label}"
    rounded: "{rounded.pill}"
    padding: "0.25rem 0.6rem"
  tab-link:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.margin-grey}"
    typography: "{typography.body}"
    height: "{spacing.touch}"
    width: "{spacing.touch}"
  tab-link-active:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.theme-accent}"
    typography: "{typography.body}"
    height: "{spacing.touch}"
    width: "{spacing.touch}"
  movement-row:
    backgroundColor: "transparent"
    textColor: "{colors.statement-ink}"
    typography: "{typography.body}"
    padding: "0.5rem 0"
  account-card:
    backgroundColor: "{colors.card-paper}"
    textColor: "{colors.statement-ink}"
    typography: "{typography.kpi}"
    rounded: "{rounded.sm}"
    height: "{spacing.touch}"
  variacion-callout:
    backgroundColor: "transparent"
    textColor: "{colors.margin-grey}"
    typography: "{typography.label}"
    padding: "0.5rem 0 0"
  group-title:
    backgroundColor: "transparent"
    textColor: "{colors.margin-grey}"
    typography: "{typography.label}"
  tappable:
    minWidth: "{spacing.touch}"
    minHeight: "{spacing.touch}"
  accounts-total:
    textColor: "{colors.statement-ink}"
    typography: "{typography.kpi}"
---

# Design System: Lopii Mini App

## Overview

**Creative North Star: "The Account Statement"**

The Mini App is a statement that closes. Every screen exists so the number at
the top can be explained by the rows underneath it, and the user can walk from
a total down to the individual movement that produced it without ever losing
the window they were reading. Trust is the deliverable: a figure that does not
reconcile is worse than no figure at all. That is why the account leaf prints
an opening and a closing balance around a capped list and says the list is
capped, and why footers come from aggregates over every row rather than from
what fits on screen.

**The app does not own its palette. Telegram does.** Every color token is a
`var(--tg-theme-*)` reference with a fallback, so the surface is painted in
whatever theme the user chose in their client — including a custom one this
system has never seen. That is the largest structural fact about this design
system, and it inverts the usual relationship: brand does not live in hue here,
because hue belongs to the user. It lives in density, in the tabular figure, and
in the discipline that every number reconciles.

The material is deliberately thin. Pico CSS 2.1.1 provides the base, `app.css`
adds roughly 430 lines of scoped corrections, and every one of them exists
because something specific broke on a 360px Telegram webview — a sticky column
losing its border, a chip whose selected state measured 1.32:1, a tab bar
sitting under the iOS home indicator. The system is a stack of small, argued
repairs, not a theme. New work continues that discipline: reach for the Pico
token before writing a value, and when you override, say why in the CSS.

**Key Characteristics:**

- Telegram-themed: 19 Pico color tokens are remapped onto `--tg-theme-*`, each
  with a fallback, so the app follows the user's client and still works outside
  it.
- Flat and tonal: depth is a 1px hairline border and a card background, never a
  decorative shadow.
- Tabular numerals everywhere money appears, so columns of figures stay
  comparable.
- 44px minimum on everything tappable — tabs, period arrows, account cards.
- No web fonts, no build step, no CDN: every dependency is vendored and pinned
  under `static/`.
- Color is reinforcement, never the only channel: the active tab carries a 2px
  bar, the default account carries a ★ with an `aria-label`.

## Colors

There is no fixed palette. There is a **mapping**, plus three narrow exceptions
where a fixed value is the right answer and the reason is written down.

Every token below resolves through `var(--tg-theme-*, <fallback>)`. Inside
Telegram the variable wins; outside it — or on a client older than Bot API 7.0,
which is when `section-bg-color`, `accent-text-color` and
`section-separator-color` arrived — the fallback wins and the app looks exactly
as it did before the mapping existed. The fallbacks are Pico's own light and
dark palettes, which is why they differ per theme.

### Primary

- **Theme Accent** (`--tg-theme-accent-text-color` → `--tg-theme-link-color` →
  Pico's azure): every interactive affordance — links, the active tab's label
  and its 2px bar, the default account star — plus the expense series in every
  chart and the Evolución heat shading.
- **Theme Button** / **Theme Button Text** (`--tg-theme-button-color` /
  `--tg-theme-button-text-color`): the selected chip's fill and its label.
  Selection is a fill, not a hue shift; Pico's one-step
  `primary → primary-hover` shift measures 1.32:1 in dark mode, below the 3:1
  WCAG asks of a state.

### Secondary

- **Ledger Green** (`#1baf7a`): the income series in charts, and the one chart
  color still fixed. Fixed because Telegram exposes a destructive color and
  **no positive one** — there is nothing in the theme to derive it from.

### Tertiary

- **Account Slots** (eight fixed hexes, `slot-1` through `slot-8`): assigned by
  position so an account's snapshot card and its trend line always carry the
  same color. Color follows the entity, never the ordering of a particular
  render. Fixed on purpose — see The Categorical Exception Rule.

### Neutral

- **Statement Ink** (`--tg-theme-text-color`): body text and every figure not
  carrying status.
- **Margin Grey** (`--tg-theme-hint-color`): labels above KPIs, the
  date-and-category line under a movement title, disabled period arrows,
  subcategory rows in Evolución. Most of the interface lives here.
- **Hairline** (`--tg-theme-section-separator-color`, falling back to the hint
  color): the 1px rule between movement rows, the tab bar's top border, every
  table cell border, an unselected chip's edge.
- **Paper** (`--tg-theme-secondary-bg-color`): the page and the tab bar, which
  must be opaque because content scrolls beneath it.
- **Card Paper** (`--tg-theme-section-bg-color`): cards and form fields — no
  longer the Variación callout, which is borderless and fill-less (see the
  Variación Callout component). The page/section split is Telegram's own: the
  page sits on the secondary background and cards rest on top of it.

### Status

- **Positive Green** and **Negative Red**: a positive or negative Neto, and
  nothing else. Both are **derived, not fixed** — see The Mixed Status Rule.

### Named Rules

**The Theme-Follows-The-User Rule.** A color literal in `app.css` is a bug
unless one of the three documented exceptions covers it (account slots, the
income series, the tap-highlight). Reach for a Pico custom property; it is
already mapped to Telegram. **Every `var(--tg-…)` carries a fallback** — a
missing one does not fail loudly, it paints the element transparent or black,
and only outside Telegram, where nobody is looking.
`TestAppCSS_EveryTelegramVarHasFallback` enforces this.

**The Three Contexts Rule.** Pico defines its palette in three theme contexts:
`:root:not([data-theme=dark])`, `@media (prefers-color-scheme: dark)` with
`:root:not([data-theme])`, and `[data-theme=dark]`. The remap must appear in
**all three**, each with that theme's own fallbacks. One plain `:root` block
loses on specificity; one high-specificity block wins everywhere and then its
single-theme fallbacks override Pico's dark palette outside Telegram, silently
turning dark mode light. `TestAppCSS_RemapsAllThreePicoThemeContexts` enforces
this.

**The Mixed Status Rule.** The Neto pair is derived, never fixed:
`color-mix(in srgb, <hue> 70%, var(--pico-color))`. CSS cannot compute a
contrast ratio, but it can mix a hue toward the theme's own text color, which
Telegram has already guaranteed legible against its background — the hue
survives and the lightness follows the theme. Measured, the fixed values gave
3.35:1 (green, light) and 3.75:1 (red, dark), passing AA only by counting as
large text at 28px; mixed at 70% the four cases measure 4.82, 6.34, 6.50 and
4.83, clearing the 4.5:1 bar for *normal* text. Lowering the 70% buys contrast
and costs saturation. The pair is asymmetric on purpose: red prefers
`--tg-theme-destructive-text-color`, green has no counterpart to prefer.

**The One Status Rule.** Positive Green and Negative Red appear on exactly one
element per screen: the Neto figure. Variación de saldos is deliberately
uncolored — a positive variation is not "good" the way a positive Neto is; it
can be a CEDEAR that rose or a load that was missing, and tinting it green
would assert something the system does not know.

**The Categorical Exception Rule.** Account slot colors stay fixed hexes while
everything else follows the theme, and that is not an unfinished migration.
There, color *is* identity — the card and the trend line must match — and
deriving eight mutually distinguishable hues from an arbitrary user theme is
not a solvable problem. Same logic for Ledger Green: no positive color exists
in the theme to derive it from.

**The Reinforcement Rule.** No meaning is carried by color alone. The active tab
gets a 2px geometric bar (muted grey against the accent is only 1.47:1); the
default account gets a ★ glyph with an `aria-label`; the Evolución heat cells cap
their mix at 45% so text on them stays legible.

## Typography

**Body Font:** `system-ui` with the Pico stack (`Segoe UI`, Roboto, Helvetica,
Arial, sans-serif). No web font is loaded, on purpose.

**Character:** Whatever the phone already renders, set tightly. The type carries
no brand — the numbers do. The one typographic signature is the tabular figure:
every money value uses `font-variant-numeric: tabular-nums`, so digits align
down a column and a list of amounts can be scanned rather than read.

### Hierarchy

- **KPI** (600, 1.75rem, 1.2): the money inside a card — Gastos / Ingresos /
  Neto and each account balance. 1.75rem is a ceiling, not a taste: the longest
  realistic balance (`$1.234.567,00`, 14 characters) fits a 360px card at this
  size and not at 2rem.
- **Body** (400, 1rem): movement titles, prose, empty-state messages.
- **Table** (400, 0.85rem): the Evolución grid only. At body size, six months
  plus the category column measure 372px and overflow the 335px usable on a
  360px phone.
- **Label** (400, 0.8rem, Margin Grey): the caption above a KPI, the
  `date · Category › Subcategory` line under a movement title, chip text, chart
  legends. Hierarchy here is carried by size *and* weight, not size alone.
- **Micro** (400, 0.75rem, Margin Grey): the explanatory line inside the
  Variación callout, the Evolución legend. The floor — nothing goes smaller.
- **Cursor** (400, 1.5rem): the period header's ‹ › chevrons, and nothing else.
  It sits above Body because these are glyphs, not text — the size is what makes
  a single character readable inside its own 44×44 tap target. It is not a
  heading step and nothing else may borrow it.

### Named Rules

**The Tabular Rule.** Anything that is money wears `.money`. No exceptions,
including inside table cells and inline balance lines.

**The Truncation Rule.** A movement's title and its meta line each truncate to
one line with an ellipsis. A long merchant name may never push the amount off
the right edge — the amount is the reason the row exists.

## Layout

One column, always. Pico's `.grid` collapses to a single column below 768px and
the app is never wider than a phone, so KPI and account cards are full-width and
stack; three figures never crowd into a row. The base rhythm is
`--pico-spacing: 0.75rem`, tightened from Pico's marketing-page default because
this is an operate surface, not a landing page.

Vertical structure is fixed at both ends, and both ends belong to Telegram. A
fixed tab bar sits at the bottom with
`padding-bottom: calc(0.5rem + var(--tg-safe-area-inset-bottom, 0px))`, and the
content area reserves `calc(4rem + var(--tg-safe-area-inset-bottom, 0px))` to
clear it — Telegram draws its own UI in the webview and iOS adds a home
indicator, both reported as CSS variables. Beyond the page itself, `app.js`
paints Telegram's **own** chrome to match: `setHeaderColor` (Bot API 6.1+) and
`setBottomBarColor` (7.10+, behind a feature guard) are both fed the app's page
background read from computed style, so the app is not a differently-colored
rectangle inside the client's frame. `overscroll-behavior: contain` on the
content area and on the Evolución scroller keeps a scroll from chaining out to
the webview.

Above the content, every view opens with the period header: a cursor row
(‹ label ›) over a chip row — period presets, a spacer, then the currency chips.
Rhythm inside components: `0.5rem` vertical for a movement row,
`0.35rem 0.4rem` for an Evolución cell, `0.25rem 0.6rem` for a chip.

### Named Rules

**The 44px Rule.** Anything tappable is at least 44×44: tab links, period
arrows, account cards, chips. `.tappable` (`app.css`) is the one shared rule
for this — `min-width`/`min-height: 44px` plus a centering flex — and is
applied in markup to the account card; `.tab-link`, the period cursor arrows
and `.chip` still carry their own hand-written copies of the same floor
rather than the class, which is how the chip row — the single most-tapped
control in the app — had been missed for as long as the rule lived only by
hand. A card that opens a drill still declares its own floor even when its
content would be shorter.

**The Rows-Not-Tables Rule.** A list of movements is a stack of
`<section class="mov-group">` day groups — each headed by a `GroupTitle`
(`templates/group.templ`) and containing a `<ul class="mov-list">` of flex
rows — not a `<table>`. Four fields per movement in a narrow webview forces
horizontal scrolling as a table; as rows, the text column flexes and
truncates while the amount stays pinned right. The date lives once, on the
group's `GroupTitle`, and not on each row — `GroupRowsByDay`
(`templates/movement_row.go`) splits the list into consecutive same-date runs
before render, so the row's meta line is free to hold only what actually
varies between rows. Evolución is the one real table, and it pays for it with
a horizontal scroller and a sticky first column.

**The Carried-State Rule.** Every view emits `AppState` — a hidden `#app-state`
block of inputs the tab bar reads with `hx-include`. A view that renders without
it silently resets the user's period on the way out, because `periodFromQuery`
never 400s: it falls to defaults, and the filter appears to change by itself. A
view may carry the state without showing the controls (Admin does exactly that,
having nothing to filter), but it may not skip it.

## Elevation & Depth

The system is flat. There is no decorative `box-shadow` in it: depth is carried
by a 1px Hairline border, a tinted card background, and stacking order. The
single shadow in the codebase is structural, not atmospheric — `inset -1px 0` on
the Evolución sticky column, standing in for the border a collapsed table takes
away from a scrolling cell.

Z-order is small and explicit: the tab bar sits at `z-index: 2` and the sticky
table column at `1`, because at equal values the table rows drew over the tabs
while scrolling.

### Named Rules

**The Flat-By-Default Rule.** Surfaces are flat at rest, and a shadow must earn
its place by solving a real layering problem the way the sticky column's inset
does. Whether a genuinely floating layer (a sheet, a modal) may use an ambient
shadow is **undecided** — the app has no such layer yet. Decide it when one
arrives; do not smuggle one in as polish.

## Shapes

Gently squared, near-rectangular. Pico's `0.25rem` radius on cards, buttons and
inputs is used unmodified — small enough to read as a document, large enough not
to look unfinished. The one deviation is the filter chip, which goes fully
rounded (`1rem`) so the period and currency selectors read as a control strip
rather than as small buttons.

Borders do the shaping work: 1px Hairline under every movement row and every
table cell, above the tab bar, and around an unselected chip. The Variación
callout carries none of them on purpose — see the Variación Callout component
below.

Tap feedback is deliberate rather than the mobile-Chrome default flash:
`-webkit-tap-highlight-color: rgba(0, 0, 0, 0.05)` on every link, button and
tab, plus `opacity: 0.7` on an account card's article while pressed. That rgba
is the third fixed literal in the system, and it is defensible: it is a neutral
darkening applied over whatever the theme painted, not a color of its own.

## Components

### Buttons

The app has essentially no conventional button — every action is a link or a
chip, because every action is navigation: htmx swaps `#content` and the URL is
pushed. The one exception is Admin's "Generar invitación", which posts. New work
should keep that shape: a `<button>` here implies a form submit the app does not
have.

### Chips

- **Style:** fully rounded (1rem), `0.25rem 0.6rem` padding, 0.8rem text, laid
  out in a `.chips` flex row with a `.chips-sep` spacer between the period
  presets and the currency pair.
- **Unselected:** Margin Grey text on a Hairline border, transparent fill.
- **Selected:** Theme Button fill and border, Theme Button Text label,
  `aria-current="page"`.
- **Implementation note:** the selected rule sets plain `background-color` and
  `color` rather than Pico custom properties. Pico's real declaration lives in
  its base button rule at specificity (0,1,0); these selectors are (0,2,0) and
  win without `!important` and without depending on stylesheet order.

### Cards / Containers

- **Corner Style:** 0.25rem (Pico default), no shadow, full-width.
- **KPI card:** a Label caption (0.8rem, Margin Grey, `margin-bottom: 0.15rem`)
  over a KPI figure (1.75rem, 600, `margin: 0`). Neto takes Positive Green or
  Negative Red from its status; every other figure stays Statement Ink.
- **Scope note:** the large-figure rule is scoped to `.grid article .money`, not
  `article .money`. Empty states and the expired-session notice are also
  `<article><p>`, and shrinking their text would ruin the one message that has to
  be read.
- **Named exception — `.accounts-total`:** Cuentas' running total sits above the
  account cards, outside the `.grid`, so the scoped rule above does not reach
  it — and it is the figure that governs that whole screen, so it takes the
  same 1.75rem/600 KPI treatment by a direct rule rather than by being inside
  a card. It reads as the screen's own header figure, not as a card that
  escaped the grid.
- **Account card:** an `<a>` wrapping a KPI card — `display: block`,
  `min-height: 44px`, `color: inherit`, no underline. The name carries at most
  one ★ default marker per screen, in the theme accent with an `aria-label`.
  While pressed, the inner `<article>` drops to `opacity: 0.7`.

### Inputs / Fields

One input exists: the account leaf's movement filter. It is a plain Pico text
input with no custom styling, and it filters the already-rendered rows on the
client — nothing round-trips. Below three characters it does nothing, because one
or two letters match half the list and the highlight becomes noise.

### Navigation

- **Style:** a fixed bottom tab bar, `justify-content: space-around`, 1px
  Hairline top border, opaque Paper background, `z-index: 2`, safe-area padding.
- **Default:** Margin Grey label. Pico colors every `<a>` with the primary, so
  this override is what keeps four tabs from all reading as active.
- **Active:** Theme Accent, weight 600, plus a 2px full-width bar positioned at
  `top: -0.5rem` — exactly the bar's own padding, so it lands on the border.
  `text-decoration: none` is restated because Pico underlines `[aria-current]`.
- **Native back button:** Telegram's own `BackButton` shows on every deep screen
  — any URL carrying `category`, `expand` or `account` — and calls
  `history.back()`, which htmx restores because it pushed the URL.
- **Loading:** `#content` drops to `opacity: 0.55` over 0.1s while an htmx
  request is in flight. That is the whole loading vocabulary.

### Movement Row

- **Grouping:** the list is a stack of `<section class="mov-group">`, one per
  day, each opened by `@GroupTitle(group.Date)` (`templates/group.templ`,
  styled `.group-title` — same size and color as a KPI caption, so it reads as
  signage and not as a row) and holding a `<ul class="mov-list">` of that
  day's rows. `GroupRowsByDay` (`templates/movement_row.go`) builds the groups
  from consecutive equal-date runs, not a map — `MovementRow.Date` is
  formatted without a year, so a map would fold "12 ago" 2025 into "12 ago"
  2026, and walking the list preserves the repository's own order, which is
  what lets the account leaf close against its balance top to bottom. Each
  group is its own `<section>` so `app.js`'s filter can hide a whole day at
  once when none of its rows match, rather than leaving a floating date
  header with nothing under it.
- **Character:** three zones per row, one line each, the amount immovable.
  The date is no longer one of the three zones — it moved to the group's
  `GroupTitle` and dropped out of every row.
- **Shape:** flex row, `0.6rem` gap, `0.5rem` vertical padding, 1px Hairline
  bottom border.
- **Structure:** an optional emoji icon (`flex: none`, `aria-hidden`), a flexing
  text column (title at Body, meta at Label, both truncating), and the amount
  (`flex: none`, `white-space: nowrap`, `.money`). The meta line now carries
  only `Category › Subcategory` (or, on the account leaf, "Sin clasificar" for
  an unrouted row) — whatever varies row to row within a day, since the date
  itself is answered once by the group above it.
- **Filtered state:** non-matching rows are hidden with `display: none` — not the
  `hidden` attribute, which loses to `display: flex` — and matches are wrapped in
  `<mark>` in the title and meta only. The amount is excluded from search and
  highlight on purpose: "500" would match dates, amounts, and any description
  containing a number.

### Variación Callout

- **Character:** context, deliberately not a fourth KPI, and deliberately not
  a card. It is the explanation of a balance that moved without a
  transaction behind it — a section footer, not a fourth number competing
  with Neto.
- **Shape:** a two-column grid (label / figure, note spanning both),
  `0.5rem 0 0` padding (top only), no radius, no border, no fill — plain text
  in Margin Grey sitting under the KPI grid it explains. It was previously a
  bordered, tinted card (Card Paper fill, `0.25rem` radius, a 3px left
  Hairline border): that shape read as the most recognizable tell of
  generated UI, and the role it plays — an aside under a group — already has
  a native form in Telegram, its own section footer, which is what it uses
  now.
- **Type:** the figure stays at Label size (`.variacion .money`, weight 600)
  — the large-figure rule is scoped to `.grid article .money` and does not
  reach here — with a 0.75rem (Micro) note beneath.
- **Color:** never a status color, same as before the shape changed. See The
  One Status Rule.

### Evolución Table

- **Character:** the one dense surface, and it earns the density.
- **Shape:** `border-collapse: separate` (a collapsed border belongs to the
  table, so a sticky cell would scroll away from its own border), `width: auto`,
  0.85rem text, right-aligned cells, `0.35rem 0.4rem` padding, inside a
  horizontal `overflow-x: auto` scroller.
- **Sticky column:** row headers stick left at `z-index: 1` with a Paper
  background and an `inset -1px 0` Hairline shadow standing in for the border.
- **Heat:** two steps of one hue against the row's own average —
  `color-mix(in srgb, var(--pico-primary) 22% | 45%, transparent)`. The mix caps
  at 45%; the earlier ramp reached full opacity and dark text on a saturated
  cell failed contrast.
- **Rows:** totals at weight 600; subcategory rows indented `1.25rem`, weight
  400, in Margin Grey.

### Charts

- **Library:** Chart.js, vendored, drawn onto `<canvas data-chart-type>` paired
  with a `templ.JSONScript` data island. Three types only: `bar-grouped`
  (Resumen), horizontal `bar-single` (Categorías), `line-multi` (Cuentas).
- **Color by role, not by hex.** Go sends a *role* (`RoleExpense` /
  `RoleIncome`), never a color; `app.js`'s `colorForRole` resolves it at draw
  time, reading `--pico-primary` from computed style. Expense follows the theme
  accent; income stays Ledger Green. Account datasets are the exception and
  still send `backgroundColor` from the slot palette — see The Categorical
  Exception Rule.
- **Theme:** `Chart.defaults.color` and `borderColor` are read from
  `--pico-color` and `--pico-muted-border-color` at load and re-read on
  Telegram's `themeChanged`, which also re-runs `initCharts` — so a theme switch
  repaints both the chrome and the series.
- **Lines:** `borderColor` is set from each dataset's resolved color and `fill`
  stays false; several filled account areas overlapping is mud.
- **Labels:** `autoSkip` is forced off on the horizontal bar chart. Each bar *is*
  a category there, so no label is optional.
- **Motion:** animation is disabled outright under
  `prefers-reduced-motion: reduce`, in the chart options.

### Empty State

- **Shape:** a plain `<article><p>`, body size, no icon, no illustration.
- **Copy:** names the specific absence in Argentine Spanish ("Todavía no tenés
  cuentas en esta moneda.", "Sin movimientos en este período."), never a generic
  "no data".

### Failure States

Three, each with its own wording, because each has a different way out:

- **401** — "Sesión vencida. Volvé a abrir la app desde el botón del chat."
  `initData` expires after 24h and the way out is reopening from the chat.
- **Any other response error** — "No se pudo cargar. Probá de nuevo en un
  momento."
- **`htmx:sendError`** — "Sin conexión. Probá de nuevo cuando vuelva." A dropped
  connection never produces a response, so it does not fire `responseError`; on
  a phone this is the likeliest failure of the three.

All three replace `#content`. Silence is not an option here: without a message
the loading opacity simply reverts and the tap reads as ignored rather than
failed, which contradicts `PRODUCT.md`'s "Never silent" principle.

## Do's and Don'ts

### Do:

- **Do** reach for a Pico custom property (`--pico-primary`,
  `--pico-muted-color`, `--pico-muted-border-color`, `--pico-background-color`,
  `--pico-card-background-color`) before writing a literal color. They are
  already mapped to the user's Telegram theme, so both themes and every custom
  one work for free.
- **Do** give every `var(--tg-…)` a fallback, always. Outside Telegram there is
  no failure — just a transparent or black element nobody sees.
- **Do** mirror any new theme mapping across all three of Pico's theme contexts.
- **Do** put `.money` on every monetary figure, wherever it renders.
- **Do** give anything tappable a 44×44 minimum and a deliberate pressed state.
- **Do** carry every meaning in a second channel besides color — a glyph, a
  label, or geometry like the active tab's 2px bar.
- **Do** emit `@AppState(p)` from every view, even one with no period controls.
- **Do** write the reason in the CSS when you override Pico. Every existing
  override says what broke; a silent one gets "cleaned up" by the next person.
- **Do** derive a footer total from an aggregate over all rows, never from the
  rows on screen — the list is capped at 50.

### Don't:

- **Don't** write a fixed color for anything but the three documented
  exceptions: account slots, the income series, and the tap-highlight rgba.
- **Don't** send a color from Go to a chart. Send a role and resolve it in
  `app.js`; that is what keeps chart data following the theme.
- **Don't** derive account slot colors from the theme. Color is identity there,
  and eight distinguishable hues cannot be pulled out of an arbitrary theme.
- **Don't** add a web font. `system-ui` is the stack; a webview on a bad
  connection cannot wait for a download to render text that is mostly numbers.
- **Don't** load anything from a CDN or add a build step. Vendor and pin it under
  `static/`, the way htmx 2.0.4, Pico 2.1.1 and Chart.js are.
- **Don't** add a decorative `box-shadow`, gradient, or backdrop blur. Depth is a
  1px border and a background.
- **Don't** color the Variación figure — or any figure other than Neto — with a
  status hue.
- **Don't** turn a movement list into a `<table>`, or take the horizontal
  scroller off Evolución.
- **Don't** use the `hidden` attribute to hide a `.mov-row`; `display: flex`
  beats it.
- **Don't** go below 0.75rem for text, or above 1.75rem for a figure inside a
  card.
