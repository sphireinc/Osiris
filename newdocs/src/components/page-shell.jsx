export function PageShell({ children, selected, onSelect, modules, chapters, query, onQuery }) {
  return <>
    <header className="topbar">
      <button className="brand" onClick={() => onSelect('overview')}><span className="mark">O</span><span>OSIRIS</span><small>Reference</small></button>
      <label className="search"><span>⌕</span><input value={query} onChange={(event) => onQuery(event.target.value)} placeholder="Search API reference" /><kbd>/</kbd></label>
      <a href="https://github.com/sphireinc/osiris">Source</a>
    </header>
    <div className="layout">
      <aside className="sidebar"><div className="side-head"><strong>REFERENCE</strong><span>v0.1</span></div><nav className="side-nav" aria-label="Reference modules"><p>REFERENCE</p><button className={selected === 'overview' ? 'active' : ''} onClick={() => onSelect('overview')}>Overview</button><button className={selected === 'support' ? 'active' : ''} onClick={() => onSelect('support')}>Backend support</button><p>CHAPTERS</p>{chapters.map((chapter) => <button key={chapter.id} className={selected === `chapter-${chapter.id}` ? 'active' : ''} onClick={() => onSelect(`chapter-${chapter.id}`)}>{chapter.title}</button>)}<p>API</p>{modules.map((module) => <button key={module.id} className={selected === module.id ? 'active' : ''} onClick={() => onSelect(module.id)}>{module.label}</button>)}<p>LANGUAGE</p><button className={selected === 'language' ? 'active' : ''} onClick={() => onSelect('language')}>Syntax & semantics</button></nav></aside>
      <main className="content">{children}</main>
    </div>
  </>;
}
