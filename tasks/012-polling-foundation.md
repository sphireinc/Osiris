# Polling foundation and platform adapters

Source: POLLING-CHK.txt.

The checklist claims a polling API but no implementation exists. First define
a small internal context/registration/wait interface and one portable backend.
Only then add build-tagged kqueue, epoll, and Windows adapters; keep poll fallback.

Acceptance: context lifecycle, registration/removal, timeout, readiness events,
and unsupported-backend errors are tested; selected backend/capabilities are
inspectable and platform tests skip correctly.
