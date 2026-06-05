# Product

## Register

product

## Users

The control panel for a topology-aware MPI scheduler platform (a BMSTU bachelor's thesis project). Three audiences, in priority order:

1. **The thesis committee** — viewing the interface as static screenshots inside the written ВКР, and watching a live walkthrough at the defense. This is the primary audience: the UI has to read as a real, shipped product at a glance, in a frame, without interaction.
2. **The author**, demoing the platform live during the defense — submitting a job, switching the placement algorithm, showing status and a result.
3. **Conceptually, simulation engineers** — who would upload a solver case and run it on the cluster. Real day-to-day usage is not the current goal, but the workflow should be honest, not faked.

Context of use: a desktop browser, deliberate (not high-frequency) actions, technical user who understands MPI/CFD/FEM terms.

## Product Purpose

A web console for a **cloud computing platform** that runs engineering simulations (CFD / FEM) on a Kubernetes cluster. From one screen the user:

- uploads a case and picks the **solver** (OpenFOAM / OpenRadioss / Code_Aster), the **MPI process count**, and the **placement algorithm** (Müller-Merbach QAP / greedy topology-aware / random baseline / default kube-scheduler);
- submits the job and watches it move through its lifecycle (queued → extracting → running → completed/failed);
- inspects results, with an optional 3D visualization of the solved field.

**The platform is the product.** It is a cloud service for running heavy engineering computations, framed and named as such ("Платформа инженерных расчётов"). The topology-aware placement algorithm is one capability inside it, and the thesis's research contribution, surfaced as a normal run setting, not as the product's identity. Success = an interface that reads as a real cloud compute platform worth defending, not a scheduler demo.

## Brand Personality

Modern product SaaS, in the restrained-engineering lane (think Linear, Vercel dashboard, Grafana Cloud — clean, dense-but-calm, confident). Three words: **precise, modern, composed**.

Voice and tone: technical but plain. **UI copy is in Russian** (the defense and the committee are Russian-speaking); solver and algorithm proper names stay in their original form (OpenFOAM, Code_Aster, MUMPS, QAP), established Russian abbreviations are used (МКЭ for FEM, ВГД for CFD). Labels say exactly what happens ("Запустить расчёт", "Мюллер-Мербах — офлайн QAP"); no marketing adjectives, no exclamation, no cute microcopy. The interface earns trust by being exact and quiet, not by being loud.

## Anti-references

- **Default Material / Bootstrap templates** — the current state of this UI (MUI-blue `#1976d2` buttons, `#f5f5f5` body, system font, no design system). The redesign must not read as a stock component-library default.
- **Bright / toy-like UIs** — acid gradients, emoji, bouncy rounded "fun" cards, playful illustration. The subject is engineering computation; the surface should feel serious.
- **Generic AI-SaaS slop** — the hero-metric template (big number + gradient accent), identical icon-card grids, cream/sand body backgrounds, tiny tracked uppercase eyebrows on every block. Modern-SaaS done by reflex looks as templated as Bootstrap; avoid the cliché while keeping the cleanliness.
- **Overloaded enterprise dashboards** — SAP/Jira density with a hundred widgets. This has one core workflow; the layout should respect that.

## Design Principles

- **Credible as a product.** Every screen should read like a shipped tool in a screenshot, not a thesis demo. Composition and hierarchy hold up in a static frame.
- **Clarity over chrome.** The submit → monitor → inspect workflow is always legible; nothing decorative obscures job state.
- **The run is the story, not the scheduler.** The product is a cloud compute platform; the run lifecycle (submit → monitor → inspect) is the spine. The placement algorithm is a first-class run setting and the research contribution, surfaced and legible, but never the headline or the product's name.
- **Calm precision.** Dense technical information (ranks, solver, status, timings) presented without noise. Restraint is the brand; flash is the failure mode.
- **Honest workflow.** Show the real lifecycle and real states (including empty, loading, error) — don't fake a polished happy-path that doesn't exist.

## Accessibility & Inclusion

Target WCAG 2.1 AA. Body text ≥ 4.5:1 contrast (no light-gray-on-tint); large/bold text ≥ 3:1. Job status is never communicated by color alone — always a label plus color. Forms are fully keyboard-navigable with visible focus states. All motion has a `prefers-reduced-motion: reduce` fallback (crossfade or instant). Palette chosen to remain distinguishable under common color-vision deficiencies (don't rely on red/green alone for pass/fail).
