export function InlineCode({ children }) {
  const text = String(children ?? '');
  return <>{text.split(/(`[^`]+`)/).map((part, index) => part.startsWith('`') && part.endsWith('`') ? <code className="inline-code" key={index}>{part.slice(1, -1)}</code> : part)}</>;
}
