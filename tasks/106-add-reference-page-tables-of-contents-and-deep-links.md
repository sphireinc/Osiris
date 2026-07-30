# Task 106: Add reference-page tables of contents and stable deep links

**Priority:** P2  
**Dependencies:** 039, 040, 105  
**Status:** Blocked on task 105  

## Objective

Add a clickable “On This Page” table of contents immediately after the
`section.hero` on every canonical Language, Data & Numerics, I/O, Standard
Library, and Tooling reference page. Each link must navigate directly to the
function, type, constant, family, or tooling entry it names.

The existing router already uses one URL fragment for the page
(`http://localhost:5173/#math`). A second `#` is not valid fragment routing.
Resolve that routing boundary first, then build the table of contents on top of
the resolved route.

## Required URL contract

Use one fragment with a page/entry path:

```text
#math             page-level route; remains backward-compatible
#math/ufn         deep link to the std/math ufN family
#io/read-limit    deep link to read_limit
#types/datetime   deep link to datetime
#tooling/osiris-lsp
```

The part before the first `/` is the existing page ID. The optional remainder
is one entry anchor. Do not introduce `#math#ufn`, query-string state, a second
router, or server-side routing requirements.

## Phase 1: route and anchor model

Complete and test this phase before adding the visible table of contents.

- Extract a small shared route parser/formatter rather than continuing to use
  raw `window.location.hash.slice(1)` in multiple places.
- Preserve all existing page-only routes, including Overview, Support,
  chapters, Technical Reference, and every module page.
- Define these cases:
  - an empty fragment selects Overview;
  - `#math` selects the top of the Math page;
  - `#math/ufn` selects Math and the `ufn` entry;
  - browser back/forward and a same-page deep-link change update both page and
    selected entry;
  - an unknown page retains the current unknown-page behavior without a
    JavaScript exception;
  - an unknown entry renders the valid page safely and does not scroll to or
    focus an unrelated element.
- Create one shared entry-anchor function used by `ApiListing`, route
  formatting, search-result links, and the table of contents. The expected
  anchors are stable lowercase URL-safe slugs (`ufN` → `ufn`,
  `read_limit` → `read-limit`).
- Audit anchor uniqueness within every page. If a derived anchor collides,
  require an explicit stable anchor in reference data and make validation fail
  until the collision is resolved. Never append an array index or another
  order-dependent suffix.
- Percent-decode route components defensively. Malformed encodings must not
  crash the app.

## Phase 2: table-of-contents component

- Render a semantic `<nav>` immediately after each applicable page hero, with
  a visible “On This Page” heading and an `aria-label`.
- Cover:
  - the Syntax & Semantics Language page;
  - Core, Types, Global Constants, and Error Constants;
  - every Data & Numerics page;
  - I/O;
  - every Standard Library page;
  - Tooling.
- Use the ordered section/entry arrays established by task 105. Do not sort a
  second time in the component.
- Preserve section grouping when a page has multiple sections. Each entry
  appears exactly once under its primary public name; aliases may be shown as
  secondary text but must not create duplicate targets.
- Make large surfaces such as `std/math` useful rather than overwhelming:
  provide a compact responsive multi-column presentation and an accessible
  expand/collapse or bounded-list treatment if needed. The complete link set
  must remain keyboard reachable and screen-reader discoverable.
- Use ordinary links with canonical fragment URLs so users can copy, open in a
  new tab, and inspect the destination without JavaScript-only button state.
- Give anchored articles sufficient `scroll-margin-top` for the sticky header.
  After routing/rendering, scroll the exact target into view and provide
  sensible focus behavior without creating a focus loop on ordinary renders.
- Indicate the currently addressed entry without making that styling the only
  accessibility signal.

## Search and navigation integration

- A search result must link to the exact entry (`#math/ufn`), not merely open
  the containing page.
- Sidebar page navigation continues to use page-only fragments and returns to
  the page top.
- Clicking another entry on the same page must update history and scroll even
  though the page component remains mounted.
- Directly loading or refreshing a deep URL must render and navigate correctly
  after React mounts.

## Required tests

- Add table-driven tests for route parsing/formatting, including empty,
  page-only, deep, malformed-encoding, unknown-page, and unknown-entry cases.
- Validate that every generated table-of-contents URL round-trips to its page
  and entry.
- Validate that every table-of-contents target exists exactly once and every
  in-scope rendered entry appears exactly once in the table of contents.
- Validate page-local anchor uniqueness and stable expected anchors for at
  least `iN`, `uN`, `ufN`, `read_limit`, `datetime`, and `osiris lsp`.
- Add coverage showing search selects the exact entry route.
- Exercise initial deep-link load, same-page link navigation, and
  back/forward behavior with the lightest existing browser/DOM test mechanism.
  Do not add a large end-to-end dependency solely for this task if the current
  stack can test the behavior more narrowly.

## Visual and accessibility checks

- Inspect Core, Types, I/O, `std/math`, `std/text`, and Tooling at narrow and
  wide viewport sizes.
- Confirm the table of contents begins immediately after the hero, never
  overlays or obscures the first entry, and does not cause horizontal page
  scrolling.
- Confirm keyboard tab order, visible focus, current-entry indication,
  semantic navigation labeling, and target focus/scroll placement.
- Confirm a copied deep link works in a fresh tab and after a production-build
  refresh served by Vite's hash-compatible preview.

## Documentation

- Document `#page/entry` deep-link syntax and backward-compatible `#page`
  routes in `newdocs/README.md`.
- Keep examples and public language content unchanged; this task changes
  navigation, not API semantics.

## Acceptance criteria

- Every requested reference page has a complete clickable table of contents
  immediately after its hero.
- `#page/entry` is the single tested deep-link contract, while all existing
  `#page` links still work.
- `std/math` includes working links for `iN`, `uN`, `ufN`, and its other
  entries.
- Search, direct loads, same-page navigation, refresh, and browser history all
  land on the exact entry.
- Anchors are unique, stable, readable, and verified against the rendered API
  surface.
- The layout is responsive and the navigation is keyboard/screen-reader
  usable.
- `npm run check-reference`, the focused routing/DOM tests,
  `npm run check-examples`, and `npm run build` pass under Node 24.
- `go test ./...`, `git diff --check`, and task-index validation pass.

## Non-goals

- No history-API/server router migration, documentation content rewrite,
  entry reordering, API rename, or Technical Reference redesign.
- Do not create a separate alias anchor for every alias unless a later task
  defines an alias-URL compatibility policy.

