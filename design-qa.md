# Lumi Design QA History

## Project list — previous completed QA

## Comparison target

- Source visual truth: `https://zettos-prd-a1-prototypes.vercel.app/lumi-picture-book/home.html`.
- Desktop source capture: `/Users/qingyang/.codex/visualizations/2026/08/29/01a04d5c-1d82-7712-af0a-d69ecf3c1eec/lumi-reference-home-list-desktop.png`.
- Desktop implementation capture: `/Users/qingyang/.codex/visualizations/2026/08/29/01a04d5c-1d82-7712-af0a-d69ecf3c1eec/lumi-implementation-home-list-desktop.png`.
- Desktop combined comparison: `/Users/qingyang/.codex/visualizations/2026/08/29/01a04d5c-1d82-7712-af0a-d69ecf3c1eec/lumi-home-list-desktop-comparison.png`.
- Mobile source capture: `/Users/qingyang/.codex/visualizations/2026/08/29/01a04d5c-1d82-7712-af0a-d69ecf3c1eec/lumi-reference-home-list-mobile-exact.png`.
- Mobile implementation capture: `/Users/qingyang/.codex/visualizations/2026/08/29/01a04d5c-1d82-7712-af0a-d69ecf3c1eec/lumi-implementation-home-list-mobile.png`.
- Mobile combined comparison: `/Users/qingyang/.codex/visualizations/2026/08/29/01a04d5c-1d82-7712-af0a-d69ecf3c1eec/lumi-home-list-mobile-comparison.png`.
- Viewports: 1728 × 992 CSS px and 390 × 844 CSS px. Source and implementation captures use matching dimensions and device scale.

## Required fidelity surfaces

- Shell: passed. Desktop sidebar is 224px, topbar is 58px, and the mobile navigation collapses behind the target menu control.
- Main layout: passed. Desktop home content is 880px with a 784px inner column. Mobile content uses 20px page gutters and a 350px inner width.
- Typography: passed. Kicker, 32px/26px headline, recent-project heading, project name, and relative edit time match the reference hierarchy and line heights.
- Composer: passed. The desktop and mobile form positions, 150.5px height, 92px textarea, 51px footer, context tag, attachment control, and send button match the source geometry.
- Project cards: passed. Desktop uses three 253.33 × 166px cards with 12px gaps and 94px covers. Mobile uses 350 × 142px cards with 70px covers.
- Media: passed. Every recent project resolves its earliest current comic image through the read-only recent-project cover endpoint. Projects without an eligible image keep the Lumi fallback without exposing internal file paths or IDs.
- Responsive behavior: passed at both requested comparison sizes. No clipping, horizontal overflow, or broken fixed-height regions were observed.
- Interaction and accessibility: passed. Mobile navigation opens and closes; the new-picture-book dialog opens and dismisses; composer input enables the send action; card overflow menus open and dismiss; controls retain accessible names and keyboard semantics.

## Comparison history

1. Initial desktop comparison found a tinted main canvas, a 6.6px headline/composer offset, a 2.5px composer-height mismatch, a lighter disabled send action, and an extra recent-project toolbar action. These were corrected against measured source geometry.
2. Initial mobile comparison found 6px narrow gutters and redistributed vertical spacing around the recent-project header. The final implementation matches the source at x=20, form y=233.1875, toolbar y=421.6875, and first card y=465.6875.
3. Final combined desktop and mobile comparisons found no actionable P0, P1, or P2 differences. Project names, cover artwork, project count, and relative timestamps intentionally come from the local SQLite data and therefore differ from the prototype fixture.

## Findings

No actionable P0, P1, or P2 findings remain.

## Verification

- Frontend unit suite: passed, 276 tests.
- Frontend production build: passed.
- Go project, HTTP API, and server packages: passed.
- Browser console after clean reload: no warnings or errors from the application.

previous result: passed

---

## Search dialog — current QA

### Comparison target

- Source visual truth: `https://zettos-prd-a1-prototypes.vercel.app/lumi-picture-book/home.html` and the user-provided `/var/folders/bh/q3rhh5290rxft20rp4691vz80000gn/T/codex-clipboard-254202ee-86e5-495b-a629-597ce8e953d6.png`.
- Desktop source capture: `/Users/qingyang/.codex/visualizations/2026/08/29/01a04dfe-fafb-76c3-bcc4-aeece162852d/source-search-dialog-1117x750.png`.
- Desktop implementation capture: `/Users/qingyang/.codex/visualizations/2026/08/29/01a04dfe-fafb-76c3-bcc4-aeece162852d/implementation-search-dialog-final-1117x750.png`.
- Desktop full-view comparison: `/Users/qingyang/.codex/visualizations/2026/08/29/01a04dfe-fafb-76c3-bcc4-aeece162852d/comparison-search-full.png` (source left, implementation right).
- Desktop focused comparison: `/Users/qingyang/.codex/visualizations/2026/08/29/01a04dfe-fafb-76c3-bcc4-aeece162852d/comparison-search-focused.png` (source left, implementation right).
- Mobile source capture: `/Users/qingyang/.codex/visualizations/2026/08/29/01a04dfe-fafb-76c3-bcc4-aeece162852d/source-search-dialog-390x844.png`.
- Mobile implementation capture: `/Users/qingyang/.codex/visualizations/2026/08/29/01a04dfe-fafb-76c3-bcc4-aeece162852d/implementation-search-dialog-390x844.png`.
- Mobile comparison: `/Users/qingyang/.codex/visualizations/2026/08/29/01a04dfe-fafb-76c3-bcc4-aeece162852d/comparison-search-mobile.png` (source left, implementation right).
- Viewports and density: desktop source and implementation are both 1117 × 750 CSS px and 1117 × 750 output px at device scale factor 1; mobile source and implementation are both 390 × 844 CSS px and 390 × 844 output px at device scale factor 1. No density normalization was required.
- State: the desktop comparison uses the open search dialog with the query `月亮`; the mobile comparison uses the open dialog with an empty query. The source fixture exposes project, full-story, and page results, while the requested implementation intentionally limits results to switchable local projects.

### Required fidelity surfaces

- Fonts and typography: passed. Both captures use Inter with Noto Sans SC/PingFang SC fallbacks, 18px/500 dialog headings, 14px/500 result titles, and 12px/500 secondary metadata with matching line heights.
- Spacing and layout rhythm: passed. The dialog is 520px wide on desktop and 350px wide at the 390px breakpoint, with 22px padding, an 8px radius, a 34px header, a 20px header gap, a 38px search field, a 14px result gap, and 60px result rows. Focused comparison confirms matching x positions, borders, padding, and icon alignment.
- Colors and visual tokens: passed. The white surface, `#dedfd8` borders, dark text, muted metadata, overlay opacity, and floating shadow map to the same Lumi tokens as the source.
- Image quality and asset fidelity: passed. This component has no raster imagery. Search, close, and book-open marks use the same Lucide icon family already used by the source and application; no placeholder, handcrafted SVG, CSS drawing, or emoji replacement was introduced.
- Copy and content: passed for the requested scope. Title, project-search label, project name, and `项目 · 最近编辑时间` hierarchy match the source. The local project count and names correctly come from the REST-backed recent-project facts rather than the prototype fixture.
- Responsive behavior: passed at 1117 × 750 and 390 × 844. The dialog remains centered, keeps 20px mobile gutters, scrolls its result region for longer project lists, and does not clip persistent controls.
- Interaction and accessibility: passed. The sidebar search button exposes dialog state, the search field receives initial focus, name/path filtering works, the empty result is announced, Escape closes and restores focus to the trigger, unavailable projects cannot be opened, and selecting an available result closes the dialog and navigates to that project's public UUID route.

### Comparison history

1. The first implementation capture matched the source geometry, but native dialog focus initially landed on the close button and exposed a prominent focus ring. The search field now receives focus after `showModal()`.
2. Browser testing found that Escape from a focused `type="search"` field did not reliably reach the native cancel event. The dialog now handles Escape directly and restores focus to the sidebar trigger.
3. The focused search input exposed a browser-native cancel glyph not present in the visual source. It was hidden without changing filtering or keyboard behavior.
4. Post-fix desktop and mobile comparisons found no actionable P0, P1, or P2 mismatch. The dialog height is now derived from the opened project-list capacity rather than the filtered result count, so live filtering and empty results do not move or resize the frame.

### Findings

No actionable P0, P1, or P2 findings remain. No P3 follow-up is required for the requested project-search scope.

### Verification

- Primary interactions tested: open, initial focus, live filtering, empty results, fixed-height filtering, Escape dismissal with focus return, available-project switching, and mobile navigation entry.
- Height stability proof: at the 1117 × 750 desktop viewport, the unfiltered, one-result, and empty-result states all measured 520 × 632px at x=380, y=44.
- Project switch proof: selecting `小熊的月亮灯` navigated to `/projects/019fe980-320b-7b62-9345-d19869d81deb/overview/summary` and removed the dialog.
- Browser console: no warnings or errors.
- Frontend unit suite: passed, 290 tests.
- Frontend production build: passed.

final result: passed
