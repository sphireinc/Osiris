import { spawnSync } from 'node:child_process';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const result = spawnSync('go', ['test', './internal/osiris', '-run', '^TestCanonicalDocumentationExamples$', '-count=1'], {
  cwd: root,
  stdio: 'inherit',
  env: {
    ...process.env,
    GOCACHE: process.env.GOCACHE || '/private/tmp/osiris-docs-gocache',
    GOMODCACHE: process.env.GOMODCACHE || '/private/tmp/osiris-docs-gomodcache',
    GOTOOLCHAIN: process.env.GOTOOLCHAIN || 'local',
  },
});
if (result.error) throw result.error;
process.exit(result.status ?? 1);
