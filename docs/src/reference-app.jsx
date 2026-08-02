import { useEffect, useMemo, useState } from 'react';
import { chapters, languageSections, modules, searchEntries } from './api-data.js';
import supportMatrix from './support-matrix.json' with { type: 'json' };
import { ApiListing } from './components/api-listing.jsx';
import { CodeBlock } from './components/code-block.jsx';
import { InlineCode } from './components/inline-code.jsx';
import { PageShell } from './components/page-shell.jsx';
import { PageTableOfContents } from './components/page-table-of-contents.jsx';
import { TechnicalReference } from './components/technical-reference.jsx';
import {
  entryAnchor,
  focusReferenceEntry,
  formatReferenceRoute,
  observeReferenceRoute,
  parseReferenceRoute,
} from './reference-route.js';

function titleCase(title) {
  const smallWords = new Set(['a', 'an', 'and', 'as', 'at', 'by', 'for', 'in', 'of', 'on', 'or', 'the', 'to', 'with']);
  const words = title.split(' ');
  return words.map((word, index) => {
    if (word.includes('/') || word === word.toUpperCase()) return word;
    const bare = word.replace(/[^a-z]/gi, '');
    if (index > 0 && index < words.length - 1 && smallWords.has(bare.toLowerCase())) return word.toLowerCase();
    return word.charAt(0).toUpperCase() + word.slice(1);
  }).join(' ');
}

function Overview({ onSelect }) {
  return <>
    <section className="hero hero-overview"><div className="hero-copy"><div className="eyebrow">TECHNICAL LANGUAGE REFERENCE</div><h1>Osiris</h1><p className="lede">A fast general-purpose language for numerical computing, expressive data work, and native C compilation.</p></div><img className="hero-logo" src="brand/osiris_logo_trans_cropped.png" alt="Osiris logo" /></section>
    <section><h2>Reference modules</h2><div className="summary-grid">{modules.map((module) => <button key={module.id} onClick={() => onSelect(module.id)}><strong>{module.label}</strong><span>{module.summary}</span></button>)}</div></section>
    <section><h2>Native compilation</h2><article className="api-entry"><h3>IR → C → executable</h3><p>Osiris lowers supported programs from its IR into generated C and invokes a system C compiler. It is not a captured-output or interpreter snapshot build.</p><CodeBlock>{'osiris build program.osr\n./program'}</CodeBlock></article></section>
  </>;
}

function LanguagePage({ currentAnchor }) {
  return <>
    <section className="hero"><div className="eyebrow">LANGUAGE REFERENCE</div><h1>{titleCase('Syntax and semantics')}</h1><p className="lede">Core declarations, fallibility, control flow, and the evolving enum/pattern surface.</p></section>
    <PageTableOfContents pageID="language" pageLabel="Syntax and Semantics" sections={languageSections} currentAnchor={currentAnchor} />
    {languageSections.map((section) => <section key={section.title}><h2>{section.title}</h2>{section.entries.map((entry) => <ApiListing key={entry.name} entry={entry} />)}</section>)}
    <section><h2>Current pattern boundary</h2><article className="api-entry"><p>Qualified enum, list, literal, relational, type, grouped, conjunctive, negated, guarded, binding, and wildcard patterns are current syntax. Record-destructuring patterns are not yet part of the language.</p><CodeBlock label="NON-CURRENT SYNTAX">{'let text = match result {\n  Point { x: > 0, y: > 0 } => "quadrant I",\n}'}</CodeBlock></article></section>
 </>;
}

function ChapterPage({ chapter }) {
  return <>
    <section className="hero"><div className="eyebrow">LANGUAGE CHAPTER</div><h1>{titleCase(chapter.title)}</h1><p className="lede">{chapter.summary}</p></section>
    {chapter.paragraphs?.length > 0 && <section>{chapter.paragraphs.map((paragraph) => <p key={paragraph}><InlineCode>{paragraph}</InlineCode></p>)}</section>}
    {chapter.cards?.length > 0 && <section><h2>Reference notes</h2><div className="summary-grid">{chapter.cards.map((card) => <article className="chapter-card" key={card.title}><h3>{card.title}</h3>{card.code && <CodeBlock>{card.code}</CodeBlock>}<p><InlineCode>{card.text}</InlineCode></p></article>)}</div></section>}
  </>;
}

function SupportPage() {
  return <><section className="hero"><div className="eyebrow">CURRENT CONTRACT</div><h1>{titleCase('Backend support matrix')}</h1><p className="lede">Feature-specific support across the parser, interpreter, VM, native C, WASM, GPU, formatter, and LSP.</p></section><section className="support-list">{supportMatrix.features.map((feature) => <article className="support-card" key={feature.id}><h2>{titleCase(feature.label)}</h2><div className="support-grid" aria-label={`${feature.label} support`}><div className="support-grid-row support-grid-head">{supportMatrix.backends.map((backend) => <div key={backend}>{backend.replace('_', ' ')}</div>)}</div><div className="support-grid-row">{supportMatrix.backends.map((backend) => <div key={backend}><span className={`support-state ${feature.support[backend]}`}>{feature.support[backend]}</span></div>)}</div></div><footer><strong>Limit</strong><p><InlineCode>{feature.notes}</InlineCode></p></footer></article>)}</section></>;
}

function ModulePage({ module, currentAnchor }) {
  const sections = module.sections;
  const referenceLabel = { core: 'LANGUAGE REFERENCE', types: 'LANGUAGE REFERENCE', 'global-constants': 'LANGUAGE REFERENCE', 'error-constants': 'LANGUAGE REFERENCE', arrays: 'DATA & NUMERICS REFERENCE', tensor: 'DATA & NUMERICS REFERENCE', stats: 'DATA & NUMERICS REFERENCE', data: 'DATA & NUMERICS REFERENCE', io: 'I/O REFERENCE', crypto: 'STANDARD LIBRARY REFERENCE', list: 'STANDARD LIBRARY REFERENCE', math: 'STANDARD LIBRARY REFERENCE', net: 'STANDARD LIBRARY REFERENCE', os: 'STANDARD LIBRARY REFERENCE', signal: 'STANDARD LIBRARY REFERENCE', text: 'STANDARD LIBRARY REFERENCE', time: 'STANDARD LIBRARY REFERENCE', tooling: 'TOOLING REFERENCE' }[module.id] || 'MODULE REFERENCE';
  return <>
    <section className="hero"><div className="eyebrow">{referenceLabel} · {module.label}</div><h1>{titleCase(module.label)}</h1><p className="lede"><InlineCode>{module.summary}</InlineCode></p><p className="import-line"><code className="inline-code">{module.importLine}</code></p></section>
    <PageTableOfContents pageID={module.id} pageLabel={module.label} sections={sections} currentAnchor={currentAnchor} />
    {sections.map((section) => <section key={section.title}><h2>{section.title}</h2>{section.entries.map((entry) => <ApiListing key={entry.name} entry={entry} />)}</section>)}
  </>;
}

function SearchResults({ query, results, onNavigate }) {
  return <>
    <section className="hero"><div className="eyebrow">REFERENCE SEARCH</div><h1>{titleCase('Search results')}</h1><p className="lede">{results.length} result{results.length === 1 ? '' : 's'} for “{query}” across the complete React reference.</p></section>
    <section>{results.map(({ module, section, entry }) => {
      const href = module.id === 'technical'
        ? formatReferenceRoute(module.id)
        : formatReferenceRoute(module.id, entryAnchor(entry));
      return <article className="search-result" key={`${module.id}-${section.title}-${entry.name}`}><a href={href} onClick={onNavigate}><span>{module.label} · {section.title}</span><strong>{entry.name}</strong></a><code className="inline-code">{entry.signature}</code><p><InlineCode>{entry.description}</InlineCode></p></article>;
    })}{!results.length && <div className="empty"><h2>No matching API entries matched</h2><p>Search function names, signatures, return types, errors, or example code.</p></div>}</section>
  </>;
}

function UnknownPage({ pageID }) {
  return <>
    <section className="hero"><div className="eyebrow">REFERENCE</div><h1>Page Not Found</h1><p className="lede">There is no reference page named <code className="inline-code">{pageID}</code>.</p></section>
  </>;
}

export function ReferenceApp() {
  const [route, setRoute] = useState(() => parseReferenceRoute(window.location.hash));
  const [query, setQuery] = useState('');
  const module = modules.find((item) => item.id === route.pageID);
  const chapter = chapters.find((item) => `chapter-${item.id}` === route.pageID);
  const normalizedQuery = query.trim().toLowerCase();
  const results = useMemo(() => searchEntries(normalizedQuery), [normalizedQuery]);
  useEffect(() => observeReferenceRoute(window, setRoute), []);
  useEffect(() => {
    if (normalizedQuery) return undefined;
    const frame = window.requestAnimationFrame(() => {
      if (!route.entryAnchor) {
        window.scrollTo({ top: 0 });
        return;
      }
      focusReferenceEntry(document, route.entryAnchor);
    });
    return () => window.cancelAnimationFrame(frame);
  }, [normalizedQuery, route.entryAnchor, route.pageID]);
  const select = (id) => {
    setQuery('');
    const hash = formatReferenceRoute(id);
    if (window.location.hash === hash) {
      window.scrollTo({ top: 0 });
      return;
    }
    window.location.hash = hash.slice(1);
  };
  const page = normalizedQuery
    ? <SearchResults query={query.trim()} results={results} onNavigate={() => setQuery('')} />
    : route.pageID === 'overview'
      ? <Overview onSelect={select} />
      : route.pageID === 'support'
        ? <SupportPage />
        : route.pageID === 'language'
          ? <LanguagePage currentAnchor={route.entryAnchor} />
          : route.pageID === 'technical'
            ? <TechnicalReference />
            : chapter
              ? <ChapterPage chapter={chapter} />
              : module
                ? <ModulePage module={module} currentAnchor={route.entryAnchor} />
                : <UnknownPage pageID={route.pageID} />;
  return <PageShell selected={route.pageID} onSelect={select} modules={modules} chapters={chapters} query={query} onQuery={setQuery}>{page}</PageShell>;
}
