export function CodeBlock({ children, label = 'EXAMPLE', className = '' }) {
  return <div className={`code-block ${className}`.trim()}>
    {label && <div className="example-label">{label}</div>}
    <pre><code>{children}</code></pre>
  </div>;
}
