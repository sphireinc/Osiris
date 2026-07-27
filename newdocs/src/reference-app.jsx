import { useEffect, useMemo, useState } from 'react';
import { chapters, languageSections, modules, searchEntries } from './api-data.js';
import supportMatrix from './support-matrix.json' with { type: 'json' };
import { ApiListing } from './components/api-listing.jsx';
import { CodeBlock } from './components/code-block.jsx';
import { PageShell } from './components/page-shell.jsx';

function Overview({ onSelect }) {
  return <>
    <section className="hero"><div className="eyebrow">TECHNICAL LANGUAGE REFERENCE</div><h1>Osiris</h1><p className="lede">A fast general-purpose language for numerical computing, expressive data work, and native C compilation.</p></section>
    <section><h2>Reference modules</h2><div className="summary-grid">{modules.map((module) => <button key={module.id} onClick={() => onSelect(module.id)}><strong>{module.label}</strong><span>{module.summary}</span></button>)}</div></section>
    <section><h2>Native compilation</h2><article className="api-entry"><h3>IR → C → executable</h3><p>Osiris lowers supported programs from its IR into generated C and invokes a system C compiler. It is not a captured-output or interpreter snapshot build.</p><CodeBlock>{'osiris build program.osi\n./program'}</CodeBlock></article></section>
  </>;
}

function LanguagePage() {
  return <>
    <section className="hero"><div className="eyebrow">LANGUAGE REFERENCE</div><h1>Syntax and semantics</h1><p className="lede">Core declarations, fallibility, control flow, and the evolving enum/pattern surface.</p></section>
    {languageSections.map((section) => <section key={section.title}><h2>{section.title}</h2>{section.entries.map((entry) => <ApiListing key={entry.name} entry={entry} />)}</section>)}
    <section><h2>Planned extensions</h2><article className="api-entry"><p>Broader C#-style structural patterns and native tagged-value lowering are still in progress. They require full formatter, LSP, interpreter, VM, and native parity.</p><CodeBlock label="TARGET SYNTAX">{'let text = match result {\n  Point { x: > 0, y: > 0 } => "quadrant I",\n}'}</CodeBlock></article></section>
 </>;
}

function ChapterPage({ chapter }) {
  return <>
    <section className="hero"><div className="eyebrow">LANGUAGE CHAPTER</div><h1>{chapter.title}</h1><p className="lede">{chapter.summary}</p></section>
    {chapter.paragraphs?.length > 0 && <section>{chapter.paragraphs.map((paragraph) => <p key={paragraph}>{paragraph}</p>)}</section>}
    {chapter.cards?.length > 0 && <section><h2>Reference notes</h2><div className="summary-grid">{chapter.cards.map((card) => <article className="chapter-card" key={card.title}><h3>{card.title}</h3><p>{card.text}</p></article>)}</div></section>}
  </>;
}

function SupportPage() {
  return <><section className="hero"><div className="eyebrow">CURRENT CONTRACT</div><h1>Backend support matrix</h1><p className="lede">Feature-specific support across the parser, interpreter, VM, native C, WASM, GPU, formatter, and LSP.</p></section><section className="support-table"><table><thead><tr><th>Feature</th>{supportMatrix.backends.map((backend) => <th key={backend}>{backend.replace('_', ' ')}</th>)}<th>Limit</th></tr></thead><tbody>{supportMatrix.features.map((feature) => <tr key={feature.id}><th>{feature.label}</th>{supportMatrix.backends.map((backend) => <td key={backend}><span className={`support-state ${feature.support[backend]}`}>{feature.support[backend]}</span></td>)}<td>{feature.notes}</td></tr>)}</tbody></table></section></>;
}

function ModulePage({ module, query }) {
  const sections = useMemo(() => module.sections.map((section) => ({ ...section, entries: section.entries.filter((entry) => `${entry.name} ${entry.signature} ${entry.description}`.toLowerCase().includes(query)) })).filter((section) => section.entries.length), [module, query]);
  return <>
    <section className="hero"><div className="eyebrow">MODULE REFERENCE · {module.label}</div><h1>{module.label}</h1><p className="lede">{module.summary}</p><pre className="import-line"><code>{module.importLine}</code></pre></section>
    {sections.map((section) => <section key={section.title}><h2>{section.title}</h2>{section.entries.map((entry) => <ApiListing key={entry.name} entry={entry} />)}</section>)}
    {!sections.length && <section className="empty"><h2>No matching API entries</h2><p>Try a function name, argument type, or concept.</p></section>}
  </>;
}

function SearchResults({ query, results, onSelect }) {
  return <>
    <section className="hero"><div className="eyebrow">REFERENCE SEARCH</div><h1>Search results</h1><p className="lede">{results.length} result{results.length === 1 ? '' : 's'} for “{query}” across the complete React reference.</p></section>
    <section>{results.map(({ module, section, entry }) => <article className="search-result" key={`${module.id}-${section.title}-${entry.name}`}><button onClick={() => onSelect(module.id)}><span>{module.label} · {section.title}</span><strong>{entry.name}</strong></button><code>{entry.signature}</code><p>{entry.description}</p></article>)}{!results.length && <div className="empty"><h2>No API entries matched</h2><p>Search function names, signatures, return types, errors, or example code.</p></div>}</section>
  </>;
}

export function ReferenceApp() {
  const [selected, setSelected] = useState(() => window.location.hash.slice(1) || 'overview');
  const [query, setQuery] = useState('');
  const module = modules.find((item) => item.id === selected);
  const chapter = chapters.find((item) => `chapter-${item.id}` === selected);
  const normalizedQuery = query.trim().toLowerCase();
  const results = useMemo(() => searchEntries(normalizedQuery), [normalizedQuery]);
  useEffect(() => {
    const syncHash = () => setSelected(window.location.hash.slice(1) || 'overview');
    window.addEventListener('hashchange', syncHash);
    return () => window.removeEventListener('hashchange', syncHash);
  }, []);
  const select = (id) => { window.location.hash = id; };
  const selectResult = (id) => { select(id); setQuery(''); };
  const page = normalizedQuery ? <SearchResults query={query.trim()} results={results} onSelect={selectResult} /> : selected === 'overview' ? <Overview onSelect={select} /> : selected === 'support' ? <SupportPage /> : selected === 'language' ? <LanguagePage /> : chapter ? <ChapterPage chapter={chapter} /> : <ModulePage module={module} query="" />;
  return <PageShell selected={selected} onSelect={select} modules={modules} chapters={chapters} query={query} onQuery={setQuery}>{page}</PageShell>;
}
