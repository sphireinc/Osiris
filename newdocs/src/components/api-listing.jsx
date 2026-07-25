import { CodeBlock } from './code-block.jsx';

export function ApiListing({ entry }) {
  const id = entry.name.toLowerCase().replaceAll(/[^a-z0-9]+/g, '-');
  return <article className="api-entry" id={id}>
    <h3>{entry.name}</h3>
    <CodeBlock label="SIGNATURE" className="signature">{entry.signature}</CodeBlock>
    <p>{entry.description}</p>
    {entry.parameters && <p><strong>Parameters:</strong> {entry.parameters}</p>}
    {entry.returns && <p><strong>Return type:</strong> {entry.returns}</p>}
    {entry.returnValue && <p><strong>Return value:</strong> {entry.returnValue}</p>}
    {entry.details?.length > 1 && <ul className="api-details">{entry.details.slice(1).map((detail) => <li key={detail}>{detail}</li>)}</ul>}
    {entry.errors && <p><strong>Errors:</strong> {entry.errors}</p>}
    {entry.native && <p><strong>Native lowering:</strong> {entry.native}</p>}
    <CodeBlock label={entry.exampleStatus === 'checked' ? 'EXAMPLE · CHECKED' : entry.exampleStatus === 'sandbox-required' ? 'EXAMPLE · SANDBOX REQUIRED' : 'EXAMPLE · ILLUSTRATIVE'}>{entry.example}</CodeBlock>
  </article>;
}
