import { readdir, readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import { chapters, modules, searchEntries } from '../src/api-data.js';

const here = dirname(fileURLToPath(import.meta.url));
const legacyDocs = resolve(here, '..', '..', 'docs');
const redirects = resolve(here, '..', 'public', 'docs');
const pages = (await readdir(legacyDocs)).filter((name) => name.endsWith('.html'));
const support = JSON.parse(await readFile(resolve(here, '..', 'src', 'support-matrix.json'), 'utf8'));

if (modules.length !== 13 || chapters.length !== 6) throw new Error(`Unexpected reference coverage: ${modules.length} API groups, ${chapters.length} chapters.`);
if (!support.features?.length || !support.backends?.length) throw new Error('Migrated support matrix is incomplete.');
for (const module of modules) for (const section of module.sections) for (const entry of section.entries) {
  for (const field of ['name', 'signature', 'description', 'returns', 'returnValue', 'example']) if (!entry[field]) throw new Error(`Incomplete API entry ${module.id}/${entry.name}: ${field}`);
}
for (const query of ['cholesky', 'readchunk', 'fixed-width integer', 'optimization report', 'convolve']) if (!searchEntries(query).length) throw new Error(`Search does not find ${query}.`);
for (const page of pages) {
  const redirect = await readFile(resolve(redirects, page), 'utf8');
  if (!redirect.includes('url=../#')) throw new Error(`Missing React redirect for ${page}.`);
}
console.log(`Reference checks passed: ${modules.length} API groups, ${chapters.length} chapters, ${pages.length} redirects.`);
