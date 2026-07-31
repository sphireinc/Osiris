import { mkdir, rm, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const output = resolve(here, '..', 'public', 'docs');
const routes = {
  'index.html': 'overview', 'core.html': 'core', 'text.html': 'text',
  'math.html': 'math', 'arrays.html': 'arrays', 'tensor.html': 'tensor',
  'stats.html': 'stats', 'io.html': 'io', 'data.html': 'data',
  'crypto.html': 'crypto', 'list.html': 'list', 'types.html': 'types',
  'semantics.html': 'chapter-semantics', 'guide.html': 'chapter-guide',
  'errors.html': 'chapter-errors', 'native.html': 'chapter-native',
  'support.html': 'support',
};

await rm(output, { recursive: true, force: true });
await mkdir(output, { recursive: true });
for (const [file, route] of Object.entries(routes)) {
  const html = `<!doctype html><meta charset="utf-8"><meta http-equiv="refresh" content="0; url=../#${route}"><title>Osiris Reference</title><p>This reference moved to <a href="../#${route}">the Osiris React reference</a>.</p>`;
  await writeFile(resolve(output, file), html);
}
console.log(`Created ${Object.keys(routes).length} route redirects.`);
