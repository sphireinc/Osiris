import Prism from 'prismjs';
import { osirisGrammar } from '../osiris-prism.js';

export function CodeBlock({ children, label = 'Example', className = '' }) {
  const code = String(children);
  const html = Prism.highlight(code, osirisGrammar, 'osiris');
  return <div className={`code-block ${className}`.trim()}>
    {label && <div className="example-label">{label}</div>}
    <pre><code className="language-osiris" dangerouslySetInnerHTML={{ __html: html }} /></pre>
  </div>;
}
