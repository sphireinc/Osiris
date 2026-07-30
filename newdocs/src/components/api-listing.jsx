import { CodeBlock } from './code-block.jsx';
import { InlineCode } from './inline-code.jsx';

export function ApiListing({ entry }) {
  const id = entry.name.toLowerCase().replaceAll(/[^a-z0-9]+/g, '-');
  return <article className="api-entry" id={id}>
    <h3>{entry.name}</h3>
    <CodeBlock label="SIGNATURE" className="signature">{entry.signature}</CodeBlock>
    <p><InlineCode>{entry.description}</InlineCode></p>
    {entry.parameters && <p><strong>Parameters:</strong> <InlineCode>{entry.parameters}</InlineCode></p>}
    {entry.returns && <p><strong>Return type:</strong> <InlineCode>{entry.returns}</InlineCode></p>}
    {entry.returnValue && <p><strong>Return value:</strong> <InlineCode>{entry.returnValue}</InlineCode></p>}
    {entry.details?.length > 1 && <ul className="api-details">{entry.details.slice(1).map((detail) => <li key={detail}><InlineCode>{detail}</InlineCode></li>)}</ul>}
    {entry.errors && <p><strong>Errors:</strong> <InlineCode>{entry.errors}</InlineCode></p>}
    {entry.native && <p><strong>Native lowering:</strong> <InlineCode>{entry.native}</InlineCode></p>}
    <CodeBlock>{entry.example}</CodeBlock>
  </article>;
}
