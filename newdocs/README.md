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
cross-reference searches, and a redirect destination for every former static
page. `build`
also generates ignored `public/docs/*.html` compatibility redirects, then
emits the publishable site to ignored `dist/`.

`check-examples` executes every self-contained `emit` (and documented
`try`/`catch`) example in the canonical reference. I/O examples are visibly
marked **sandbox required** and are not executed against the host filesystem.

GitHub Pages deployment is defined in `.github/workflows/docs.yml`.
