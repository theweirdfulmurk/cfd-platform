---
name: Платформа инженерных расчётов
description: Console for a cloud engineering-simulation platform — near-black canvas, single lime signal, frosted-glass panels. UI copy in Russian.
colors:
  slate-canvas: "oklch(0.155 0.004 250)"
  slate-panel: "oklch(0.195 0.005 250)"
  slate-raised: "oklch(0.235 0.006 250)"
  slate-border: "oklch(0.275 0.006 250)"
  slate-border-strong: "oklch(0.340 0.006 250)"
  ink: "oklch(0.970 0.003 250)"
  muted: "oklch(0.700 0.010 250)"
  faint: "oklch(0.520 0.010 250)"
  lime: "oklch(0.860 0.180 130)"
  lime-bright: "oklch(0.910 0.160 128)"
  lime-deep: "oklch(0.800 0.185 132)"
  lime-tint: "oklch(0.300 0.060 132)"
  on-lime: "oklch(0.160 0.012 250)"
  signal-cyan: "oklch(0.740 0.100 224)"
  signal-green: "oklch(0.740 0.140 158)"
  signal-red: "oklch(0.640 0.200 25)"
  glass-panel: "oklch(0.205 0.006 250 / 0.6)"
  glass-border: "oklch(0.985 0.012 250 / 0.1)"
typography:
  display:
    fontFamily: "Inter, system-ui, sans-serif"
    fontSize: "1.5rem"
    fontWeight: 600
    lineHeight: 1.2
    letterSpacing: "-0.01em"
  headline:
    fontFamily: "Inter, system-ui, sans-serif"
    fontSize: "1.125rem"
    fontWeight: 600
    lineHeight: 1.3
    letterSpacing: "-0.005em"
  title:
    fontFamily: "Inter, system-ui, sans-serif"
    fontSize: "1rem"
    fontWeight: 600
    lineHeight: 1.35
  body:
    fontFamily: "Inter, system-ui, sans-serif"
    fontSize: "0.9375rem"
    fontWeight: 400
    lineHeight: 1.55
  label:
    fontFamily: "Inter, system-ui, sans-serif"
    fontSize: "0.8125rem"
    fontWeight: 560
    lineHeight: 1.3
    letterSpacing: "0.005em"
  mono:
    fontFamily: "\"JetBrains Mono\", ui-monospace, monospace"
    fontSize: "0.875rem"
    fontWeight: 450
    lineHeight: 1.4
    fontFeature: "\"tnum\" 1"
rounded:
  sm: "6px"
  md: "10px"
  lg: "14px"
  full: "999px"
spacing:
  xs: "4px"
  sm: "8px"
  md: "12px"
  lg: "16px"
  xl: "24px"
  xxl: "32px"
  xxxl: "48px"
components:
  button-primary:
    backgroundColor: "{colors.lime-deep}"
    textColor: "{colors.on-lime}"
    rounded: "{rounded.sm}"
    padding: "10px 16px"
  button-primary-hover:
    backgroundColor: "{colors.lime}"
    textColor: "{colors.on-lime}"
  button-secondary:
    backgroundColor: "{colors.slate-raised}"
    textColor: "{colors.ink}"
    rounded: "{rounded.sm}"
    padding: "9px 16px"
  input:
    backgroundColor: "{colors.slate-canvas}"
    textColor: "{colors.ink}"
    rounded: "{rounded.sm}"
    padding: "8px 12px"
  panel:
    backgroundColor: "{colors.glass-panel}"
    rounded: "{rounded.lg}"
    padding: "24px"
  badge-running:
    backgroundColor: "{colors.lime-tint}"
    textColor: "{colors.lime-bright}"
    rounded: "{rounded.full}"
    padding: "3px 10px"
  list-row-selected:
    backgroundColor: "{colors.lime-tint}"
    textColor: "{colors.ink}"
    rounded: "{rounded.md}"
---

# Design System: Платформа инженерных расчётов

> Console for a cloud computing platform that runs engineering simulations (CFD/FEM) on a Kubernetes cluster. UI copy is in Russian; this spec is in English for tooling. The topology-aware scheduler is one capability, not the product's identity.

## 1. Overview

**Creative North Star: "The Control Console"**

A dark instrument panel for launching, watching, and inspecting engineering simulations, not a marketing surface and not a generic SaaS dashboard. The interface sits on a near-black neutral canvas, calm and low-glare, the way a real monitoring console reads in a dimmed room. Against that quiet field, one warm **lime/chartreuse** is the only brand color: it marks what you can act on (the submit button, the focused field, the toggle) and what is alive (a running job). Nothing else competes for that lime. The two work panels float as **frosted-glass cards** over the canvas; depth elsewhere comes from layering tones of slate, not from drop shadows. Type is a single neutral sans with a monospace companion for the numbers that matter (ranks, durations, field ranges, IDs).

The system rejects the stock looks named in the brief. It is not a default Material/Bootstrap surface (no `#1976d2` stock blue, no stock component shapes). It is not bright or toy-like (no acid gradients, no emoji, no bouncy "fun" cards). And it is not generic AI-SaaS slop (no hero-metric block, no identical icon-card grid, no cream background, no tracked-uppercase eyebrow over every section). The lime carries the energy; the surface stays a disciplined near-black.

The feeling to hit is an engineer trusting an instrument: precise, modern, composed. If a screenshot could be mistaken for a stock admin template or a colorful startup landing, it has failed.

**Key Characteristics:**
- Near-black neutral canvas; the two work panels are frosted glass, everything else flat tonal layers.
- A single lime signal, reserved for action and live state (≤10% of any screen).
- One sans (Inter) for everything; monospace for numerals and identifiers only.
- Warm/cool counterpoint: lime acts, cyan informs. They never blur.
- Calm density: a lot of precise information, no visual noise. UI copy in Russian.

## 2. Colors

A near-black neutral ramp carries the surface; a single lime is the brand; cyan, green, and red speak only as status.

### Primary
- **Signal Lime** (`oklch(0.860 0.180 130)`): the one brand color. The running-state indicator, current selection, focus rings, the active toggle, small accents. Bright resting lime for dots and rings.
- **Lime Deep** (`oklch(0.800 0.185 132)`): the filled-button lime. Slightly deeper so the near-black label clears contrast; also the pressed/active state.
- **Lime Bright** (`oklch(0.910 0.160 128)`): hover lift and the text color inside a lime-tinted badge.
- **Lime Tint** (`oklch(0.300 0.060 132)`): a lime-stained dark surface for the selected list row and the running-status badge. Carries lime identity without a fill.
- **On-Lime** (`oklch(0.160 0.012 250)`): the near-black used as text/icon on any lime fill. Lime is a light color; dark text on it reads cleanly where white would not.

### Secondary
- **Signal Cyan** (`oklch(0.740 0.100 224)`): the cool counterpoint. Links, info, and the queued/extracting pre-run states. Clearly distinct from lime (warm action vs. cool information); never a primary action.

### Tertiary (status only)
- **Signal Green** (`oklch(0.740 0.140 158)`): completed / success. A clearly cooler green than the yellow-green lime, so "running" (lime) and "completed" (green) never read as the same color.
- **Signal Red** (`oklch(0.640 0.200 25)`): failed / destructive.

### Neutral
- **Slate Canvas** (`oklch(0.155 0.004 250)`): the app background. Near-black with a whisper of cool, softer than pure `#000`.
- **Slate Panel** (`oklch(0.195 0.005 250)`): the opaque step used inside glass (inputs, raised rows).
- **Slate Raised** (`oklch(0.235 0.006 250)`): inputs/rows on hover, secondary buttons, chips.
- **Slate Border** (`oklch(0.275 0.006 250)`): hairline dividers and edges.
- **Slate Border Strong** (`oklch(0.340 0.006 250)`): input borders and stronger separators.
- **Ink** (`oklch(0.970 0.003 250)`): primary text, near-white with a faint cool cast.
- **Muted** (`oklch(0.700 0.010 250)`): secondary text, labels, metadata, placeholders. The floor for any real text.
- **Faint** (`oklch(0.520 0.010 250)`): disabled text and decorative marks only. Never body copy.

### Glass
- **Glass Panel** (`oklch(0.205 0.006 250 / 0.6)`) + `backdrop-filter: blur(22px) saturate(140%)`: the two work panels, the modal, and translucent overlays. Frosted, not transparent.
- **Glass Border** (`oklch(0.985 0.012 250 / 0.1)`): the faint light hairline that gives glass its edge.

### Named Rules
**The One Signal Rule.** Lime appears on at most ~10% of any screen, and only on something actionable or alive. The moment lime decorates a static element, it stops meaning "here," and the console loses its single most valuable cue.

**The Warm/Cool Rule.** Lime acts; cyan informs. A control the user can trigger is lime; a fact the system reports is cyan. They never trade roles.

**The No-Cream Rule.** The surface is a cool near-black, full stop. No warm-neutral background, no paper/sand/parchment tint. Energy lives only in the lime.

## 3. Typography

**Display / Body Font:** Inter (with system-ui, sans-serif)
**Data / Mono Font:** JetBrains Mono (with ui-monospace, monospace)

**Character:** One disciplined neutral sans does all the talking; a monospace handles the numbers and identifiers an engineer scans (ranks, np, durations, field min/max, IDs), set with tabular figures so columns line up. Functional contrast, not decorative: prose vs. data.

### Hierarchy
Fixed rem scale (no fluid clamp; product UI at consistent DPI), ratio ~1.2.
- **Display** (600, 1.5rem/24px, -0.01em): the app title "Платформа инженерных расчётов". Modest; a console, not a hero.
- **Headline** (600, 1.125rem/18px): panel headings ("Новый расчёт", "Расчёты"), modal title.
- **Title** (600, 1rem/16px): a simulation name in a row.
- **Body** (400, 0.9375rem/15px, 1.55): descriptive/helper copy. Cap prose at 65–75ch.
- **Label** (560, 0.8125rem/13px, +0.005em): field labels, toggle name, segment buttons. Sentence case, not all-caps.
- **Mono** (450, 0.875rem/14px, tabular-nums): np, timings, IDs, the field ranges on the legend. The numeric spine.

### Named Rules
**The Mono-for-Data Rule.** Anything compared across rows (counts, durations, IDs, field ranges) is monospace with tabular figures. Anything read as language is Inter.

**The Quiet-Header Rule.** The page title never exceeds 1.5rem. Authority comes from precision, not a shouting hero heading.

## 4. Elevation

A hybrid: the two work panels and the modal are **frosted glass** that float over the canvas (`glass-panel` + blur + `glass-border` + a soft float shadow); everything inside them is **flat and tonally layered** (canvas → panel → raised + 1px borders), no inner shadows. On a dark UI, stacked shadows muddy fast; tonal lift stays crisp. Beyond the floating glass shells, shadow appears only for true overlays (dropdowns, toasts, the modal).

### Shadow Vocabulary
- **Float** (`box-shadow: 0 18px 48px -20px rgba(0,0,0,0.75)`): the glass work panels and modal, so they lift off the canvas.
- **Overlay** (`box-shadow: 0 8px 28px -6px rgba(0,0,0,0.6)`): dropdown menus, toast stack.
- **Focus ring** (`box-shadow: 0 0 0 3px oklch(0.860 0.180 130 / 0.38)`): the lime focus halo on inputs, selects, buttons, toggle. State, not elevation, but the same channel.

### Named Rules
**The Glass-Shell, Flat-Inside Rule.** Glass is for the panel/modal shell only. Inside a panel, separate things with a tonal step or a 1px border, never another shadow or another pane of glass. Nested glass is always wrong.

## 5. Components

### Buttons
- **Shape:** gently squared (6px radius). Pills are for status only, never actions.
- **Primary:** `lime-deep` fill, `on-lime` (near-black) label, padding 10px 16px, weight 560. The signature control: a lime key with a dark legend. White text is forbidden here (it fails contrast on lime).
- **Hover / Focus:** hover lifts the fill to `lime` and the button up 1px, 160ms ease-out. Focus shows the lime ring. Active presses back to `lime-deep`.
- **Secondary:** `slate-raised` fill, `ink` label, 1px `slate-border-strong`. For "Скачать результаты", "Повторить", "Обновить".
- **Ghost / icon:** transparent, `muted` icon, hover fills `slate-raised`. Row delete, search-clear, modal close.
- **Danger:** `signal-red` (the inline delete confirm "Удалить" fills red with near-white text).

### Toggle (switch)
- A 40×23px pill switch. Off: `slate-raised` track, `ink` knob. On: `lime-deep` track, `on-lime` knob slid right. `role="switch"`, lime focus ring. Used for "Параллелизация вычислений", which progressively reveals the process count and load-distribution mode.

### Chips / Status pills
- **Style:** fully rounded, 3px 10px, `label` type, a leading 8px dot plus a word. Status is never color alone, always dot + label — survives a grayscale screenshot and color-vision deficiency.
- **Running:** `lime-tint` bg, `lime-bright` text, lime dot pulsing (1.6s, opacity 1↔0.45). Reduced-motion: static.
- **Queued (В очереди):** `signal-cyan`. **Completed (Завершён):** `signal-green`. **Failed (Ошибка):** `signal-red`.
- Pills live in a fixed-width column so the names beside them line up across rows.

### Panels / Modal (glass)
- **Work panels:** `glass-panel` + blur + 1px `glass-border` + float shadow, 14px radius. The run form (left) and simulation list (right).
- **Modal:** same glass over a blurred dark backdrop (`oklch(0.1 0.004 250 / 0.6)` + blur 6px). Closes on Esc / backdrop click / ×. Used for the result viewer. Body scroll locks while open.
- **Internal padding:** 16–24px.

### Inputs / Fields
- **Style:** `slate-canvas` fill (recessed, darker than the glass panel around it), 1px `slate-border-strong`, 6px radius, `ink` text, `muted` placeholder.
- **Focus:** border → `lime`, plus the 3px lime ring.
- **Select:** custom caret; open menu items on `slate-raised`.
- **File / dropzone:** dashed 1px border, hover/drag border → `lime`, chosen filename in `mono`.
- **Error / Disabled:** `signal-red` border + helper for errors; `faint` text for disabled.

### Header
- A slim top bar on `slate-canvas`, 1px bottom `slate-border`, slightly translucent + blur. Lime diamond mark + "Платформа инженерных расчётов" (`display`), and a live cluster indicator on the right (green pulsing dot + "кластер активен"). No side nav; the two-pane body is the whole app.

### Simulation row (signature component)
One job per row: fixed-width status pill, then the name (`title`) with `solver · mode` as a `muted` sub-line, then `mono` data (MPI count, time). Completed rows get a lime "Результат" action that opens the result modal; delete shows an inline `Удалить? [Удалить] [Отмена]` confirm (never a native `confirm()`). Selected row is marked by the `lime-tint` background alone (no gutter dot, never a left border stripe). Hover raises the row half a tonal step.

### Result viewer (signature component)
A vtk.js (WebGL) viewport inside the modal renders the result's scalar field on a 3D surface, interactive (rotate/zoom). A field selector (top-left, glass) switches the displayed field; a cool-to-warm color legend (right) shows the real min/max numbers and unit; a "Скачать результаты" secondary button exports the data. Demo renders a synthetic field; with a backend the same viewer loads the run's exported `.vtp` (field names + ranges come from `getArrays()` / `getRange()`).

### Empty / Loading / Error states
- **Empty:** centered, teaches the next action ("Расчётов пока нет" + help pointing at "Новый расчёт").
- **Loading:** skeleton rows (shimmering `slate-raised`), never a centered spinner inside content.
- **Connection lost:** a `signal-red` banner ("Связь с кластером потеряна. Повтор каждые 5 с." + Повторить) that keeps the last data; demo-data banner is cyan. The two are distinct: demo = never connected, error = was connected then dropped.

## 6. Do's and Don'ts

### Do:
- **Do** keep lime to ≤10% of any screen, only on actionable or live elements (the One Signal Rule).
- **Do** put dark `on-lime` text on every lime fill; lime is light, dark text is the legible choice.
- **Do** convey status with a dot plus a word, never color alone.
- **Do** set every count, duration, ID, and field range in JetBrains Mono with tabular figures.
- **Do** use glass only for the panel/modal shell; keep everything inside flat and tonal.
- **Do** ship every interactive component with default, hover, focus-visible, active, disabled, and (where it applies) loading and error states.
- **Do** give every animation a `prefers-reduced-motion: reduce` fallback (the running-dot pulse becomes static).
- **Do** write UI copy in Russian; keep solver/tech proper names (OpenFOAM, Code_Aster, MPI) as-is; present the placement algorithm by its user-facing mode (Точный / Сбалансированный / Быстрый / Стандартный), not by "Müller-Merbach / QAP".

### Don't:
- **Don't** reintroduce the default Material/Bootstrap look: no `#1976d2` stock blue, no stock-template buttons or `#f5f5f5` body.
- **Don't** go bright or toy-like: no acid gradients, no emoji, no bouncy rounded "fun" cards.
- **Don't** ship AI-SaaS slop: no hero-metric block, no identical icon-card grid, no cream/sand background, no tracked-uppercase eyebrow over every section.
- **Don't** use gradient text (`background-clip: text`).
- **Don't** nest glass inside glass, or apply glass to small inner elements; it is a shell material, used sparingly and purposefully, never decoration.
- **Don't** use a colored `border-left`/`border-right` greater than 1px as a stripe accent; mark selection with the `lime-tint` fill.
- **Don't** let lime and cyan trade roles; lime acts, cyan informs.
- **Don't** use white text on lime, or `faint` for any real body text (both fail contrast).
- **Don't** add drop shadows to flat inner surfaces, or a centered spinner where a skeleton belongs.
