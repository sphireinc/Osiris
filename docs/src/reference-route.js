const pageAliases = new Map([
  ['constants', 'global-constants'],
]);

function safeDecode(component) {
  try {
    return decodeURIComponent(component);
  } catch {
    return component;
  }
}

function safeEncode(component) {
  return encodeURIComponent(component);
}

export function entryAnchor(entry) {
  const explicit = typeof entry === 'object' ? entry.anchor : undefined;
  const name = typeof entry === 'string' ? entry : entry.name;
  const anchor = explicit || name.toLowerCase()
    .normalize('NFKD')
    .replaceAll(/[\u0300-\u036f]/g, '')
    .replaceAll(/[^a-z0-9]+/g, '-')
    .replaceAll(/^-+|-+$/g, '');
  if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(anchor)) {
    throw new Error(`Reference entry ${name} needs an explicit lowercase URL-safe anchor.`);
  }
  return anchor;
}

export function parseReferenceRoute(hash) {
  const raw = hash.startsWith('#') ? hash.slice(1) : hash;
  if (!raw) return { pageID: 'overview', entryAnchor: null };
  const slash = raw.indexOf('/');
  const rawPageID = slash < 0 ? raw : raw.slice(0, slash);
  const rawEntryAnchor = slash < 0 ? '' : raw.slice(slash + 1);
  const decodedPageID = safeDecode(rawPageID) || 'overview';
  return {
    pageID: pageAliases.get(decodedPageID) || decodedPageID,
    entryAnchor: rawEntryAnchor ? safeDecode(rawEntryAnchor) : null,
  };
}

export function formatReferenceRoute(pageID, anchor = null) {
  const normalizedPageID = pageAliases.get(pageID) || pageID || 'overview';
  return `#${safeEncode(normalizedPageID)}${anchor ? `/${safeEncode(anchor)}` : ''}`;
}

export function validateReferenceAnchors(pageID, sections) {
  const seen = new Map();
  for (const section of sections) {
    for (const entry of section.entries) {
      const anchor = entryAnchor(entry);
      const previous = seen.get(anchor);
      if (previous) {
        throw new Error(
          `Reference anchor collision on ${pageID}: ${previous.name} and ${entry.name} both use "${anchor}". Add an explicit stable anchor.`,
        );
      }
      seen.set(anchor, entry);
    }
  }
  return seen;
}

export function observeReferenceRoute(windowObject, onRoute) {
  const synchronize = () => onRoute(parseReferenceRoute(windowObject.location.hash));
  windowObject.addEventListener('hashchange', synchronize);
  return () => windowObject.removeEventListener('hashchange', synchronize);
}

export function focusReferenceEntry(documentObject, anchor) {
  if (!anchor) return false;
  const target = documentObject.getElementById(anchor);
  if (!target) return false;
  target.scrollIntoView({ block: 'start' });
  target.focus({ preventScroll: true });
  return true;
}
