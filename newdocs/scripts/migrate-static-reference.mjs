import { readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const repositoryRoot = resolve(here, '..', '..');
const sourcePath = resolve(repositoryRoot, 'docs', 'content.js');
const outputPath = resolve(here, '..', 'src', 'legacy-reference.json');
const supportPath = resolve(repositoryRoot, 'docs', 'support-matrix.json');
const supportOutputPath = resolve(here, '..', 'src', 'support-matrix.json');
const source = await readFile(sourcePath, 'utf8');
const context = { globalThis: {} };

vm.runInNewContext(source.replace('window.OSIRIS_MANUAL =', 'globalThis.OSIRIS_MANUAL ='), context, { filename: sourcePath });

if (!context.globalThis.OSIRIS_MANUAL) throw new Error('Static manual did not define OSIRIS_MANUAL.');
await writeFile(outputPath, `${JSON.stringify(context.globalThis.OSIRIS_MANUAL, null, 2)}\n`);
await writeFile(supportOutputPath, await readFile(supportPath));
console.log(`Migrated ${context.globalThis.OSIRIS_MANUAL.groups.length} API groups and ${context.globalThis.OSIRIS_MANUAL.sections.length} chapters.`);
