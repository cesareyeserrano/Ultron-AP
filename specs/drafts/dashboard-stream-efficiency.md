# AF-SPEC: <Feature Name>

STATUS: DRAFT

Tech Stack: <optional — e.g., "Node.js + React", "Python + FastAPI", "Go service">

## 1. Context
Optimize Ultron dashboard streaming for Raspberry Pi: move high-frequency dashboard updates to lightweight SSE JSON channels, keep UX fluid, add per-channel update cadence (metrics fast, charts medium, services slower), preserve security boundaries and low resource footprint.

---

(Complete all requirement sections with explicit user-provided requirements before approve.)

## 2. Actors
- <Actor 1>: <role description>
- <Actor 2>: <role description>

## 3. Functional Rules (traceable)
Use stable IDs. Sub-bullets add detail — they are included in the rule text.

- FR-1: <verifiable rule — must have a clear pass/fail criterion>
  - <optional: concrete detail or constraint>
- FR-2: <verifiable rule>

## 4. Edge Cases
- <edge case>

## 5. Failure Conditions
- <failure condition>

## 6. Non-Functional Requirements
<!-- For visual features, replace this section with "## 6. UI Structure" to enable UI traceability:
## 6. UI Structure
Screen: <ScreenName>
Flow: <ScreenName> → <TargetScreen>
### References
- UI-REF-1: <path/to/mockup.png> → AC-1, AC-2
-->
- <nfr>

## 7. Security Considerations
- <at least one security note/control>

## 8. Out of Scope
- <explicitly excluded>

## 9. Acceptance Criteria (Given/When/Then)
- AC-1: Given <context>, when <action>, then <expected>.
- AC-2: Given <context>, when <action>, then <expected>.

## 10. Requirement Source Statement
- Requirements must be provided explicitly by the user.
- Aitri does not invent requirements.
