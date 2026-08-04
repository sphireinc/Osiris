import { buildModuleNavigationGroups } from '../module-navigation.js';

export function PageShell({ children, selected, onSelect, modules, chapters, query, onQuery }) {
  const moduleByID = new Map(modules.map((module) => [module.id, module]));
  const moduleGroups = [...buildModuleNavigationGroups(modules), { label: 'TECHNICAL', ids: [], technical: true }];
  return <>
    <header className="topbar">
      <button className="brand" onClick={() => onSelect('overview')}><img className="brand-logo" src="brand/osiris_simple_logo.png" alt="" /><span>OSIRIS</span><small>Reference</small></button>
      <label className="search"><span>⌕</span><input value={query} onChange={(event) => onQuery(event.target.value)} placeholder="Search API reference" /><kbd>/</kbd></label>
      <a href="https://github.com/sphireinc/osiris">Source</a>
    </header>
    <div className="layout">
      <aside className="sidebar"><div className="side-head"><strong>REFERENCE</strong><span>v0.1</span></div><nav className="side-nav" aria-label="Reference modules"><p>REFERENCE</p><button className={selected === 'overview' ? 'active' : ''} onClick={() => onSelect('overview')}>Overview</button><button className={selected === 'support' ? 'active' : ''} onClick={() => onSelect('support')}>Backend support</button><p>CHAPTERS</p>{chapters.map((chapter) => <button key={chapter.id} className={selected === `chapter-${chapter.id}` ? 'active' : ''} onClick={() => onSelect(`chapter-${chapter.id}`)}>{chapter.title}</button>)}{moduleGroups.map((group) => <div key={group.label}><p>{group.label}</p>{group.language && <button className={selected === 'language' ? 'active' : ''} onClick={() => onSelect('language')}>Syntax & Semantics</button>}{group.ids.map((id) => { const module = moduleByID.get(id); return module && <button key={module.id} className={selected === module.id ? 'active' : ''} onClick={() => onSelect(module.id)}>{module.label}</button>; })}{group.technical && <button className={selected === 'technical' ? 'active' : ''} onClick={() => onSelect('technical')}>Technical Reference</button>}</div>)}</nav></aside>
      <main className="content">{children}<footer className="site-footer"><img src="brand/logo_black_bg.png" alt="Osiris" /><div><strong>Osiris</strong><p>Deterministic language tooling for readable, inspectable programs.</p><a href="https://github.com/sphireinc/Osiris">View the source on GitHub</a></div></footer></main>
    </div>
  </>;
}
