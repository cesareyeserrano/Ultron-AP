# Implementation Brief: US-4

Feature: total-stabilization
Story: As an admin, I want session cookie security behavior to remain correct under TLS and proxied TLS, so that session transport remains hardened.
Trace: FR-4, AC-3

## 1. Feature Context
- A deep total stabilization and optimization pass for Ultron-AP covering runtime reliability, security hardening, automated test expansion, and documentation hygiene optimization.
- Primary actor: Admin (single Raspberry Pi owner/operator) and maintainers who evolve the codebase.
- Expected outcome: The system remains stable under backup and operational failures with observable error reporting, critical security/session paths are strongly validated, all core tests pass, and project documentation is reduced to a clear, non-duplicated, maintained set with controlled file lengths.

## 2. Acceptance Criteria
- Given direct TLS request, when login succeeds, then cookie includes secure semantics.
- Given trusted proxy TLS termination (`X-Forwarded-Proto=https`), when login succeeds, then cookie security behavior remains correct.

## 3. Test Cases to Satisfy
- TC-5: Validate TC-5 (Trace FR: FR-4)
- TC-6: Validate TC-6 (Trace FR: FR-4)

## 4. Scaffold References
- Interface: src/contracts/fr-4-the-system-must-strengthen-secur.js
- Test stub: tests/total-stabilization/generated/tc-4-validate-tc-4-behavior.test.mjs

## 5. Dependency Notes
- Order rationale: Implement after US-3, US-5, US-1
- Plan sequence hint: Follow IMPLEMENTATION_ORDER.md from this command.
- Plan dependency hint: Use scaffold interfaces as non-breaking contracts.

## 6. Quality Constraints
- Domain profile: Not specified
- Stack constraint: Not specified
- Forbidden defaults: Not specified
- Non-negotiable: keep FR traceability comments in interfaces and TC markers in tests.

