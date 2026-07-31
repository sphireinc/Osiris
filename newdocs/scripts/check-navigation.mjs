import assert from 'node:assert/strict';
import { languageSections, modules, searchEntries } from '../src/api-data.js';
import {
  entryAnchor,
  focusReferenceEntry,
  formatReferenceRoute,
  observeReferenceRoute,
  parseReferenceRoute,
  validateReferenceAnchors,
} from '../src/reference-route.js';

const routeCases = [
  ['', { pageID: 'overview', entryAnchor: null }],
  ['#overview', { pageID: 'overview', entryAnchor: null }],
  ['#math', { pageID: 'math', entryAnchor: null }],
  ['#math/ufn', { pageID: 'math', entryAnchor: 'ufn' }],
  ['#io/read-limit', { pageID: 'io', entryAnchor: 'read-limit' }],
  ['#types/date%74ime', { pageID: 'types', entryAnchor: 'datetime' }],
  ['#tooling/osiris-lsp', { pageID: 'tooling', entryAnchor: 'osiris-lsp' }],
  ['#constants', { pageID: 'global-constants', entryAnchor: null }],
  ['#unknown-page', { pageID: 'unknown-page', entryAnchor: null }],
  ['#math/unknown-entry', { pageID: 'math', entryAnchor: 'unknown-entry' }],
  ['#math/%E0%A4%A', { pageID: 'math', entryAnchor: '%E0%A4%A' }],
];
for (const [hash, expected] of routeCases) {
  assert.deepEqual(parseReferenceRoute(hash), expected, `route parsing failed for ${hash || '<empty>'}`);
}

const formatCases = [
  ['math', null, '#math'],
  ['math', 'ufn', '#math/ufn'],
  ['io', 'read-limit', '#io/read-limit'],
  ['types', 'date time', '#types/date%20time'],
];
for (const [pageID, anchor, expected] of formatCases) {
  assert.equal(formatReferenceRoute(pageID, anchor), expected);
  const parsed = parseReferenceRoute(expected);
  assert.equal(parsed.pageID, pageID);
  assert.equal(parsed.entryAnchor, anchor);
}

for (const [name, expected] of [
  ['iN', 'in'],
  ['uN', 'un'],
  ['ufN', 'ufn'],
  ['read_limit', 'read-limit'],
  ['datetime', 'datetime'],
  ['osiris lsp', 'osiris-lsp'],
]) assert.equal(entryAnchor(name), expected, `unexpected stable anchor for ${name}`);
assert.equal(entryAnchor({ name: 'let?', anchor: 'let-result' }), 'let-result');

const pages = [...modules, { id: 'language', label: 'Syntax and Semantics', sections: languageSections }];
let entryCount = 0;
for (const page of pages) {
  const targets = validateReferenceAnchors(page.id, page.sections);
  const tocAnchors = page.sections.flatMap((section) => section.entries.map((entry) => entryAnchor(entry)));
  assert.equal(tocAnchors.length, targets.size, `${page.id} has a duplicate or missing table-of-contents target`);
  for (const anchor of tocAnchors) {
    const route = formatReferenceRoute(page.id, anchor);
    assert.deepEqual(parseReferenceRoute(route), { pageID: page.id, entryAnchor: anchor });
    assert.equal(tocAnchors.filter((candidate) => candidate === anchor).length, 1, `${page.id}/${anchor} is not unique`);
  }
  entryCount += tocAnchors.length;
}

assert.throws(
  () => validateReferenceAnchors('collision-test', [{
    title: 'Collision',
    entries: [{ name: 'let' }, { name: 'let?' }],
  }]),
  /collision-test.*let.*let\?.*explicit stable anchor/i,
);

for (const [query, pageID, anchor] of [
  ['ufN', 'math', 'ufn'],
  ['read_limit', 'io', 'read-limit'],
  ['datetime', 'types', 'datetime'],
  ['osiris lsp', 'tooling', 'osiris-lsp'],
]) {
  const result = searchEntries(query).find((candidate) => (
    candidate.module.id === pageID && entryAnchor(candidate.entry) === anchor
  ));
  assert.ok(result, `search does not find exact route target ${pageID}/${anchor}`);
  assert.equal(formatReferenceRoute(result.module.id, entryAnchor(result.entry)), `#${pageID}/${anchor}`);
}

const listeners = new Set();
const fakeWindow = {
  location: { hash: '#math/ufn' },
  addEventListener(name, listener) {
    if (name === 'hashchange') listeners.add(listener);
  },
  removeEventListener(name, listener) {
    if (name === 'hashchange') listeners.delete(listener);
  },
};
const observed = [parseReferenceRoute(fakeWindow.location.hash)];
const stopObserving = observeReferenceRoute(fakeWindow, (route) => observed.push(route));
for (const hash of ['#math/un', '#io/read-limit', '#math/ufn']) {
  fakeWindow.location.hash = hash;
  for (const listener of listeners) listener();
}
stopObserving();
assert.deepEqual(observed, [
  { pageID: 'math', entryAnchor: 'ufn' },
  { pageID: 'math', entryAnchor: 'un' },
  { pageID: 'io', entryAnchor: 'read-limit' },
  { pageID: 'math', entryAnchor: 'ufn' },
]);
assert.equal(listeners.size, 0, 'hash route observer did not clean up');

const focusCalls = [];
const target = {
  scrollIntoView(options) { focusCalls.push(['scroll', options]); },
  focus(options) { focusCalls.push(['focus', options]); },
};
const fakeDocument = {
  getElementById(id) {
    return id === 'ufn' ? target : null;
  },
};
assert.equal(focusReferenceEntry(fakeDocument, 'ufn'), true);
assert.deepEqual(focusCalls, [
  ['scroll', { block: 'start' }],
  ['focus', { preventScroll: true }],
]);
assert.equal(focusReferenceEntry(fakeDocument, 'unknown-entry'), false);
assert.equal(focusCalls.length, 2, 'an unknown entry focused or scrolled an unrelated target');

console.log(`Reference navigation checks passed: ${pages.length} pages, ${entryCount} unique entry routes.`);
