# Task 105: Define and enforce reference-page entry ordering

**Priority:** P2  
**Dependencies:** 039, 040, 100, 101, 102, 103, 104  
**Status:** Ready  

## Objective

Give every canonical API/reference page a predictable entry order. Readers
should not have to infer whether functions, types, constants, or tooling
commands are ordered by implementation history, source-file placement, or the
order in which supplemental data happened to be merged.

This task establishes the one canonical rendered order that task 106's
per-page table of contents will reuse.

## Pages in scope

Apply the policy to entries rendered on:

- Language Reference pages, including Syntax & Semantics, Core, Types, Global
  Constants, and Error Constants;
- Data & Numerics Reference pages;
- the I/O Reference page;
- Standard Library Reference pages, including large families such as
  `std/math`;
- the Tooling Reference page.

Do not reorder prose chapters, the support matrix, the Technical Reference's
long-form sections, sidebar page groups, or intentionally ordered syntax/code
examples in this task.

## Canonical ordering policy

Preserve the intentional order of pages and of named sections within a page.
Within each rendered API section, order entries by their primary public display
name:

1. compare a locale-independent lowercase key;
2. compare by Unicode code point, not `localeCompare`, host locale, insertion
   order, or object/map enumeration;
3. use the original primary name as the deterministic tie-breaker;
4. if two entries still collide, fail reference validation rather than relying
   on a stable-sort side effect.

Aliases do not determine placement. A family entry such as `iN`, `uN`, `ufN`,
or `bfN` remains one entry and sorts by that primary family name. Curated
conceptual sections remain intact: for example, constants continue to live
under their measurement/physical/error sections, but entries inside each
section become predictable.

## Implementation requirements

- Audit how `legacy-reference.json`, `supplemental-reference.json`,
  `time-reference.json`, `constant-reference.json`, `cli-reference.json`, and
  the React-owned language data are assembled into `modules` and
  `languageSections`.
- Implement one shared deterministic comparison/order function at the
  canonical data-assembly boundary. Do not hand-maintain a second ordering
  table and do not require future authors to place JSON records perfectly for
  the UI to remain ordered.
- Ensure page rendering, reference search result construction, and the entry
  sequence exposed to task 106 all consume the same ordered section arrays.
- Do not mutate source arrays in place where that could make import order or
  repeated rendering observable. Return copied ordered arrays.
- Preserve each entry's primary name, aliases, signature, description,
  examples, explicit section, and stable object identity where the current
  reference checker depends on it.
- Document the policy briefly in `newdocs/README.md` so future reference
  additions know where ordering is enforced.

## Required tests

- Extend `newdocs/scripts/check-reference.mjs` or add a narrow companion check
  that verifies every in-scope rendered section is monotonic under the shared
  comparator.
- Reject duplicate primary sort identities inside a rendered section with an
  actionable page/section diagnostic.
- Add representative assertions for:
  - the `std/math` constructor families (`bfN`, `iN`, `uN`, `ufN`);
  - mixed lowercase/uppercase constant names;
  - error constants;
  - Tooling entries whose display names begin with `osiris`;
  - entries contributed by both a base group and a supplemental group.
- Prove repeated module construction yields the same order and does not mutate
  the imported JSON arrays.
- Keep exact runtime/standard-library/constant surface checks passing; ordering
  must not add, drop, rename, or merge API entries.

## Documentation and UI checks

- Inspect at least Core, Types, Global Constants, I/O, `std/math`, `std/text`,
  and Tooling in a production build.
- Confirm section boundaries remain pedagogically useful and headings are not
  alphabetized together with their children.
- Confirm search still finds primary names and aliases and returns entries in a
  deterministic page/section/entry order.

## Acceptance criteria

- Every in-scope rendered section follows the documented deterministic
  ordering policy.
- One canonical ordered data path feeds page listings, search, and the future
  page table of contents.
- `std/math` and other large pages have visibly predictable entry order.
- Automated validation prevents later source/merge changes from silently
  reintroducing arbitrary ordering.
- `npm run check-reference`, `npm run check-examples`, and `npm run build`
  pass under Node 24.
- `go test ./...`, `git diff --check`, and task-index validation pass.

## Non-goals

- No API renames, alias changes, section redesign, sidebar reordering, runtime
  implementation change, or Technical Reference rewrite.
- Do not add deep-link routing or a visible table of contents here; task 106
  owns that behavior after this ordering contract is complete.

