import technicalReference from '../technical-reference.json' with { type: 'json' };
import { CodeBlock } from './code-block.jsx';
import { InlineCode } from './inline-code.jsx';

function TechnicalTable({ table }) {
  return <div className="technical-table-wrap">
    <h3>{table.title}</h3>
    <table className="technical-table">
      <thead><tr>{table.columns.map((column) => <th key={column}>{column}</th>)}</tr></thead>
      <tbody>{table.rows.map((row, rowIndex) => <tr key={`${table.title}-${rowIndex}`}>{row.map((cell, cellIndex) => <td key={cellIndex}><InlineCode>{cell}</InlineCode></td>)}</tr>)}</tbody>
    </table>
  </div>;
}

function TechnicalIndex({ index }) {
  const items = index.source ? technicalReference[index.source] : index.items;
  return <div className="technical-index">
    <h3>{index.title} <small>{items.length}</small></h3>
    <div className="technical-index-items">{items.map((item) => <code className="inline-code" key={item}>{item}</code>)}</div>
  </div>;
}

export function TechnicalReference() {
  return <>
    <section className="hero">
      <div className="eyebrow">TECHNICAL</div>
      <h1>{technicalReference.title}</h1>
      <p className="lede">{technicalReference.summary}</p>
    </section>
    <section className="technical-toc">
      <h2>Contents</h2>
      <ol>{technicalReference.sections.map((section) => <li key={section.id}><a href="#technical" onClick={(event) => { event.preventDefault(); document.getElementById(`technical-${section.id}`)?.scrollIntoView(); }}>{section.title}</a></li>)}</ol>
    </section>
    {technicalReference.sections.map((section) => <section className="technical-section" id={`technical-${section.id}`} key={section.id}>
      <h2>{section.title}</h2>
      <p className="technical-summary"><InlineCode>{section.summary}</InlineCode></p>
      {section.paragraphs?.map((paragraph) => <p key={paragraph}><InlineCode>{paragraph}</InlineCode></p>)}
      {section.syntax?.map((syntax) => <CodeBlock label={syntax.label} key={syntax.label}>{syntax.code}</CodeBlock>)}
      {section.tables?.map((table) => <TechnicalTable table={table} key={table.title} />)}
      {section.indexes?.map((index) => <TechnicalIndex index={index} key={index.title} />)}
    </section>)}
  </>;
}
