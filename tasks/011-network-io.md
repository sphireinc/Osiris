# Socket networking primitives

Source: ASYNC-PLAN-CHK.txt and IO.txt.

Define minimal TCP client/server capabilities with contexts, deadlines, close
rules, and structured errors. Do not expose polling internals yet.

Acceptance: loopback client/server integration tests, timeout/cancellation
tests, and documentation identify blocking and task-aware operations.
