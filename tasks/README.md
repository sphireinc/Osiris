# Osiris task backlog

This directory holds the remaining implementation work extracted from the root
roadmaps. The numbered files are focused implementation outcomes. The 46
async, error, polling, and type files are semi-grouped active tasks containing
every remaining unchecked source-roadmap item. `roadmaps/` preserves source
checklists only; see `PRIORITY.md` for the required native-compiler order.

## Reconciliation

- The C backend currently evaluates a program and emits C that prints those
  captured outputs. It is transitional: Osiris is moving to general C lowering.
- Interpreter, VM, parser, optimizer, IR/SSA/CFG analyses, modules, structured
  errors, task primitives, and artifact backends have automated tests.
- Polling, sockets, FFI, LLVM/JIT, and most native performance work are not
  implemented. Their remaining work is split into the semi-grouped task files.

Source roadmaps remain for historical design context until their useful prose is
consolidated into docs or tasks.
