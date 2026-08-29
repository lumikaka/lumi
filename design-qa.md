# Design QA

## Evidence

- Source visual truth: `https://zettos-prd-a1-prototypes.vercel.app/lumi-picture-book/home.html`
- Source global-home capture: `/tmp/lumi-target-global-home-1280x720-current.png`
- Implementation global-home capture: `/tmp/lumi-implementation-home-1280x720-final.png`
- Full-view global-home comparison: `/tmp/lumi-home-comparison-final.png`
- Focused composer/card comparison: `/tmp/lumi-home-focus-comparison.png`
- Source project-workspace capture: `/tmp/lumi-target-project-1280x720.png`
- Implementation project-workspace capture: `/tmp/lumi-implementation-project-1280x720-final.png`
- Full-view project comparison: `/tmp/lumi-project-comparison-final.png`
- Focused Agent comparison: `/tmp/lumi-agent-focus-comparison.png`
- Implementation URL: `http://127.0.0.1:5803/`

The primary comparison viewport was `1280 x 720` CSS pixels at device scale factor `1`. Every source and implementation screenshot used for the main comparison is `1280 x 720` pixels, so no density resampling was needed. Focused comparisons use identical crops from those normalized screenshots: home `880 x 480` per side and Agent `340 x 720` per side.

## State

- Light user application only.
- Global home compared with the desktop sidebar expanded.
- Project workspace compared with the sidebar collapsed and the `330px` Agent visible, matching the target region split.
- Product-specific project data and copy differ between the prototype and Lumi implementation. Existing Lumi information architecture, routes, and business content were intentionally preserved.
- The target's project-cover imagery was not implemented because this iteration explicitly excludes covers, placeholder art, and cover APIs.

## Findings

No actionable P0, P1, or P2 visual differences remain within the agreed scope.

- Fonts and typography: the implementation uses the target-style system sans stack with `14px` body, `12px` secondary, and `11px` micro text; weights are constrained to `430/500/600`. Headline wrapping differs where Lumi's real copy differs, but hierarchy and density are aligned.
- Spacing and layout rhythm: the desktop sidebar is `224px`, collapsed sidebar `60px`, title bar `58px`, home composer `784px` wide with a `92px` input region, and project Agent `330px`. The normalized full-view and focused comparisons confirm the major region proportions.
- Colors and visual tokens: page, sidebar, surfaces, borders, green accent, warm decorative color, radii, and restrained elevation follow the target palette. Secondary text was darkened to `#666b63` because the initially planned `#73786f` did not meet AA on every target-style surface.
- Image quality and asset fidelity: existing Lumi favicon and Lucide icons remain sharp and code-native. No target project imagery was approximated or replaced with fake placeholders; omission is intentional and required by scope.
- Copy and content: Lumi's existing business wording, actions, routes, project states, and recovery behavior remain unchanged. The visual hierarchy was migrated without inventing a target-only search route or new product content.
- Accessibility and interaction: visible focus styles, keyboard activation, selected/active/expanded hover combinations, drawer escape dismissal, menu dismissal, and unavailable-project recovery affordances were retained.

## Comparison History

### Pass 1 — blocked by P2 differences

- [P2] Home content was too wide and centered differently from the source. The composer/card region measured about `880px` instead of the target `784px`, and the hero alignment drifted.
- [P2] The creation composer was too tall at roughly `200px`, versus the target's roughly `150px` shell and `92px` text area.
- [P2] The Agent used a hard white surface instead of the source's soft panel background.
- [P2] The planned `#73786f` secondary color produced insufficient contrast on the page/sidebar surfaces.

Fixes: constrained and left-aligned the home content region, set the composer to `784 x 148` with a `92px` text area, moved Agent surfaces to the soft token, and changed secondary text to `#666b63`.

### Pass 2 — passed

Post-fix evidence is in `/tmp/lumi-home-comparison-final.png`, `/tmp/lumi-home-focus-comparison.png`, `/tmp/lumi-project-comparison-final.png`, and `/tmp/lumi-agent-focus-comparison.png`. The implementation now matches the target's major geometry, surface hierarchy, density, and responsive shell while retaining Lumi-specific content and the explicit no-cover constraint.

## Responsive and Interaction Verification

- `1280 x 720`: sidebar, content, and Agent render together; project split is `60 / 890 / 330` when the sidebar is collapsed.
- `1199 x 720`: sidebar remains; Agent opens as a right-side `330px` dialog overlay.
- `760 x 720`: sidebar becomes a `224px` drawer with backdrop; Agent becomes a full-screen dialog; title bar wraps without hiding primary controls.
- Sidebar collapse/expand persists through `lumi.globalSidebarCollapsed`.
- Home cards keep unavailable projects non-enterable and expose relocate/forget/reveal actions through the more menu.
- Settings, Premise, Chapters, and Agent were opened in the browser; the checked legacy blue/orange computed colors were absent.
- Browser console errors checked after navigation: none.

## Implementation Checklist

- [x] Central light-theme tokens and typography scale
- [x] Persistent/collapsible desktop sidebar and mobile drawer
- [x] `58px` top bar and responsive `330px` Agent behavior
- [x] Coverless home project cards with preserved business flows
- [x] User-workspace color, radius, density, and elevation migration
- [x] Combined-state hover/focus rules
- [x] `1280`, `1199`, and `760` responsive verification
- [x] 276 frontend tests and production build

final result: passed
