# Documentation consolidation

Source: README.txt, LANG.txt, IO.txt, and docs/.

Choose documentation sources of truth, migrate durable content, and correct
claims the implementation does not meet—especially native C lowering and polling.
Keep one root README in a conventional filename.

Acceptance: no competing reference, GitHub Pages and root links work, and the
command list matches osiris help.

## Completion record — 2026-07-28

Tasks 039 and 040 completed and verified this work. `newdocs/` is the sole
canonical componentized public reference and GitHub Pages source; `docs/` is
frozen compatibility material and durable non-public design documentation.
The React reference has checked examples, searchable detailed API contracts,
redirect coverage, and CI build/check scripts. `README.txt` remains an
intentional fixed-width plain-text README by project decision, so the obsolete
“conventional filename” wording above is not an outstanding requirement.
