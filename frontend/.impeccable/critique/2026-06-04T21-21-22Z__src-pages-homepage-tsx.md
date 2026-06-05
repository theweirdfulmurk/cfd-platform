---
target: главный экран
total_score: 29
p0_count: 0
p1_count: 2
timestamp: 2026-06-04T21-21-22Z
slug: src-pages-homepage-tsx
---
# Critique: Main screen (Topology Scheduler Console)

## Design Health Score: 29/40 (Good)

| # | Heuristic | Score | Key Issue |
|---|-----------|:---:|---|
| 1 | Visibility of System Status | 3 | New row appears with up-to-5s lag after submit (no optimistic insert) |
| 2 | Match System / Real World | 3 | Domain jargon (np, QAP) unexplained for non-MPI viewers |
| 3 | User Control & Freedom | 3 | No undo on delete; no deselect; native confirm() |
| 4 | Consistency & Standards | 4 | Unified button/field/pill/mono vocabulary |
| 5 | Error Prevention | 3 | accept/min/max/required + confirm; no domain guards |
| 6 | Recognition vs Recall | 3 | Visible/labelled; delete icon is hover-reveal |
| 7 | Flexibility & Efficiency | 2 | No shortcuts, no bulk, no filter/sort of the list |
| 8 | Aesthetic & Minimalist | 4 | Restrained, single accent, no noise (strongest) |
| 9 | Error Recovery | 2 | Delete failure swallowed silently; demo fallback masks real backend errors |
| 10 | Help & Documentation | 2 | No inline algorithm explanation, no tooltips/help |

## Anti-Patterns Verdict
Detector clean ([], 0 findings). Not obviously AI: dark slate + amber (not the cliche terminal green/cyan) + warm/cool discipline + mono data spine. Second-order risk: "dev-tool that is not SaaS-cream -> dark dashboard" lane; mitigated by amber-as-only-signal and instrument framing. Browser overlay unavailable (no browser automation); review is code + detector.

## What's Working
- Accent discipline: one amber on action/alive only; algorithm tag made neutral so the signal stays meaningful.
- Status system: dot + word, pulse, grayscale- and color-vision-safe.
- State coverage: skeleton / empty / error / demo all designed in the first pass.

## Priority Issues
- [P1] Silent delete failure: handleDelete catch {} gives no feedback; a failed delete looks like success. Fix: shared notifier -> error toast.
- [P1] List does not scale: no filter/sort/bulk; the thesis runs 3 solvers x 3 algorithms x repeats = dozens of rows. Fix: sort + filter by solver/algorithm/status.
- [P2] Demo fallback masks real backend errors: any list() failure silently shows demo data. Fix: demo only when explicitly offline, else error state.
- [P2] Native confirm()/alert on delete clashes with the system. Fix: inline confirm or styled <dialog>.
- [P3] Thin help: algorithms unexplained; a tooltip on Placement algorithm would help non-MPI viewers (committee).

## Persona Red Flags
- Alex (power user, 72 runs): no submit shortcut beyond Enter; per-row confirm(); no filter/sort to find a specific run among dozens.
- Sam (a11y): good overall (amber dark-text ~5:1, muted >=4.5:1, status not color-only, focus rings, keyboard-selectable rows, reduced-motion). Nit: metric-k "np" in faint (~3.8:1) is below 4.5:1 for text.
- Committee/first-timer: jargon (np, QAP, Müller-Merbach) without inline explanation.

## Minor Observations
- Up-to-5s lag before a submitted row appears in real mode (no optimistic insert).
- Unused fileInput ref (dead code).
- Google Fonts dependency (offline -> system fallback).
- Algorithm tag hidden at very narrow viewport (intentional).

## Questions to Consider
- Does the list need filter/sort now (dozens of runs) or stay minimal for screenshots?
- Is the dark theme final, or is a light variant wanted for print embedding in the thesis?
- Should the algorithm choice get a one-line explainer for non-expert viewers?
