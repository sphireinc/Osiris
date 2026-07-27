import { readdir, readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import { chapters, languageSections, modules, searchEntries } from '../src/api-data.js';

const here = dirname(fileURLToPath(import.meta.url));
const legacyDocs = resolve(here, '..', '..', 'docs');
const redirects = resolve(here, '..', 'public', 'docs');
const pages = (await readdir(legacyDocs)).filter((name) => name.endsWith('.html'));
const support = JSON.parse(await readFile(resolve(here, '..', 'src', 'support-matrix.json'), 'utf8'));
const moduleSource = await readFile(resolve(here, '..', '..', 'internal', 'osiris', 'module.go'), 'utf8');

if (modules.length !== 14 || chapters.length !== 6) throw new Error(`Unexpected reference coverage: ${modules.length} API groups, ${chapters.length} chapters.`);
if (!support.features?.length || !support.backends?.length) throw new Error('Migrated support matrix is incomplete.');
const referenceModules = [...modules, { id: 'language', sections: languageSections }];
for (const module of referenceModules) for (const section of module.sections) for (const entry of section.entries) {
  for (const field of ['name', 'signature', 'description', 'returns', 'returnValue', 'example']) if (!entry[field]) throw new Error(`Incomplete API entry ${module.id}/${entry.name}: ${field}`);
  if (entry.kind !== 'construct' && entry.kind !== 'type' && (!entry.parameters || !entry.details?.length)) throw new Error(`Function reference entry lacks parameter or behavioral detail: ${module.id}/${entry.name}.`);

  // The UI uses this same index. Every published API entry must be reachable
  // by searching its documented name, not merely by a handful of examples.
  if (!searchEntries(entry.name).some((result) => result.entry === entry)) {
    throw new Error(`Search does not find documented API entry ${module.id}/${entry.name}.`);
  }
	for (const alias of entry.aliases || []) if (!searchEntries(alias).some((result) => result.entry === entry)) {
		throw new Error(`Search does not find documented API alias ${module.id}/${alias}.`);
	}
}
const documentedNames = new Set(modules.flatMap((module) => module.sections.flatMap((section) => section.entries.flatMap((entry) => [entry.name, ...(entry.aliases || [])]))));
const stdlibTables = [...moduleSource.matchAll(/case "(std\/[^"]+)"[\s\S]*?return makeModule\([^,]+,[\s\S]*?map\[string\]Value\{([\s\S]*?)\}\), nil/g)];
if (!stdlibTables.length) throw new Error('Cannot locate stdlib export tables for reference coverage.');
for (const [, moduleName, table] of stdlibTables) {
	const exports = [...table.matchAll(/^\s*"([^"]+)":\s+functionValueOf/gm)].map((match) => match[1]);
	const undocumented = exports.filter((name) => !documentedNames.has(name));
	if (undocumented.length) throw new Error(`Canonical reference is missing ${moduleName} exports: ${undocumented.join(', ')}.`);
}
for (const query of ['cholesky', 'readchunk', 'end-of-file', 'uint64 byte count', 'non-byte-compatible', 'transfer', 'fixed-width integer', 'optimization report', 'convolve', 'tcp_dial', 'tcp_nodelay', 'let?', 'to_option', 'collect', 'discard', 'must_handle', 'permission_denied', 'redact']) if (!searchEntries(query).length) throw new Error(`Search does not find ${query}.`);
for (const page of pages) {
  const redirect = await readFile(resolve(redirects, page), 'utf8');
  if (!redirect.includes('url=../#')) throw new Error(`Missing React redirect for ${page}.`);
}
console.log(`Reference checks passed: ${modules.length} API groups, ${chapters.length} chapters, ${pages.length} redirects.`);
