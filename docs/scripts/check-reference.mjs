import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import { chapters, languageSections, modules, searchEntries } from '../src/api-data.js';
import { compareReferenceNames, orderReferenceEntries, orderReferenceSections } from '../src/reference-order.js';
import baseLanguageSections from '../src/language-reference.json' with { type: 'json' };
import manual from '../src/reference-data.json' with { type: 'json' };
import supplemental from '../src/supplemental-reference.json' with { type: 'json' };
import technical from '../src/technical-reference.json' with { type: 'json' };
import constants from '../src/constant-reference.json' with { type: 'json' };

const here = dirname(fileURLToPath(import.meta.url));
const support = JSON.parse(await readFile(resolve(here, '..', 'src', 'support-matrix.json'), 'utf8'));
const moduleSource = await readFile(resolve(here, '..', '..', 'internal', 'osiris', 'module.go'), 'utf8');
const osSource = await readFile(resolve(here, '..', '..', 'internal', 'osiris', 'os_std.go'), 'utf8');
const timeSource = await readFile(resolve(here, '..', '..', 'internal', 'osiris', 'time_value.go'), 'utf8');
const runtimeSource = await readFile(resolve(here, '..', '..', 'internal', 'osiris', 'runtime.go'), 'utf8');
const namedSource = await readFile(resolve(here, '..', '..', 'internal', 'osiris', 'named.go'), 'utf8');
const lexerSource = await readFile(resolve(here, '..', '..', 'internal', 'osiris', 'lexer.go'), 'utf8');
const valueSource = await readFile(resolve(here, '..', '..', 'internal', 'osiris', 'value.go'), 'utf8');
const cliSource = await readFile(resolve(here, '..', '..', 'cmd', 'osiris', 'main.go'), 'utf8');

if (modules.length !== 18 || chapters.length !== 6) throw new Error(`Unexpected reference coverage: ${modules.length} API groups, ${chapters.length} chapters.`);
if (!support.features?.length || !support.backends?.length) throw new Error('Migrated support matrix is incomplete.');
const referenceModules = [...modules, { id: 'language', sections: languageSections }];
for (const module of referenceModules) for (const section of module.sections) {
  for (let index = 1; index < section.entries.length; index += 1) {
    const previous = section.entries[index - 1];
    const current = section.entries[index];
    if (compareReferenceNames(previous.name, current.name) >= 0) {
      throw new Error(`Reference entries are not strictly ordered on ${module.id}/${section.title}: ${previous.name}, ${current.name}.`);
    }
  }
}

const duplicateDiagnostic = (() => {
  try {
    orderReferenceEntries([{ name: 'same' }, { name: 'same' }], 'duplicate-page', 'Duplicate section');
    return '';
  } catch (error) {
    return error.message;
  }
})();
if (!duplicateDiagnostic.includes('duplicate-page/Duplicate section') || !duplicateDiagnostic.includes('same')) {
  throw new Error(`Duplicate reference sort identities lack an actionable diagnostic: ${duplicateDiagnostic || '<none>'}.`);
}

const sourceOrderSnapshot = JSON.stringify({
  language: baseLanguageSections,
  groups: manual.groups,
  supplementalGroups: supplemental.groups,
});
const syntheticSections = [{
  title: 'Mixed source fixture',
  entries: [{ name: 'Zulu' }, { name: 'alpha' }, { name: 'Beta' }],
}];
const firstSyntheticOrder = orderReferenceSections(syntheticSections, 'synthetic');
const secondSyntheticOrder = orderReferenceSections(syntheticSections, 'synthetic');
if (firstSyntheticOrder[0].entries.map((entry) => entry.name).join('|') !== 'alpha|Beta|Zulu') {
  throw new Error('Shared reference comparator does not follow lowercase/code-point/original-name ordering.');
}
if (JSON.stringify(firstSyntheticOrder) !== JSON.stringify(secondSyntheticOrder)) {
  throw new Error('Repeated reference construction does not produce the same order.');
}
if (syntheticSections[0].entries.map((entry) => entry.name).join('|') !== 'Zulu|alpha|Beta') {
  throw new Error('Reference ordering mutated its source array.');
}
if (sourceOrderSnapshot !== JSON.stringify({
  language: baseLanguageSections,
  groups: manual.groups,
  supplementalGroups: supplemental.groups,
})) {
  throw new Error('Canonical reference assembly mutated imported JSON arrays.');
}

function assertEntryOrder(moduleID, sectionTitle, expectedNames) {
  const section = referenceModules.find((module) => module.id === moduleID)?.sections
    .find((candidate) => candidate.title === sectionTitle);
  if (!section) throw new Error(`Missing ordering fixture section ${moduleID}/${sectionTitle}.`);
  const positions = expectedNames.map((name) => section.entries.findIndex((entry) => entry.name === name));
  if (positions.some((position) => position < 0) || positions.some((position, index) => index > 0 && position <= positions[index - 1])) {
    throw new Error(`Unexpected reference order on ${moduleID}/${sectionTitle}: wanted ${expectedNames.join(', ')}.`);
  }
}

assertEntryOrder('math', 'std/math', ['bfN', 'iN', 'ufN', 'uN']);
assertEntryOrder('global-constants', 'Mathematical Constants', ['Apery', 'Catalan', 'E', 'EulerGamma']);
assertEntryOrder('error-constants', 'Error Kinds', ['AlreadyExists', 'Cancelled', 'Conflict', 'DatabaseError']);
assertEntryOrder('tooling', 'Compilation and Artifacts', ['osiris build', 'osiris c', 'osiris compile', 'osiris gpu']);
assertEntryOrder('core', 'Core language constructs', ['emit', 'export', 'for', 'format']);

const coreSourceNames = new Set([
  ...manual.groups.find((group) => group.id === 'core').items.map((entry) => entry.name),
  ...supplemental.groups.find((group) => group.id === 'core').items.map((entry) => entry.name),
]);
const renderedCoreNames = referenceModules.find((module) => module.id === 'core').sections[0].entries.map((entry) => entry.name);
if (renderedCoreNames.length !== coreSourceNames.size || renderedCoreNames.some((name) => !coreSourceNames.has(name))) {
  throw new Error('Base and supplemental Core entries do not share one ordered rendered section.');
}

for (const module of referenceModules) for (const section of module.sections) for (const entry of section.entries) {
  for (const field of ['name', 'signature', 'description', 'returns', 'returnValue', 'example']) if (!entry[field]) throw new Error(`Incomplete API entry ${module.id}/${entry.name}: ${field}`);
  if ((entry.kind === undefined || entry.kind === 'function' || entry.kind === 'family') && (!entry.parameters || !entry.returns || !entry.details?.length)) {
    throw new Error(`Function reference entry lacks accepted values, return type, or behavioral detail: ${module.id}/${entry.name}.`);
  }

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
for (const section of languageSections) for (const entry of section.entries) {
	documentedNames.add(entry.name);
	for (const alias of entry.aliases || []) documentedNames.add(alias);
}
const stdlibTables = [...moduleSource.matchAll(/case "(std\/[^"]+)":([\s\S]*?)(?=\n\tcase "std\/|\n\tdefault:)/g)]
	.map(([, moduleName, block]) => {
		const table = block.match(/return makeModule\([^,]+,[\s\S]*?map\[string\]Value\{([\s\S]*?)\}\), nil/);
		return table ? [table[0], moduleName, table[1]] : null;
	})
	.filter(Boolean);
const timeTable = timeSource.match(/func stdlibTimeModule[\s\S]*?return makeModule\([^,]+,[\s\S]*?map\[string\]Value\{([\s\S]*?)\}\), nil/);
if (timeTable) stdlibTables.push([timeTable[0], 'std/time', timeTable[1]]);
if (!stdlibTables.length) throw new Error('Cannot locate stdlib export tables for reference coverage.');
const implementedStdlibExports = new Map();
for (const [, moduleName, table] of stdlibTables) {
	const exports = [...table.matchAll(/^\s*"([^"]+)":\s+(?:functionValueOf|contextFunctionValueOf|numberValue|durationValue|stringValue|timeLocationValueOf)/gm)].map((match) => match[1]);
	implementedStdlibExports.set(moduleName, exports);
	const undocumented = exports.filter((name) => !documentedNames.has(name));
	if (undocumented.length) throw new Error(`Canonical reference is missing ${moduleName} exports: ${undocumented.join(', ')}.`);
}

function sortedUnique(values) {
	return [...new Set(values)].sort((left, right) => left.localeCompare(right));
}

function assertNoDuplicates(label, values) {
	const seen = new Set();
	const duplicates = new Set();
	for (const value of values) {
		if (seen.has(value)) duplicates.add(value);
		seen.add(value);
	}
	if (duplicates.size) throw new Error(`${label} contains duplicates: ${sortedUnique(duplicates).join(', ')}.`);
}

function assertExactIndex(label, documented, implemented) {
	const wanted = sortedUnique(implemented);
	const got = sortedUnique(documented);
	const missing = wanted.filter((name) => !got.includes(name));
	const stale = got.filter((name) => !wanted.includes(name));
	if (missing.length || stale.length) {
		throw new Error(`${label} is out of sync.${missing.length ? ` Missing: ${missing.join(', ')}.` : ''}${stale.length ? ` Stale: ${stale.join(', ')}.` : ''}`);
	}
}

const osModuleBlock = osSource.slice(osSource.indexOf('func stdOSModule'), osSource.indexOf('// Keep these imports'));
const implementedStdOSExports = sortedUnique([
	...osModuleBlock.matchAll(/"([A-Za-z][A-Za-z0-9_]*)"\s*:\s*(?=(?:fn|functionValueOf|contextFunctionValueOf|intValue|stringValue|uintValue))/g),
	...osModuleBlock.matchAll(/mod\["([A-Za-z][A-Za-z0-9_]*)"\]\s*=/g),
].map((match) => match[1] || match[2]));
if (!implementedStdOSExports.length) throw new Error('Cannot locate std/os export table for reference coverage.');
const documentedStdOSExports = modules.find((module) => module.id === 'os')?.sections
	.flatMap((section) => section.entries.map((entry) => entry.name)) || [];
assertExactIndex('std/os per-symbol reference', documentedStdOSExports, implementedStdOSExports);

const runtimeBuiltins = [...runtimeSource.matchAll(/dst\["([^"]+)"\]\s*=/g)].map((match) => match[1]);
const undocumentedBuiltins = sortedUnique(runtimeBuiltins).filter((name) => !documentedNames.has(name));
if (undocumentedBuiltins.length) throw new Error(`Canonical reference is missing runtime built-ins: ${undocumentedBuiltins.join(', ')}.`);
assertExactIndex('Technical runtime built-in index', technical.runtimeBuiltins, runtimeBuiltins);

const namedConstants = [];
for (const match of namedSource.matchAll(/namedDefinition\("([^"]+)",\s*(?:nil|\[\]string\{([^}]*)\})/g)) {
	namedConstants.push(match[1]);
	for (const alias of (match[2] || '').matchAll(/"([^"]+)"/g)) namedConstants.push(alias[1]);
}
assertNoDuplicates('Implementation named-constant catalog', namedConstants);
const documentedConstantNames = [constants.global, constants.errors]
	.flatMap((page) => page.sections.flatMap((section) => section.entries.flatMap((entry) => entry.names)));
assertNoDuplicates('Detailed named-constant reference', documentedConstantNames);
assertExactIndex('Detailed named-constant reference', documentedConstantNames, namedConstants);
const undocumentedConstants = sortedUnique(namedConstants).filter((name) => !documentedNames.has(name));
if (undocumentedConstants.length) throw new Error(`Canonical reference is missing named constants: ${undocumentedConstants.join(', ')}.`);
assertExactIndex('Technical named-constant index', technical.namedConstants, namedConstants);
const collidingConstants = sortedUnique(namedConstants).filter((name) => runtimeBuiltins.includes(name));
if (collidingConstants.length) throw new Error(`Named constants collide with global callables or type constructors: ${collidingConstants.join(', ')}.`);
for (const section of constants.global.sections) {
	if (!section.source || !section.exactness) throw new Error(`Global constant section ${section.title} lacks source or representation metadata.`);
	for (const entry of section.entries) {
		for (const field of ['names', 'type', 'description', 'value', 'example']) if (!entry[field]?.length) {
			throw new Error(`Incomplete global constant entry ${section.title}/${entry.names?.[0] || '<unnamed>'}: ${field}.`);
		}
	}
}

const specialFormBlock = runtimeSource.slice(runtimeSource.indexOf('func specialFormName'), runtimeSource.indexOf('func builtinHasTaskEffect'));
const specialForms = [...specialFormBlock.matchAll(/case ([^:]+):/g)].flatMap((line) => [...line[1].matchAll(/"([^"]+)"/g)].map((match) => match[1]));
const technicalIndexes = technical.sections.find((section) => section.id === 'surface-indexes')?.indexes || [];
for (const moduleName of ['std/math', 'std/text', 'std/time', 'std/os']) {
	const index = technicalIndexes.find((candidate) => candidate.title === `${moduleName} exports`);
	const documented = index?.source ? technical[index.source] : index?.items || [];
	const implemented = moduleName === 'std/os' ? implementedStdOSExports : implementedStdlibExports.get(moduleName) || [];
	assertExactIndex(`Technical ${moduleName} export index`, documented, implemented);
}
const documentedSpecialForms = technicalIndexes.find((index) => index.title === 'Evaluator special forms')?.items || [];
assertExactIndex('Technical special-form index', documentedSpecialForms, specialForms);
const undocumentedSpecialForms = sortedUnique(specialForms).filter((name) => !documentedNames.has(name));
if (undocumentedSpecialForms.length) throw new Error(`Canonical reference is missing evaluator special forms: ${undocumentedSpecialForms.join(', ')}.`);

const keywordBlock = lexerSource.slice(lexerSource.indexOf('func (l *lexer) scanIdent'), lexerSource.indexOf('func (l *lexer) scanNumber'));
const keywords = [...keywordBlock.matchAll(/case "([^"]+)":/g)].map((match) => match[1]);
const documentedKeywords = technicalIndexes.find((index) => index.title === 'Reserved keywords')?.items || [];
assertExactIndex('Technical keyword index', documentedKeywords, keywords);

const valueKindBlock = valueSource.slice(valueSource.indexOf('func (k valueKind) String()'), valueSource.indexOf('func (v Value) Truthy()'));
const runtimeKinds = [...valueKindBlock.matchAll(/return "([a-z_]+)"/g)].map((match) => match[1]);
assertExactIndex('Technical runtime-kind index', technical.runtimeKinds, runtimeKinds);

const commandBlock = cliSource.slice(cliSource.indexOf('func run(ctx'), cliSource.indexOf('func runVersion'));
const commands = [...commandBlock.matchAll(/case "([a-z]+)"/g)].map((match) => match[1]);
const documentedCommands = technicalIndexes.find((index) => index.title === 'CLI commands')?.items || [];
assertExactIndex('Technical CLI command index', documentedCommands, commands);

const detailedEntriesByName = new Map();
for (const module of referenceModules) for (const section of module.sections) for (const entry of section.entries) {
	for (const name of [entry.name, ...(entry.aliases || [])]) {
		if (!detailedEntriesByName.has(name)) detailedEntriesByName.set(name, []);
		detailedEntriesByName.get(name).push(entry);
	}
}
for (const index of technicalIndexes) {
	const names = index.source ? technical[index.source] : index.items;
	for (const name of names) {
		const entries = detailedEntriesByName.get(name) || [];
		if (!entries.length) throw new Error(`${index.title} entry ${name} has no detailed reference outside the technical index.`);
		if (!entries.some((entry) => entry.signature && entry.description && entry.example)) {
			throw new Error(`${index.title} entry ${name} lacks a listing, description, or code example.`);
		}
		if ((index.title === 'Global built-in functions' || index.title === 'Evaluator special forms') && !entries.some((entry) => entry.parameters && entry.returns)) {
			throw new Error(`${index.title} entry ${name} lacks accepted values or a return type.`);
		}
	}
}

for (const query of ['cholesky', 'readchunk', 'end-of-file', 'uint64 byte count', 'non-byte-compatible', 'transfer', 'fixed-width integer', 'optimization report', 'convolve', 'tcp_dial', 'tcp_nodelay', 'let?', 'to_option', 'collect', 'discard', 'must_handle', 'permission_denied', 'redact', 'precedence', 'truthiness', 'race', 'read_async', 'Planck', 'InvalidArgument', 'clone', 'rotl', 'parse_duration', 'rfc3339_nano', 'timezone gap', 'datetime', 'equal_fold', 'split_after_n', 'Unicode code-point index']) if (!searchEntries(query).length) throw new Error(`Search does not find ${query}.`);
console.log(`Reference checks passed: ${modules.length} API groups, ${chapters.length} chapters, ${sortedUnique(runtimeBuiltins).length} built-ins, ${sortedUnique(specialForms).length} special forms, ${sortedUnique(namedConstants).length} named constants.`);
