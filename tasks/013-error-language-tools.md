# Error syntax, combinators, and static analysis

Source: ERROR-CHK.txt.

Design one concise Result binding form compatible with try/catch and postfix ?.
Then add map, map-error, flat-map, conversions, collect, must-handle metadata,
explicit discard syntax, and diagnostics.

Acceptance: grammar/formatter/evaluator/docs agree; metadata and causes survive
helpers; diagnostics carry spans and do not flag APIs without a fallibility contract.
