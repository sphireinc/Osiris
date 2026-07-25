# Migration notes

Breaking releases add a section here before publication. Each section names
the first affected version, explains the old and new behavior, provides a
before/after source or command example, and lists a rollback/compatibility
option where one exists.

## Deprecation warnings

Warnings use stable codes of the form `OSIRIS-Dnnn`. Their message names the
replacement and the earliest removal major version. Users may suppress a
specific warning with `OSIRIS_SUPPRESS_WARNINGS=OSIRIS-Dnnn` (comma-separated)
or all deprecation warnings with `OSIRIS_SUPPRESS_WARNINGS=all`. Suppression
does not retain removed behavior and should be temporary.

There are no active source or CLI deprecations in 0.1.0. This file is the
required destination for the first breaking migration.
