# US0020: Stabilization and Bug Fixes

> **Status:** Done
> **Epic:** [EP0004: Service Controls](../epics/EP0004-service-controls.md)
> **Created:** 2026-02-23

## User Story
**As an** Admin
**I want** reliable hardware and settings controls
**So that** the system behaves predictably and configuration changes are saved

## Acceptance Criteria
- [x] Fix Hardware Controls: Ensure RGB and Fan can be turned OFF, not just ON.
- [x] Fix Performance Settings: Ensure performance configuration changes are saved and persisted.
- [x] Verify that the UI reflects the current state correctly after a page reload.

## Investigation Notes
- User reports hardware "things activate but don't deactivate".
- User reports "performance settings cannot be configured".

## Fix Details
- Updated `pironman/controls.go` to use "on"/"off" instead of "true"/"false" for CLI compatibility.
- Updated `internal/database/sqlite.go` schema to allow `performance` as a valid notification channel, and added auto-migration for existing databases.
