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
  heat-mild: "color-mix(in srgb, var(--pico-color) 8%, transparent)"
  heat-high: "color-mix(in srgb, var(--pico-color) 18%, transparent)"
  focus-ring: "color-mix(in srgb, var(--tg-theme-accent-text-color, var(--tg-theme-link-color, #0172ad)) 50%, transparent)"
  selection: "color-mix(in srgb, var(--pico-primary) 30%, transparent)"
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
  page-total:
    fontFamily: "system-ui, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif"
    fontSize: "1.5rem"
    fontWeight: 600
    lineHeight: 1.2
    fontFeature: "tabular-nums"
  heading:
    fontFamily: "system-ui, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif"
    fontSize: "1.25rem"
    lineHeight: 1.15
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
  data-cell: "0 0.4rem"
  row: "0.5rem"
  base: "0.75rem"
  touch: "44px"
  chip-height: "32px"
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
    padding: "0 0.6rem"
    height: "{spacing.chip-height}"
  chip-active:
    backgroundColor: "{colors.theme-button}"
    textColor: "{colors.theme-button-text}"
    typography: "{typography.label}"
    rounded: "{rounded.pill}"
    padding: "0 0.6rem"
    height: "{spacing.chip-height}"
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
  row-link:
    textColor: "inherit"
    height: "{spacing.touch}"
  data-cell:
    backgroundColor: "transparent"
    textColor: "{colors.statement-ink}"
    typography: "{typography.table}"
    padding: "{spacing.data-cell}"
    height: "{spacing.touch}"
  page-total:
    backgroundColor: "transparent"
    textColor: "{colors.statement-ink}"
    typography: "{typography.page-total}"
  legend-swatch:
    rounded: "{rounded.sm}"
    size: "0.8rem"
  heat-cell-mild:
    backgroundColor: "{colors.heat-mild}"
    textColor: "{colors.statement-ink}"
    typography: "{typography.table}"
    padding: "{spacing.data-cell}"
    height: "{spacing.touch}"
  heat-cell-high:
    backgroundColor: "{colors.heat-high}"
    textColor: "{colors.statement-ink}"
    typography: "{typography.table}"
    padding: "{spacing.data-cell}"
    height: "{spacing.touch}"
  button-retry:
    backgroundColor: "transparent"
    textColor: "{colors.theme-accent}"
    typography: "{typography.body}"
    rounded: "{rounded.sm}"
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
adds a set of scoped corrections, and every one of them exists because
something specific broke on a 360px Telegram webview — a sticky column losing
its border, a chip whose selected state measured 1.32:1, a tab bar sitting
under the iOS home indicator, an inline `<a>` inside a table cell that a
44px rule could not reach. The system is a stack of small, argued repairs, not
a theme. New work continues that discipline: reach for the Pico token before
writing a value, and when you override, say why in the CSS.

There is no admin surface in this app. An invitation-management screen lived
here through 2026-08-25 and was removed outright at the user's request — route,
templates, middleware and tab slot all gone, not merely hidden. Nothing below
describes it; the OOB-into-a-reserved-slot technique it used is recorded once,
in `internal/controller/miniapp/AGENTS.md`, as a pattern to reuse if a future
view needs to depend on the viewer — not as a screen this file still owns.

**Key Characteristics:**

- Telegram-themed: 21 Pico color tokens are remapped onto `--tg-theme-*`, each
  with a fallback, so the app follows the user's client and still works outside
  it. **Every Pico colour the stylesheet touches is in that map** — the focus
  ring and the table hairline were the last two to escape it, and each escape
  looked exactly like a working default until someone with a custom theme
  opened the app.
- Flat and tonal: depth is a 1px hairline border and a card background, never a
  decorative shadow.
- Tabular numerals everywhere money appears, so columns of figures stay
  comparable.
- 44px minimum on everything tappable — tabs, period arrows, account cards,
  back links, table row-links — reached by whichever of two mechanisms fits
  the element (a padded box, or an invisible overlay over a compact one).
- One heading level in the whole app (`#content h1`), used by every drill
  screen and deliberately absent from Resumen.
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
default account gets a ★ glyph with an `aria-label`; the Evolución heat cells are
named in a legend that sits above the table.

**The Tint-From-The-Ink Rule.** A background tint mixes from the **text** colour
(`--pico-color`), never from the accent. The accent is whatever the user's client
hands us: mix a tint from it and the resulting cell can land anywhere, including
right on top of the text's own luminance, with nothing to warn you. Mixing from
the ink makes the tint a fixed fraction of the ink-to-paper distance, so it
always moves the right way — darker on a light theme, lighter on a dark one —
and the contrast that survives is bounded by construction rather than by luck.
This is what the Evolución heat cells do (8% and 18%). Note the asymmetry with
the status colours, which mix *toward* the ink to make a **foreground**: same
token, opposite job, and swapping the two makes contrast worse, not better.

**The Browser-Surfaces Rule.** The parts of the page nobody drew still belong to
the design. Text selection and the caret are themed from the palette
(`{colors.selection}`, accent caret); the focus ring is `{colors.focus-ring}`.
A default-blue selection halo on a green Telegram theme is the cheapest possible
tell that the page was assembled rather than built.

## Typography

**Body Font:** `system-ui` with the Pico stack (`Segoe UI`, Roboto, Helvetica,
Arial, sans-serif). No web font is loaded, on purpose.

**Character:** Whatever the phone already renders, set tightly. The type carries
no brand — the numbers do. The one typographic signature is the tabular figure:
every money value uses `font-variant-numeric: tabular-nums`, so digits align
down a column and a list of amounts can be scanned rather than read.

### Hierarchy

- **KPI** (600, 1.75rem, 1.2): the money inside a card — Gastos / Ingresos /
  Neto and each account balance. The top of the ramp, and a ceiling rather than
  a taste: the longest realistic balance (`$1.234.567,00`, 14 characters) fits a
  360px card at this size and not at 2rem.
- **Page Total** (600, 1.5rem, 1.2): the one figure that answers the screen —
  Categorías' and its drill's `Total`, Cuentas' `Saldo hoy`. It sits a step
  below the KPI because it shares a line with its own label and would crush it;
  the KPI stands alone in its card and keeps the top step.
- **Heading** (1.25rem, 1.15, Pico's default heading weight): `#content h1`,
  the one heading level the app uses. Opens `AccountLeaf`, `SubcategoryDrill`,
  `SubcategoryLeaf` and `Evolución`. It sits **below both money steps** on
  purpose. It used to be 1.75rem, level with the KPI figure and the page total,
  which meant the largest type on screen was simultaneously a label and a
  number and neither dominated — read on a phone as a title shouting over the
  figure it introduces.
- **Body** (400, 1rem): movement titles, prose, empty-state messages.
- **Table** (400, 0.85rem): the Evolución grid and the Categorías table. At
  body size, Evolución's six months plus its category column measure 372px and
  overflow the 335px usable on a 360px phone; Categorías compacts for the same
  reason Pico's own td/th padding gives it.
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

**The Number Wins Rule.** The largest thing on any screen is always a number,
never a label. The ramp encodes it: KPI figure (1.75rem) → page total
(1.5rem) → heading (1.25rem) → body (1rem). A title that ties the figure it
introduces is a title that competes with it, and on a phone the figure loses.
Any new step slots below the money, not beside it.

**The One Heading Rule.** There is exactly one heading level in the app:
`#content h1`, styled at 1.25rem/1.15. Every drill screen (account leaf,
subcategory drill, subcategory leaf, Evolución) opens with it. **Resumen shows
no heading, and that is kept, argued behavior, not a gap** — the tab already
says where you are, and the period header earns that space by saying *when* you
are instead. It does carry `<h1 class="sr-only">Resumen</h1>`: the decision is
that the title should not take vertical space, not that the entry screen should
be unreachable by heading navigation. Do not add an `<h2>` or a second heading
weight anywhere; a screen that needs a visible title uses `<h1>`, a screen that
doesn't hides one rather than going without.

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
rectangle inside the client's frame. The Evolución scroller contains its
overscroll on **one named axis** — `overscroll-behavior-x: contain` — and
nothing else in the app contains any. The unqualified form sets both axes, and
because an explicit `overflow-x: auto` promotes the other axis from `visible`
to `auto`, that turned a horizontal scroller into a vertical scroll container
with nothing to scroll: it swallowed the upward gesture instead of chaining it
to the page, and since the table fills the viewport there was nowhere left to
touch. The content area carried the same declaration and it did nothing at all
— `<main>` has no `overflow`, so it was never a scroll container.

Above the content, every view opens with the period header: a cursor row
(‹ label ›) over a chip row — period presets, a spacer, then the currency chips.
Rhythm inside components: `0.5rem` vertical for a movement row,
`0.35rem 0.4rem` for a table **header** cell, `0` vertical / `0.6rem` horizontal
for a chip. A table **data** cell takes no vertical padding at all — its height
is set on the cell itself (see The 44px Rule).

### Named Rules

**The 44px Rule.** Anything tappable is at least 44×44 — but not every
element gets there the same way, and which mechanism applies depends on what
the element is:

- A block-level target (the account card, the three "← Volver" back links) is
  `.tappable`: `min-width`/`min-height: 44px`, `display: inline-flex`,
  `align-items: center`. It does **not** center its label — `justify-content:
  center` was dropped, because every current consumer reads from the start of
  its own cell or row, and centering them looked like a bug. An element that
  genuinely wants centering (the chip) writes that itself.
- An inline target sitting inside a table cell (a Categorías category-row link,
  an Evolución row link) cannot use `.tappable` at all: an inline `<a>` ignores
  `min-height` outright, which is why these rows used to measure ~32–36px with
  the 44px rule already written and simply unreachable. `.row-link` fixes this
  by being a 44px-tall flex row (`display: flex; align-items: center;
  min-height: 44px; color: inherit;`) rather than a min-height override.
- **The row height belongs to the cell, not to the link.** `.row-link`'s 44px
  floor lived inside a `td`/`th` that also charged `0.35rem` top and bottom, so
  the padding was paid twice and a row measured ~55px against 13.6px type. Data
  cells now carry `height: 44px` (in table layout `height` acts as a minimum)
  with no vertical padding, and the link stretches inside. Header cells keep
  their compact padding and are deliberately *not* 44px — they are not tappable
  and stretching them pushes the table off a phone.
- **A row that is not tappable still takes the pitch.** Only Evolución's
  category rows get an `Href`, so its subcategory rows carry no `.row-link` and
  used to measure ~32px beside a 55px parent. They take 44px anyway: the floor
  the tappable rows impose sets the pitch for the whole list, and a list with
  two pitches reads as broken.
- The chip is neither: `a[role="button"].chip` is a fixed 32px pill —
  Telegram's own segmented-control height — with `line-height: 1` and
  `display: inline-flex; align-items: center; justify-content: center`. Giving
  it `min-height: 44px` directly made it the heaviest element on the busiest
  row and still left the label mis-centered, because `min-height` on an
  `inline-block` (which is what Pico's own `a[role=button]` rule forces) does
  not resize the way a flex box does. The 44px tap target is a separate
  `::after` overlay (`position: absolute; inset: -6px 0`) that grows the hit
  area only vertically, without enlarging the pill or stealing a neighboring
  chip's touch. The selector itself has to be `a[role="button"].chip`, not
  `.chip` alone — Pico's own `a[role=button]{display:inline-block}` rule
  outranks a bare class on specificity, and losing that fight is what silently
  breaks the flex centering.
- `.tab-link` and the period cursor arrows still carry their own hand-written
  copies of the 44px floor rather than any shared class — historically how the
  chip row, the single most-tapped control in the app, was the one missed for
  as long as the rule lived only by hand.

**The Rows-Not-Tables Rule.** A list of movements is a stack of
`<section class="mov-group">` day groups — each headed by a `GroupTitle`
(`templates/group.templ`) and containing a `<ul class="mov-list">` of flex
rows — not a `<table>`. Four fields per movement in a narrow webview forces
horizontal scrolling as a table; as rows, the text column flexes and
truncates while the amount stays pinned right. The date lives once, on the
group's `GroupTitle`, and not on each row — `GroupRowsByDay`
(`templates/movement_row.go`) splits the list into consecutive same-date runs
before render, so the row's meta line is free to hold only what actually
varies between rows. Evolución and Categorías are the two real tables in the
app; only Evolución pays for it with a horizontal scroller and a sticky first
column — Categorías' three columns fit a 360px phone at table-size text
without either.

**The Carried-State Rule.** Every view emits `AppState` — a hidden `#app-state`
block of inputs the tab bar reads with `hx-include`. A view that renders without
it silently resets the user's period on the way out, because `periodFromQuery`
never 400s: it falls to defaults, and the filter appears to change by itself.
`PeriodHeader` is today the rule's only consumer and it always shows the
controls it carries state for — the historical case of a view carrying state
without showing it (the removed Admin screen, which had nothing to filter) is
gone, but the rule is written for the next one: a future controls-less view
must still emit `AppState`, not skip it because there is nothing to show.

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

The app has essentially no conventional button — every action is a link,
because every action is navigation: htmx swaps `#content` and the URL is
pushed. There is no exception left in the app: the one that used to exist (a
`<button>` that posted, on the now-removed admin screen) is gone with it. New
work should keep the current shape by default; a `<button>` implies a form
submit the app does not otherwise have, and should earn its place rather than
be added by habit.

### Chips

- **Style:** a fixed 32px pill (Telegram's own segmented-control height),
  `line-height: 1`, `0.8rem` text, `0 0.6rem` padding, laid out in a `.chips`
  flex row with a `.chips-sep` spacer between the period presets and the
  currency pair. The 44px tap target does not grow the pill — see The 44px
  Rule's `::after` overlay technique.
- **Unselected:** Margin Grey text on a Hairline border, transparent fill.
- **Selected:** Theme Button fill and border, Theme Button Text label,
  `aria-current="page"`.
- **Implementation notes:** the selector is `a[role="button"].chip`, not
  `.chip` alone — Pico's `a[role=button]{display:inline-block}` rule outranks
  a bare class on specificity, and without the qualifier the pill silently
  loses its flex centering. The selected rule itself sets plain
  `background-color` and `color` rather than Pico custom properties, because
  Pico's real declaration lives in its base button rule at specificity
  (0,1,0) and these selectors are (0,2,0) — they win without `!important` and
  without depending on stylesheet order.

### Cards / Containers

- **Corner Style:** 0.25rem (Pico default), no shadow, full-width.
- **KPI card:** a Label caption (0.8rem, Margin Grey, `margin-bottom: 0.15rem`)
  over a KPI figure (1.75rem, 600, `margin: 0`). Neto takes Positive Green or
  Negative Red from its status; every other figure stays Statement Ink.
- **Scope note:** the large-figure rule is scoped to `.grid article .money`, not
  `article .money`. Empty states and the expired-session notice are also
  `<article><p>`, and shrinking their text would ruin the one message that has to
  be read.
- **Page total — `.balance-line.page-total`:** the figure that governs a whole
  screen (Cuentas' `Saldo hoy`, Categorías' and its drill's `Total`) sits above
  the content, outside the `.grid`, so the scoped rule above does not reach it.
  It is **not** a third shape: `.page-total` is a modifier that declares only
  the figure's size (Page Total, 1.5rem), while the shape — `space-between`,
  baseline alignment, weight 600 — comes from `.balance-line`, which the two
  movement leaves already used. Label and figure share one line:
  `<p class="balance-line page-total"><span>Total</span><span class="money">…</span></p>`.
  The inner `.money` is load-bearing, not decoration — it is where the tabular
  figure lives, and the largest number on the screen is the last one that can
  afford to lose it.
- **Account card:** an `<a class="account-card tappable">` wrapping a KPI card
  — `display: block`, `color: inherit`, no underline, its 44×44 floor from
  `.tappable` in markup (see The 44px Rule). The name carries at most one ★
  default marker per screen, in the theme accent with an `aria-label`. While
  pressed, the inner `<article>` drops to `opacity: 0.7`.

### Inputs / Fields

One input exists: the account leaf's movement filter. It is a plain Pico text
input with no custom styling, and it filters the already-rendered rows on the
client — nothing round-trips. Below three characters it does nothing, because one
or two letters match half the list and the highlight becomes noise.

### Navigation

- **Style:** a fixed bottom tab bar, `justify-content: space-around`, 1px
  Hairline top border, opaque Paper background, `z-index: 2`, safe-area padding.
  Four tabs: Resumen, Categorías, Cuentas, Evolución.
- **Default:** Margin Grey label. Pico colors every `<a>` with the primary, so
  this override is what keeps four tabs from all reading as active.
- **Active:** Theme Accent, weight 600, plus a 2px full-width bar positioned at
  `top: -0.5rem` — exactly the bar's own padding, so it lands on the border.
  `text-decoration: none` is restated because Pico underlines `[aria-current]`.
- **Native back button:** Telegram's own `BackButton` shows on every deep screen
  — any URL carrying `category`, `expand` or `account` — and calls
  `history.back()`, which htmx restores because it pushed the URL. The three
  "← Volver" links in-page (account leaf, subcategory drill, subcategory leaf)
  are `.tappable`, left-aligned, and exist alongside the native button rather
  than instead of it.
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

### Category Table

- **Character:** three columns (name, total, share) that fit a 360px phone
  outright — no scroller, no sticky column, unlike Evolución.
- **Shape:** `.cat-table`, `0.85rem` text — the same compaction Evolución
  already carried, applied here for the same reason: Pico's own `td`/`th`
  padding overflows a phone once a table has real content in every cell.
  Header cells take `0.35rem 0.4rem`; data cells take `height: 44px` with
  `0 0.4rem` and no vertical padding (see The 44px Rule).
- **Row link:** each category/subcategory name is a `.row-link` when it drills
  further (the top-level view links to subcategories; the drill itself does
  not link further and renders the name as plain text). `.row-link` exists
  because `.tappable`'s `min-height` does nothing on an inline `<a>` inside a
  table cell — see The 44px Rule.
- **Row header:** the name column is a `<th scope="row">`, not a `<td>`, the same
  as Evolución — without it a screen reader reading a money cell cannot say which
  category it belongs to. It carries `font-weight: inherit` so the semantic
  change stays invisible: a row header is not a heading here, it is a label.
- **Accessible name:** the table itself carries an `aria-label`
  (`Gastos por categoría` / `…por subcategoría`), because the Categorías index
  has no `<h1>` above it to borrow one from.

### Evolución Table

- **Character:** the one dense surface, and it earns the density.
- **Heading placement:** `<h1>Gasto por categoría</h1>` sits **outside**
  `.evolution-scroll`, above the scroller — the title used to scroll away
  sideways with the table when it lived inside it. It does not repeat the
  period label; that already lives in the period header above, and it no longer
  carries the scale either — that moved into the legend.
- **Legend:** one Label-size muted line **above** the table, never below it. It
  explains what you are about to read, so putting it under the table meant
  scrolling past the shading you could not interpret in order to find out what
  it meant. It carries two real `.legend-swatch` chips (0.8rem square, `sm`
  radius, plus a 1px inset Hairline ring so a neutral tint is still visible as a
  standalone square) filled by `.cell-mild` and `.cell-high` themselves, so the
  legend cannot drift from the table it explains — the heat has two steps and a legend
  that names one leaves half the shading unexplained. The scale segment
  (`en miles de $`) is composed here with its own separator and **omitted
  entirely for USD**, which is not scaled; `ScaleLabel` returns the bare label
  precisely so the template, not the string, owns the punctuation.
- **Shape:** `border-collapse: separate` (a collapsed border belongs to the
  table, so a sticky cell would scroll away from its own border), `width: auto`,
  0.85rem text, right-aligned cells, inside a horizontal `overflow-x: auto`
  scroller. Header cells take `0.35rem 0.4rem`; data cells take `height: 44px`
  with `0 0.4rem`.
- **Sticky column:** row headers stick left at `z-index: 1` with a Paper
  background and an `inset -1px 0` Hairline shadow standing in for the border.
- **Row link:** a category row that expands to subcategories is a `.row-link`
  — same reasoning as the Category Table above.
- **Heat:** two neutral steps against the row's own average — `{colors.heat-mild}`
  and `{colors.heat-high}`, both mixed from the ink per The Tint-From-The-Ink
  Rule. The shading is deliberately **not** tinted with the accent: it used to
  be (22% and 45% of `--pico-primary`), and because the accent is arbitrary per
  user, the text sitting on a shaded cell had no guaranteed contrast. Two things
  do not fix this and both look like they would — mixing toward `--pico-color`
  the way the status colours do makes a *foreground*, and mixing against the page
  background produces the identical pixel that `transparent` already composites
  to. Losing the hue is the price; the accent still leads everywhere it carries
  meaning (links, chips, the active tab), and the legend above the table names
  both steps in words.
- **Rows:** totals at weight 600; subcategory rows indented `1.25rem`, weight
  400, in Margin Grey — and at the same 44px pitch as their parents, even
  though they are not tappable. The indent survives the cell-height rule
  because `.evolution-sub th[scope="row"]` is (0,2,1) and outranks
  `.evolution tbody th` at (0,1,2).

### Charts

- **Library:** Chart.js, vendored, drawn onto `<canvas data-chart-type>` paired
  with a `templ.JSONScript` data island. Three types: `bar-grouped` (Resumen),
  horizontal `bar-single` (Categorías index and its subcategory drill),
  `line-multi` (Cuentas) — four canvases across the app.
- **Sizing:** every `<canvas data-chart-type>` is wrapped in a `.chart-box`
  div (`position: relative; height: 220px`), because Chart.js's
  `responsive: true` overwrites the canvas's own width/height attributes on
  resize — aspect control has to come from a sized parent, and without it no
  chart in the app had a stable height. All four chart configs set
  `maintainAspectRatio: false` to let that box govern them. The two
  horizontal bar charts (Categorías index and its subcategory drill) get a
  dynamically computed box height in `app.js`
  (`Math.max(220, labels.length * 28 + 48)` px), because a fixed 220px box
  squeezes every bar down to a sliver once there are more than a handful of
  categories — each bar *is* a category there, at 28px plus room for the axis.
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
- **Labels:** `autoSkip` is forced off on the horizontal bar charts. Each bar
  *is* a category there, so no label is optional.
- **Motion:** animation is disabled outright under
  `prefers-reduced-motion: reduce`, in the chart options.
- **Loaded on demand.** Chart.js is not in the shell. `app.js` injects it from
  `body[data-chart-src]` the first time a view actually contains a
  `canvas[data-chart-type]`, memoising the promise. Evolución has no chart and
  used to pay 204 KB for it anyway. If the injection fails the `.chart-box` is
  hidden rather than left as an empty 220px hole.
- **Two kinds of canvas, named differently.** A chart that repeats a table
  already on screen (both `bar-single` charts, whose bars *are* the rows of the
  table under them) is `aria-hidden="true"` — announcing it would read the same
  figures twice. A chart that is the **only** representation of its data
  (Resumen's `bar-grouped`, Cuentas' `line-multi`) is `role="img"` with an
  `aria-label` built in Go by `TrendChartAlt`, which describes each series by its
  *shape* — first value, last value, peak and where it fell — rather than
  enumerating every bucket. A month view has up to 31 buckets; a literal
  enumeration is 62 numbers in one label, which is worse than silence.

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

**The Way Out Is In The Error Rule.** An error carries its own recovery. The two
transient failures ship a **Reintentar** button that re-fires the current route
(`htmx.ajax` against `location.pathname + location.search`); the 401 deliberately
does **not**, because retrying cannot mint a new `initData` and a button that
cannot work is worse than no button — its copy is the way out instead. Before
this, a failed load was a dead end whose only exit was closing the app and
reopening it from the chat.

All three also announce themselves through the route status region. They arrive
by direct `innerHTML`, which never fires `htmx:afterSwap`, so an error was
inaudible even after the swap announcements existed — the one path where being
silent mattered most.

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
- **Do** give anything tappable a 44×44 minimum — via `.tappable` for a
  block-level element, `.row-link` for an inline `<a>` inside a table cell, or
  an `::after` overlay for a compact control like the chip — and a deliberate
  pressed state.
- **Do** use `#content h1` for a drill screen's title, and nothing else as a
  heading. Resumen's stays `.sr-only` — see The One Heading Rule.
- **Do** finish the non-visual half of every screen you add: a name for any
  table or `role="group"`, a text alternative for any canvas that is the only
  copy of its data, `aria-hidden` on anything purely decorative, and a `<th
  scope="row">` on the column that identifies the row. This app's visual
  accessibility was good long before its semantics were, which is exactly how
  the gap stayed invisible.
- **Do** announce anything that replaces `#content` through the route status
  region. htmx swaps do it via `htmx:afterSwap`; anything writing `innerHTML`
  directly has to call `announce()` itself.
- **Do** carry every meaning in a second channel besides color — a glyph, a
  label, or geometry like the active tab's 2px bar.
- **Do** emit `@AppState(p)` from every view, even one with no period controls.
- **Do** record the reason for a Pico override **here**, not in the CSS. This
  package's code carries no comments — the code is the source of truth for
  behaviour, and this file is the source of truth for why the visual system is
  what it is. An override whose reason is written nowhere gets "cleaned up" by
  the next person, so an override and a line in this document ship together.
- **Do** keep the largest thing on any screen a number. See The Number Wins Rule.
- **Do** set a data row's height on the cell and let the link stretch inside it.
  Charging the touch floor on the link *and* padding on the cell pays for it
  twice.
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
- **Don't** give `.tappable` a `justify-content: center`; its current
  consumers all read left-aligned from the start of their row or cell. An
  element that wants centering states that itself, the way the chip does.
- **Don't** use `min-height` to make an inline `<a>` inside a table cell
  tappable — it does nothing there. Use `.row-link`.
- **Don't** go below 0.75rem for text, or above 1.75rem for a figure inside a
  card. A heading tops out at 1.25rem — below both money steps, on purpose.
- **Don't** tint a background from the accent. Mix it from `--pico-color` — see
  The Tint-From-The-Ink Rule.
- **Don't** leave a Pico colour out of the theme map. The test in
  `theme_test.go` is the guard; adding a token to the stylesheet without adding
  it there is how the focus ring and the table hairline both got missed.
- **Don't** put a script in the shell that only one view needs. Load it from a
  `data-` attribute the first time a view actually contains the element it
  draws, the way Chart.js is loaded.
- **Don't** reference a static asset without the `?v=` build hash. An
  unversioned URL is served `no-cache` on purpose, and the versioned one is
  `immutable` — a long cache without the hash is how a webview keeps a stale
  stylesheet forever.
- **Don't** write `overscroll-behavior` without an axis. Unqualified it sets
  both, and paired with an explicit `overflow-x: auto` it turns a horizontal
  scroller into a vertical scroll container that swallows the page's scroll.
- **Don't** add a comment to this package's code. Behaviour belongs in the code
  and in a test's name; the reasoning belongs in this file.
- **Don't** print a page total as a label above a figure. It is one line —
  `.balance-line page-total`, label left, figure right, `.money` on the figure.
- **Don't** let a legend sit below what it explains, or name fewer steps than
  the thing actually has.
