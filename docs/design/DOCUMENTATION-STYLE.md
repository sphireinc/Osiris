# Documentation style and examples

Every public function, language construct, module, type, compiler target, and
configuration component documented in `newdocs/` must include at least one
copyable usage example. The example belongs immediately after the contract and
error information, and shows idiomatic Osiris source rather than pseudocode.

Function examples must show a realistic call and, where deterministic, its
observable result. Component examples must show setup plus one meaningful use.
When a feature needs an external dependency (for example a numerical provider),
the example must state that requirement and include the relevant build command.

Examples are executable claims: when a documented behavior changes, update its
example and add or amend its automated test in the same change. Do not replace
an example with a bare list of names, a signature, or prose paraphrase.
