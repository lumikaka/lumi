# Lumi Project List Design QA

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

final result: passed
