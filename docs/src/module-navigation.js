// Navigation is derived from the canonical API module list. Standard-library
// modules are identified by their public `std/` label, so adding a new module
// to api-data.js automatically makes it visible in the sidebar.
const fixedGroups = [
  { label: 'LANGUAGE', ids: ['core', 'types', 'global-constants', 'error-constants'], language: true },
  { label: 'DATA & NUMERICS', ids: ['arrays', 'tensor', 'stats', 'data'] },
  { label: 'I/O & CONCURRENCY', ids: ['io'] },
];

export function buildModuleNavigationGroups(modules) {
  const moduleByID = new Map(modules.map((module) => [module.id, module]));
  const groups = fixedGroups.map((group) => ({ ...group, ids: group.ids.filter((id) => moduleByID.has(id)) }));
  const assigned = new Set(groups.flatMap((group) => group.ids));
  const standardLibrary = modules
    .filter((module) => module.label.startsWith('std/') && module.id !== 'testing')
    .map((module) => module.id);
  groups.push({ label: 'STANDARD LIBRARY', ids: standardLibrary });
  standardLibrary.forEach((id) => assigned.add(id));
  groups.push({ label: 'TESTING', ids: modules.filter((module) => module.id === 'testing').map((module) => module.id) });
  assigned.add('testing');
  groups.push({ label: 'TOOLING', ids: modules.filter((module) => module.id === 'tooling').map((module) => module.id) });
  assigned.add('tooling');
  const unclassified = modules.filter((module) => !assigned.has(module.id)).map((module) => module.id);
  if (unclassified.length) groups.push({ label: 'OTHER MODULES', ids: unclassified });
  return groups;
}

export function validateModuleNavigationGroups(modules, groups) {
  const moduleIDs = new Set(modules.map((module) => module.id));
  const seen = new Set();
  for (const group of groups) {
    for (const id of group.ids) {
      if (!moduleIDs.has(id)) throw new Error(`navigation references missing module ${id}`);
      if (seen.has(id)) throw new Error(`module ${id} appears in multiple navigation groups`);
      seen.add(id);
    }
  }
  const missing = modules.filter((module) => !seen.has(module.id)).map((module) => module.id);
  if (missing.length) throw new Error(`modules missing from navigation: ${missing.join(', ')}`);
  return true;
}
