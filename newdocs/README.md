# Osiris React reference

`newdocs/` is the canonical public language reference. It is a Vite/React
single-page application designed around reusable `PageShell`, `ApiListing`, and
`CodeBlock` components.

## Content model

- `src/legacy-reference.json` is the checked-in migration snapshot of the
  former static reference: chapters, types, API signatures, return types,
  detail notes, and examples.
- `src/support-matrix.json` is the checked-in backend-support contract.
- `src/api-data.js` derives navigation and global search records from that
  source data and holds small component-specific reference entries such as CLI
  contracts. Do not maintain a second handwritten API index.

Run `npm run migrate-reference` only when deliberately importing changes from
the preserved legacy source. The normal public build does not read `docs/`.

## Commands

```sh
npm ci
npm run check-reference
npm run check-examples
npm run build
```

`check-reference` verifies API metadata completeness, representative global
searches, and a redirect destination for every former static page. `build`
also generates ignored `public/docs/*.html` compatibility redirects, then
emits the publishable site to ignored `dist/`.

`check-examples` executes every self-contained `emit` (and documented
`try`/`catch`) example in the canonical reference. I/O examples are visibly
marked **sandbox required** and are not executed against the host filesystem.

GitHub Pages deployment is defined in `.github/workflows/docs.yml`.
