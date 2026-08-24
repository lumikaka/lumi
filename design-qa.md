# Trajectory visual alignment QA

- Source implementation: `/Users/qingyang/project/collection-node/deepseek-harness/packages/client/ui-trajectory`
- Source full capture: `/Users/qingyang/.codex/visualizations/2026/08/21/01a02308-df51-7330-8bdd-27c88d755a86/trajectory-reference-current-3080.png`
- Source Request detail crop: `/var/folders/bh/q3rhh5290rxft20rp4691vz80000gn/T/codex-clipboard-0f6d9f94-1578-440f-8c1b-65a6f6a5214a.png`
- Implementation capture: `/Users/qingyang/.codex/visualizations/2026/08/21/01a02308-df51-7330-8bdd-27c88d755a86/trajectory-realigned-unselected-5801.png`
- Request focus capture: `/Users/qingyang/.codex/visualizations/2026/08/21/01a02308-df51-7330-8bdd-27c88d755a86/trajectory-realigned-request-focus-5801.png`
- Request selected capture: `/Users/qingyang/.codex/visualizations/2026/08/21/01a02308-df51-7330-8bdd-27c88d755a86/trajectory-realigned-request-selected-5801.png`
- System Prompt capture: `/Users/qingyang/.codex/visualizations/2026/08/21/01a02308-df51-7330-8bdd-27c88d755a86/trajectory-realigned-system-prompt-5801.png`
- Full-view comparison: `/Users/qingyang/.codex/visualizations/2026/08/21/01a02308-df51-7330-8bdd-27c88d755a86/trajectory-realigned-comparison.png`
- Focused Request comparison: `/Users/qingyang/.codex/visualizations/2026/08/21/01a02308-df51-7330-8bdd-27c88d755a86/trajectory-realigned-request-comparison.png`
- Source timeline capture: `/Users/qingyang/.codex/visualizations/2026/08/21/01a02308-df51-7330-8bdd-27c88d755a86/timeline-reference-before-3080.png`
- Pre-alignment Lumi timeline capture: `/Users/qingyang/.codex/visualizations/2026/08/21/01a02308-df51-7330-8bdd-27c88d755a86/timeline-current-before-5801.png`
- Viewport: 1280 × 720 CSS px; device pixel ratio 1.

## Comparison history

### Pass 1

- P1: At 1280 px the solo Lumi workspace still inherited a 320 px chat column, reducing the trajectory to 960 px. Fixed by preserving the one-column solo grid at that breakpoint.
- P1: The initial header, overview cards, filters, and 100 px chart consumed about 388 px before the Ledger. Reworked them into a 40 px identity bar, 32 px control bar, and 51 px three-lane chart.
- P2: The empty Inspector permanently reserved width. It now stays hidden until selection and opens as a resizable 390 px panel.
- P2: Ledger rows were 38–44 px with heavy boundaries. They were reduced to the reference's 30 px rhythm.

### Pass 2

- P2: The 1000 × 126 SVG view box letterboxed the 51 px chart. The chart coordinate system now fills its available width.
- P2: Search and filters competed with thread identity. They now sit in the compact trajectory-control band.
- P2: A missing Inspector width preference incorrectly resolved to 320 px. It now resolves to the intended 390 px default.

### Pass 3 — source-structure alignment

- P1: Lumi rendered Turn and Request as standalone rows. The source implementation keeps those facts in the projection but presents Turn as a 2 px vertical rail with a label on the first row, and Request as a 5 px boundary dot on the first eligible event. Lumi now uses the same structure.
- P1: The first persisted model request exposed a real system/tool digest and request payload but the Ledger omitted it. It now renders `Initial System Prompt` before Turn 1 and loads the actual prompt and tool catalog on demand. When older history is not complete, the copy safely falls back to `System Prompt Snapshot`.
- P2: SYSTEM and Model Request intentionally share the LLM log UUID. `item_kind` now disambiguates their URL-local selection while `item_uuid` remains the public UUIDv7 required for direct restoration.
- P2: The old three-column visual treatment left 190 px before content. The visual Ledger now matches the source's 122 px event column while preserving the semantic row markup for accessibility.

### Pass 4 — timeline alignment

- P1: Lumi defaulted to elapsed duration, so one recorded request expanded into a long violet span. The default is now the source's equal-width sequence view; Duration remains an explicit toggle and switches unknown-duration facts to 2 px markers.
- P1: The timeline now follows the source geometry exactly: a 32 px control strip, 50 px plot, 44 px label column, three 8 px lanes at 14 px intervals, 1 px Turn boundaries, and solid semantic blocks.
- P2: The control strip now presents the same `Duration / Turns / Calls` actions. Turns and Calls operate the existing independent collapse state instead of introducing timeline-only state.
- P2: Duplicate persisted Assistant output no longer consumes a second Model-lane block when it is already represented by its persisted Model Request. The join uses the projected Assistant-to-Request relation when the compact overview omits `request_uuid`.
- P2: The HTML timeline now preserves the source interactions: wheel zoom, left-drag range filtering, right-drag panning, double-click/Escape clearing, hover cursor, range edges, Turn boundaries, and click-through Inspector selection.

### Final comparison

No actionable P0, P1, or P2 differences remain inside the trajectory workspace. Lumi retains its own project shell, localized controls, and safe UUID facts. The reference session contains persisted CONTEXT, TOOL, and SUBTOOL facts; the inspected Lumi thread does not, so those rows are correctly absent rather than fabricated. The existing CONTEXT renderer remains active for real persisted context records.

## Required fidelity surfaces

- Fonts and typography: System/PingFang-style stack, 12 px dense row text, tokenized micro labels, single-line ellipsis, and 30 px row height.
- Spacing and hierarchy: 122 px event column, first-row Turn tab, continuous Turn rail, Request boundary marker, conditional 390 px Inspector, and subtle row rules.
- Colors and states: Neutral SYSTEM, blue USER, green CONTEXT, violet ASSISTANT, amber TOOL, explicit error color, blue selection rail, and a selected Request ring. Combined selected/hover rules remain after base selected rules.
- Data fidelity: Initial SYSTEM and later SYSTEM updates come from persisted LLM request snapshots/digests. CONTEXT appears only for persisted context facts. Full prompt/tools/response data remains an on-demand LLM detail read.
- Assets: The trajectory workspace has no required raster asset. DeepSeek branding and its mascot belong to the source application shell and were not copied into Lumi.

## Interaction and runtime checks

- Direct route load and refresh.
- Request #1 dot focus shows the label and selecting it opens the Model Request Inspector.
- SYSTEM and Model Request selection remain distinct after URL updates despite sharing a UUID.
- Initial System Prompt and Tools load from the persisted LLM detail.
- Closing Inspector removes both local selection parameters.
- Turn collapse remains operable from the compact Turn label; Assistant/Tool collapse stays independent.
- Timeline geometry measured in the browser: header 1280 × 32 px, plot 1280 × 50 px, label column 44 × 50 px, track 1236 × 50 px, and every lane block 8 px high.
- Default timeline contains one equal-width SYSTEM, USER, and MODEL REQUEST block for the inspected thread; the persisted Assistant output is correctly merged into its Model Request representation.
- Duration toggles to recorded time, Turns collapses/expands all available Turns, and clicking the Model Request span opens the matching Inspector URL.
- Browser console: no runtime errors or warnings; DevTools reports one form-field `id`/`name` advisory outside the timeline behavior.
- `git diff --check`: passed.
- `pnpm --dir web test`: 237 passed.
- `pnpm --dir web build`: passed.

## Follow-up polish

No remaining visual follow-up is required for this alignment pass.

final result: passed

---

# Trajectory bottom execution summary QA

- Source visual truth: `/Users/qingyang/.codex/visualizations/2026/08/21/01a02308-df51-7330-8bdd-27c88d755a86/trajectory-summary-reference-3080.png`
- Implementation screenshot: `/Users/qingyang/.codex/visualizations/2026/08/21/01a02308-df51-7330-8bdd-27c88d755a86/trajectory-summary-after-5801.png`
- Full-view comparison: `/Users/qingyang/.codex/visualizations/2026/08/21/01a02308-df51-7330-8bdd-27c88d755a86/trajectory-summary-full-comparison.png`
- Focused summary comparison: `/Users/qingyang/.codex/visualizations/2026/08/21/01a02308-df51-7330-8bdd-27c88d755a86/trajectory-summary-focused-comparison.png`
- Viewport: 1280 × 720 CSS px; device pixel ratio 1.
- Source pixels: 1280 × 720. Implementation pixels: 1280 × 720. No density normalization was required.
- State: complete Trace/Trajectory loaded without an open Inspector; whole-session/thread summary visible at the bottom.

## Comparison history

### Pass 1

- P2: Lumi had no bottom execution summary, so the aggregate cost and timing shape visible in deepseek-harness was absent. Added a dedicated final grid row backed by whole-Thread overview facts rather than the paged Ledger window.
- P2: The first implementation used Lumi's 10 px metadata token while the measured source summary uses 12 px text with a 20 px line height. Switched the line to the 12 px small-text token while retaining the source's centered, single-line, pipe-separated rhythm.

### Pass 2

No actionable P0, P1, or P2 differences remain for the summary strip. Both implementations use a centered 12/20 muted line at the bottom, preserve the same five information groups, and elide safely when space is insufficient. Lumi intentionally says `Turn` and `Request` instead of reintroducing the deprecated public `Step` term.

## Required fidelity surfaces

- Fonts and typography: Both source and implementation measure 12 px with 20 px line height, system UI glyphs, muted regular weight, nowrap, and end ellipsis.
- Spacing and layout rhythm: The summary occupies a 24 px bottom row, centers within a 980 px readable measure, and uses 10 px spacing around pipe separators to match the reference density.
- Colors and visual tokens: Source text is RGB 129/133/140; Lumi uses its muted text token, visually equivalent against the same white surface.
- Image quality and assets: The summary is text-only. No source imagery, logo, mascot, or decorative asset is part of this component.
- Copy and content: Counts, LLM/tool duration, TTFT/throughput, cache hit, and input/output tokens preserve the reference order. Missing streaming facts are explicitly labeled `未记录` instead of being inferred from total request time.

## Data and interaction checks

- Whole-Thread overview returns 1 Turn, 4 Requests, LLM 14.629 s, Tool 17.758 s, cache hit 22%, input 9,946 tokens, and output 689 tokens for the verified Lumi thread.
- TTFT and decode throughput remain absent in the REST DTO and render as unrecorded because Lumi's current Chat Agent does not persist those boundaries.
- Duration and token totals require complete recorded facts; partial legacy data does not become an exact total.
- At 1280 px the line does not overflow (`scrollWidth === clientWidth`); the complete line is also available through its title when a narrower viewport elides it.
- Browser console errors/warnings: none.
- `pnpm --dir web test`: 241 passed.
- `pnpm --dir web build`: passed.
- `go test ./internal/agent ./internal/httpapi ./internal/llmlog`: passed.
- `git diff --check`: passed.

## Follow-up polish

No remaining visual follow-up is required for this summary strip.

final result: passed

---

# Tool Payload / Response JSON QA

- Source visual truth: `/var/folders/bh/q3rhh5290rxft20rp4691vz80000gn/T/codex-clipboard-5c5d10bc-4b59-409c-954a-9d5ad522410f.png`
- Implementation URL: `http://127.0.0.1:5801/projects/019fff8b-eb9b-794a-8b35-3ea889c7e2a5/threads/01a02382-3e7d-722e-b33f-f63553d23476/trajectory?item_uuid=01a02382-5596-72b3-a2f2-4f37f2ec4036&item_kind=tool`
- Browser-rendered implementation: `/Users/qingyang/.codex/visualizations/2026/08/21/01a02308-df51-7330-8bdd-27c88d755a86/trajectory-tool-after-5801.png`
- Focused comparison: `/Users/qingyang/.codex/visualizations/2026/08/21/01a02308-df51-7330-8bdd-27c88d755a86/trajectory-tool-comparison.png`
- Viewport: 1280 × 720 CSS px; device pixel ratio 1.
- Source pixels: 513 × 846. Source Inspector crop: 461 × 846.
- Implementation pixels: 1280 × 720. Implementation Inspector crop: 390 × 544; scaled to 461 px wide and top-padded only for the side-by-side focused comparison.
- State: Tool `request_user_input` selected, Summary tab active, completed lifecycle with persisted Payload and Response.

## Comparison history

### Pass 1

- P2: The existing eight-row fact table pushed both Tool values below the visible Summary region. Fixed by matching the reference hierarchy: compact `Hierarchy` and `Status` facts first, then Payload and Response, with the full UUID/provider/model facts retained in a collapsed Metadata disclosure.
- P2: Unbounded pretty-printed JSON made the second value hard to reach in a narrow Inspector. Each Tool JSON block now keeps a 220 px readable viewport with independent overflow, so both Payload and Response remain discoverable without truncating their data.

### Pass 2

The focused side-by-side comparison shows no remaining actionable P0, P1, or P2 difference for the requested Tool value treatment. Lumi keeps its existing Inspector shell and valid JSON quoting while matching the source's compact hierarchy, section order, purple keys, red strings, and dense monospace presentation.

## Required fidelity surfaces

- Fonts and typography: Existing Lumi UI stack is retained; machine values use the established monospace stack, compact heading hierarchy, 1.5 line height, and safe wrapping.
- Spacing and layout rhythm: Compact two-row facts lead directly into Payload and Response. The 14 px section rhythm and bounded code view preserve density at the current 390 px Inspector width.
- Colors and visual tokens: JSON keys use violet, strings red, numbers blue, booleans teal, and null an italic amber tone; punctuation and braces inherit Lumi's neutral text color.
- Image quality and assets: This Inspector state has no product imagery. The source mascot belongs to the reference application's shell and is intentionally not copied.
- Copy and content: Tabs and headings now use `Payload` and `Response`; both render the complete persisted Tool arguments/result rather than the Ledger preview.

## Interaction and runtime checks

- Direct deep link restores the selected Tool.
- Summary exposes both Payload and Response.
- Payload and Response tabs each show the corresponding complete persisted object.
- Highlighted content is rendered as React text spans; no `dangerouslySetInnerHTML` path exists.
- Browser console errors/warnings: none.
- `pnpm --dir web test`: 237 passed.
- `pnpm --dir web build`: passed.
- `go test ./internal/agent ./internal/httpapi ./internal/llmlog`: passed.
- `git diff --check`: passed.

## Follow-up polish

No remaining visual follow-up is required for the requested Tool Payload/Response state.

final result: passed

---

# Design QA — ChatArea Codex-style duration summary

## Comparison Target

- Source visual truth: `/var/folders/bh/q3rhh5290rxft20rp4691vz80000gn/T/codex-clipboard-b01d4cec-0f67-4200-be6b-52a5bd48370b.png`
- Rendered implementation: `/Users/qingyang/.codex/visualizations/2026/08/24/01a032c8-16e2-7df0-adc8-47415827814f/chat-area-codex-final-collapsed.png`
- Expanded-state evidence: `/Users/qingyang/.codex/visualizations/2026/08/24/01a032c8-16e2-7df0-adc8-47415827814f/chat-area-codex-final-expanded.png`
- Browser viewport: `1280 × 720` CSS px, device pixel ratio `1`.
- Source pixels: `756 × 352`.
- Implementation pixels: `1280 × 720`; ChatArea panel crop `320 × 720`; duration control `295 × 38` CSS px.
- Density normalization: source and implementation were compared at their native `1×` density. The focused comparison preserves each component's native width because Lumi's ChatArea is intentionally narrower than the Codex conversation column.
- State: completed Turn, answered user input collapsed, duration/tool activity collapsed by default.

## Evidence

- Full-view combined comparison: `/Users/qingyang/.codex/visualizations/2026/08/24/01a032c8-16e2-7df0-adc8-47415827814f/chat-area-final-comparison.png`
- Focused duration-row comparison: `/Users/qingyang/.codex/visualizations/2026/08/24/01a032c8-16e2-7df0-adc8-47415827814f/duration-summary-final-comparison.png`
- A focused comparison was required because the source and implementation use different conversation-column widths; the target is the duration disclosure row rather than the surrounding product shell.

## Findings

- No actionable P0, P1, or P2 differences remain for the scoped duration disclosure.
- Fonts and typography: the final row uses the existing system/PingFang stack at `13px`, weight `400`, `18.2px` line height, matching the source's quiet metadata hierarchy and optical weight.
- Spacing and layout rhythm: the control is a full-width `38px` row with `1px` horizontal inset, no card radius, shadow, fill, tool icon, status badge, or secondary count summary. The bottom divider and compact chevron spacing match the source pattern.
- Colors and tokens: text uses Lumi's muted foreground (`rgba(31, 35, 40, 0.68)`), the background remains transparent, and the divider uses the existing subtle border token. Hover and focus states remain accessible without changing the resting appearance.
- Image quality and asset fidelity: the source contains no raster assets in the scoped component. The chevron uses the project's existing icon system and stays crisp at `12px`.
- Copy and content: normal state contains only localized elapsed time. Tool count, completion badge, tool icon, and expansion hint are absent. Exceptional Turns retain a compact semantic warning.

## Comparison History

1. Initial focused pass found a P2 typography/density mismatch: the implementation used `12px` at weight `500` in a `36px` row, appearing smaller and heavier than the source.
2. Fix applied: changed the label to `13px`/`400`, increased the row to `38px`, and reduced the chevron to `12px`.
3. Post-fix evidence: `duration-summary-final-comparison.png` shows matching hierarchy, spacing, muted color, transparent treatment, right chevron, and bottom divider. No P0/P1/P2 differences remain.

## Interaction And Runtime Checks

- Clicking the elapsed-time row expands three logical tool activities.
- Clicking again restores the default collapsed state.
- The disclosure remains a native `details/summary` control and exposes a visible keyboard focus state.
- Browser console errors checked: none.
- Existing raw arguments and results remain available after expansion.

## Follow-up Polish

- None required for the scoped control. The narrower Lumi ChatArea column is an intentional product constraint rather than design drift.

final result: passed
