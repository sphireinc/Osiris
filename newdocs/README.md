# Osiris React reference

`newdocs/` is the canonical public language-reference source. It is the only
public documentation application published by GitHub Pages, and is a
Vite/React single-page application designed around reusable `PageShell`,
`ApiListing`, and `CodeBlock` components. Public documentation changes belong
here; `docs/` is frozen legacy compatibility and durable internal design
material, not an alternate reference to edit.

## Content model

- `src/legacy-reference.json` is the checked-in, React-owned reference data.
  Its historical filename records its origin only: it now owns chapters,
  types, API signatures, return types, detail notes, and examples. Edit it (or
  the small component-specific entries in `api-data.js`) for public-reference
  changes; do not regenerate it from `docs/`.
- `src/supplemental-reference.json` records implementation-audit additions:
  runtime helpers, special forms, runtime kinds, and the original grouped
  constant audit that were absent from the migrated snapshot.
- `src/constant-reference.json` owns the detailed **Global Constants** and
  **Error Constants** language pages, including aliases, meanings, values,
  return types, and checked examples.
- `src/cli-reference.json` provides the per-command entries that supplement the
  deeper tooling contracts in `api-data.js`.
- `src/technical-reference.json` owns the dense lexical, grammar, precedence,
  value-model, backend, CLI, and exhaustive surface indexes rendered by the
  final **Technical Reference** page.
- `src/support-matrix.json` is the checked-in backend-support contract.
- `src/api-data.js` derives navigation and global search records from that
  source data and holds small component-specific reference entries such as CLI
  contracts. It also exposes concrete members of documented constructor
  families as searchable aliases. Do not maintain a second handwritten API
  index.

Named entries are ordered at this assembly boundary by
`src/reference-order.js`. Sections retain their authored conceptual order;
entries inside each section use a locale-independent lowercase Unicode
code-point key with the original primary name as a tie-breaker. Rendering,
search, and navigation must consume those copied ordered arrays rather than
sorting independently or relying on JSON insertion order.

## Reference routes

Page links use a single URL fragment, such as `#math` or `#types`. Reference
entries add one readable path component to that fragment: `#math/ufn`,
`#io/read-limit`, and `#types/datetime`. Page-only links remain supported and
return to the top of the page.

`src/reference-route.js` owns parsing, formatting, entry-anchor derivation,
collision validation, and exact-target focus behavior. `ApiListing`, global
search, and each page's **On This Page** navigation use those shared anchors.
If two names on one page derive the same anchor, give the exceptional entry an
explicit lowercase URL-safe `anchor` in its reference data; never add an
order-dependent numeric suffix.

The normal public build and reference checks do not read `docs/`. Compatibility
redirects are generated from the legacy URL names so existing links continue to
work without making the old static manual a second source of truth.

## Commands

```sh
npm ci
npm run check-reference
npm run check-examples
npm run build
```

`check-reference` verifies API metadata completeness, that every published API
entry and documented family member is discoverable through global search, that
every current runtime built-in, evaluator special form, named constant,
reserved keyword, CLI command, and standard-library export has canonical
reference coverage outside the exhaustive technical index, representative
cross-reference searches, a redirect destination for every former static
page, and the complete unique deep-link surface. `check-navigation` can run
the route, anchor, search-target, history-observer, and focus checks by itself.
`build`
also generates ignored `public/docs/*.html` compatibility redirects, then
emits the publishable site to ignored `dist/`.

`check-examples` executes every self-contained `emit` (and documented
`try`/`catch`) example in the canonical reference. I/O examples are visibly
marked **sandbox required** and are not executed against the host filesystem.

GitHub Pages deployment is defined in `.github/workflows/docs.yml`.

## Brand assets

The published app keeps its own copies of the repository logos in
`public/brand/`. `osiris_simple_logo.png` is used in the navigation,
`osiris_logo_trans_cropped.png` is used in the overview hero, and
`logo_black_bg.png` is used in the site footer. The source images at the
repository root remain unchanged.
