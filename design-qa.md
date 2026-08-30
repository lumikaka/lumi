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

---

## Chapter entry opens the existing page editor — current QA

### Comparison target

- Upstream interaction reference: `https://zettos-prd-a1-prototypes.vercel.app/lumi-picture-book/home.html`, opened in its `进入画面` / `逐页编辑` state.
- User-confirmed canonical implementation target: `/projects/01a05234-b434-7f1b-9aad-9a1468bdf599/chapters/01a0523b-cca1-7524-8085-49f47f347c31/sections/01a05241-8103-7340-8a9e-807a5e3d9a71`.
- Prototype state capture: `/Users/qingyang/.codex/visualizations/2026/08/30/01a05362-26de-7d20-bd06-d00b9bffd776/prototype-page-editor-1440x900.png`.
- Chapter-entry full capture: `/Users/qingyang/.codex/visualizations/2026/08/30/01a05362-26de-7d20-bd06-d00b9bffd776/chapter-entry-full-1440x900.png`.
- Chapter-entry focused content capture: `/Users/qingyang/.codex/visualizations/2026/08/30/01a05362-26de-7d20-bd06-d00b9bffd776/chapter-entry-content.png`.
- Canonical section focused content capture: `/Users/qingyang/.codex/visualizations/2026/08/30/01a05362-26de-7d20-bd06-d00b9bffd776/section-canonical-content.png`.
- Focused side-by-side comparison: `/Users/qingyang/.codex/visualizations/2026/08/30/01a05362-26de-7d20-bd06-d00b9bffd776/chapter-entry-content-comparison.png` (chapter entry left, canonical section route right).
- Viewport and density: both implementation states use a 1440 × 900 CSS viewport at device scale factor 1. Full output is 1440 × 900 pixels; both focused `.simple-project-content` captures are 886 × 842 pixels. No density normalization was required. The upstream prototype was inspected at 1440 × 900 CSS pixels; its Browser capture is normalized to 1440 × 900 output pixels despite the embedded surface reporting DPR 2.
- State: cover selected, saved/idle, project Agent expanded, global sidebar expanded. The focused comparison uses the same project facts, page, images, scroll position, and interaction state.

### Required fidelity surfaces

- Fonts and typography: passed. The chapter entry renders the exact existing `SimplePageView` component, so heading, labels, fields, status copy, weights, sizes, line heights, wrapping, and truncation match the canonical section route.
- Spacing and layout rhythm: passed. Both focused captures measure 886 × 842 pixels and retain the same toolbar, 142px page rail, editor column, gutters, borders, radii, and vertical rhythm. The normalized RMSE is `0.00352012`; the remaining pixel noise is JPEG/paint timing rather than visible layout drift.
- Colors and visual tokens: passed. Both routes reuse the same scoped Sass tokens and rendered component tree; no route-specific color, opacity, border, or surface variation is visible.
- Image quality and asset fidelity: passed. The default cover and every rail thumbnail are the same REST-backed image assets at the same crop, scale, and containment mode. No new or substitute asset was introduced.
- Copy and content: passed. The chapter title, page label, metadata, storyboard text, direction, references, generation state, candidates, and version content are the same SQLite-backed facts as the canonical section view.
- Responsive and route behavior: passed for the requested desktop entry. `/chapters/:chapterUuid` remains in place and directly renders the editor instead of redirecting; it selects the first ordered page (cover, then body pages, then back cover). Explicit `/sections/:sectionUuid` deep links still select the requested page.
- Interaction and accessibility: passed. Rail navigation moves to the public section UUID route, browser Back restores the chapter entry with the cover selected, the project-home action is no longer a self-link, and the chapter entry supplies the selected comic-section reference to Project Agent. Accessible roles, names, current-page state, and form controls are inherited unchanged from the canonical editor.

### Comparison history

1. Initial browser inspection showed the chapter root rendering the separate page-card management grid while the user-confirmed target lived at the cover section deep link.
2. The chapter route now renders `SimplePageView` directly. `SimplePageView` resolves an omitted section UUID to the first ordered page and delays section-dependent queries until that UUID exists; an empty chapter retains the existing create-page fallback.
3. The page-editor back action now returns to the project home instead of self-linking to the chapter entry. Project Agent context at the chapter entry resolves to the selected default comic section.
4. Post-fix focused comparison found no actionable P0, P1, or P2 differences between the chapter entry and the existing canonical section view.

### Findings

No actionable P0, P1, or P2 findings remain. No P3 follow-up is required for the requested chapter-entry scope.

### Verification

- Primary interactions tested: direct chapter entry, default cover selection, rail navigation to `正文第 1 页`, explicit section deep-link persistence, browser Back to the cover entry, project-home target, and comic-section Agent reference construction.
- Browser console after clean navigation and interaction checks: no warnings or errors.
- Focused source/implementation comparison: passed.
- Frontend unit suite: passed, 324 tests.
- Frontend production build: passed.

final result: passed

---

## Simple project topbar — current QA

### Comparison target

- Source visual truth: `https://zettos-prd-a1-prototypes.vercel.app/lumi-picture-book/home.html`.
- Desktop source capture: `/Users/qingyang/.codex/visualizations/2026/08/30/01a05146-0d34-79a0-b7b6-b9836883f940/simple-topbar-reference-capture.png`.
- Desktop implementation capture: `/Users/qingyang/.codex/visualizations/2026/08/30/01a05146-0d34-79a0-b7b6-b9836883f940/simple-topbar-implementation-capture.png`.
- Desktop full-view comparison: `/Users/qingyang/.codex/visualizations/2026/08/30/01a05146-0d34-79a0-b7b6-b9836883f940/simple-topbar-full-comparison.png` (source left, implementation right).
- Desktop focused comparison: `/Users/qingyang/.codex/visualizations/2026/08/30/01a05146-0d34-79a0-b7b6-b9836883f940/simple-topbar-focused-comparison.png` (source left, implementation right).
- Viewport and density: both source and implementation are 1440 × 900 CSS px and 1440 × 900 output px. The source surface reports DPR 2 but the Browser capture is normalized to one output pixel per CSS pixel; the implementation reports DPR 1. No further density normalization was required.
- State: simple project home, global sidebar collapsed to 60px, project Agent expanded, topbar idle. The source fixture and implementation intentionally show different project facts and artwork.

### Required fidelity surfaces

- Fonts and typography: passed. Both topbars use the existing Inter/Noto Sans SC/PingFang SC stack. Project title is 14px/500 with a 21px line height; the page label is 12px/430 with a 21px line height and the same baseline treatment.
- Spacing and layout rhythm: passed. Both topbars are 58px high with 18px horizontal padding, a 12px main gap, an 8px title-label gap, and two 34 × 34px right actions separated by 12px. Title and action positions align at x=78/y=17.5 and x=1342,1388/y=11.5 in the matched state.
- Colors and visual tokens: passed. The white surface, `#dedfd8` bottom border, `#20231f` primary text, `#747970` secondary text, and `#f7f7f3` hover surface match the source.
- Image quality and asset fidelity: passed for this scope. The topbar contains no raster imagery. Menu, message-circle, and ellipsis controls use the matching Lucide family at 16px with a 1.6 stroke; no placeholder or handcrafted asset was introduced.
- Copy and content: passed. The implementation preserves the source hierarchy of dynamic project name plus current page label. Different names are expected because the implementation reads local SQLite facts rather than prototype fixtures.
- Responsive behavior: passed for the supplied desktop target. The implementation keeps the existing mobile menu breakpoint and removes the desktop-only centered simple-mode page tabs, so the topbar remains a single compact row at narrow widths. No separate mobile source state was available from the supplied prototype surface.
- Interaction and accessibility: passed. The Agent action toggles expanded and collapsed ChatArea states and updates its accessible name; the more-project-actions control opens the existing project configuration dialog; the mobile navigation control retains its accessible name. The expert route still renders `.project-topbar` and `.project-topbar__nav` and does not render the simple topbar.

### Comparison history

1. The initial comparison found a P1 structural mismatch: simple mode reused the expert-style centered page tabs and account action. These were removed from the simple topbar, which now uses the source's left context block and right message/ellipsis action group. The expert topbar was left unchanged.
2. The first post-fix measurement found a P2 height mismatch: global button minimum height made the intended 34px actions render at 38px. The simple topbar actions now explicitly set `min-height: 34px`.
3. The final full-view and focused comparisons confirm the 58px frame, borders, padding, baselines, title rhythm, icon sizing, and right-edge action positions. No actionable P0, P1, or P2 mismatch remains.

### Findings

No actionable P0, P1, or P2 findings remain. No P3 follow-up is required for the requested topbar scope.

### Verification

- Primary interactions tested: collapse Agent, expand Agent, open project configuration from the ellipsis action, dismiss the dialog, and load an expert-mode route.
- Browser console after clean reload and interaction checks: no warnings or errors.
- Frontend unit suite: passed, 315 tests.
- Focused source/implementation comparison: passed.
- Frontend production build: passed.

final result: passed

---

## Simple page rail create menu — current QA

### Comparison target

- User-supplied rail reference: `/var/folders/bh/q3rhh5290rxft20rp4691vz80000gn/T/codex-clipboard-6c8d22c3-d5b1-4c7a-9ccd-8afdc940ead4.png`.
- Upstream interaction reference: `https://zettos-prd-a1-prototypes.vercel.app/lumi-picture-book/home.html`, opened in its `进入画面` / `逐页编辑` state.
- Implementation closed state: `/Users/qingyang/.codex/visualizations/2026/08/30/01a05362-26de-7d20-bd06-d00b9bffd776/page-rail-closed.png`.
- Implementation menu state: `/Users/qingyang/.codex/visualizations/2026/08/30/01a05362-26de-7d20-bd06-d00b9bffd776/page-rail-menu-open.png`.
- Focused side-by-side comparison: `/Users/qingyang/.codex/visualizations/2026/08/30/01a05362-26de-7d20-bd06-d00b9bffd776/page-rail-comparison.png` (reference left, implementation right).
- Narrow-screen menu capture: `/Users/qingyang/.codex/visualizations/2026/08/30/01a05362-26de-7d20-bd06-d00b9bffd776/page-rail-mobile-menu.png`.
- Comparison normalization: the supplied 392 × 760 DPR-2 rail crop is compared with a 392 × 760 implementation crop at the same visual scale and interaction state.

### Required fidelity surfaces

- Fonts and typography: passed. The rail heading is the same 13px semibold muted label as the reference after density normalization; menu items use the existing 12px/500 Lumi control typography.
- Spacing and layout rhythm: passed. The rail is 154px wide with 16px side gutters, a 54px header, 28px plus target, 122px page cards, and 9px card gaps. In the focused comparison, the heading, plus center, divider, and thumbnail bounds align with the supplied reference.
- Colors and visual tokens: passed. The rail uses the standard subtle surface, shared border, muted text, white floating menu, accent hover/focus state, and existing floating shadow tokens.
- Image quality and asset fidelity: passed for the requested scope. Existing SQLite-backed page images and Lucide `Plus` icon are reused; no substitute raster, placeholder asset, handcrafted SVG, or CSS drawing was introduced.
- Copy and content: passed. The visible heading is `页面`. The menu reproduces the expert-mode page-role choices in the same order: `封面`, `正文页`, `封底`. Existing roles are disabled by the same shared rule, so the current project correctly disables its already-present cover.
- Responsive behavior: passed at the desktop reference size and 760 × 844. Desktop keeps the vertical rail; narrow layouts keep the heading and plus above a horizontally scrollable page list, with the menu anchored to the trigger and fully visible.
- Interaction and accessibility: passed. The plus exposes `aria-haspopup="menu"` and `aria-expanded`; opening focuses the first enabled item, Arrow keys cycle enabled choices, Home/End jump, Escape closes and restores trigger focus, Tab and outside pointer dismiss, and selecting a role uses the same REST-backed blank-page creation semantics as expert mode. Dirty current-page edits are saved before creation.

### Comparison history

1. Initial implementation exposed the required choices, but the first visual crop showed the existing 142px rail and 10px gutters were narrower than the supplied reference.
2. Measurement of the DPR-2 reference established a 154px CSS rail with 16px gutters and 122px cards. The implementation now uses those dimensions and the shared subtle-surface token.
3. The first closed-state capture retained the trigger focus ring after Escape. The final comparison moves focus back into the editor so the idle plus state matches the supplied reference; focus feedback remains intact for keyboard users.
4. Final desktop and narrow-screen checks found no actionable P0, P1, or P2 mismatch for the requested page-heading and create-menu scope.

### Findings

No actionable P0, P1, or P2 findings remain. No P3 follow-up is required for the requested page-creation menu scope.

### Verification

- Primary interactions tested: click open, disabled existing cover, ArrowDown navigation, Escape dismissal with focus return, outside-pointer dismissal, desktop layout, and narrow-screen layout.
- Clean browser tab console: no warnings or errors.
- Frontend unit suite: passed, 325 tests.
- Frontend production build: passed.

final result: passed
