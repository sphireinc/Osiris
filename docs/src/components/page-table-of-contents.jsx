import { entryAnchor, formatReferenceRoute } from '../reference-route.js';

export function PageTableOfContents({ pageID, pageLabel, sections, currentAnchor }) {
  return <nav className="page-toc" aria-label={`${pageLabel} entries`}>
    <h2>On This Page</h2>
    <div className="page-toc-sections">
      {sections.map((section) => <div className="page-toc-section" key={section.title}>
        {sections.length > 1 && <h3>{section.title}</h3>}
        <ul className="page-toc-list">
          {section.entries.map((entry) => {
            const anchor = entryAnchor(entry);
            return <li key={entry.name}>
              <a
                aria-current={currentAnchor === anchor ? 'location' : undefined}
                href={formatReferenceRoute(pageID, anchor)}
              >
                {entry.name}
              </a>
            </li>;
          })}
        </ul>
      </div>)}
    </div>
  </nav>;
}
