## Feature
Add AI-generated insights on top of the existing rule-based insights engine: an LLM reads already-collected telemetry (metrics, logs, service/docker/systemd state, active insights) and explains the **probable cause + suggested remediation** in plain language — read-only, no actions taken.

## Problem / Why
The insights engine surfaces rule-based signals, but the operator still has to interpret raw metrics and logs to answer "why is it slow / what's wrong / what do I do." There is no plain-language explanation of probable cause or next step. The goal is faster diagnosis without SSHing in to read logs manually.

## Target Users
The single admin/operator (existing user; US-007 single-admin model). No new user types. No multi-user / no exposing AI output to anyone but the admin.

## New Behavior
- The system must let the operator request an AI explanation for the current system state (or a specific insight), returning probable cause + suggested remediation in plain language.
- The system must base that explanation **only on data already collected** (metrics, logs, service/docker/systemd state, insights) and **cite which signals it used** — it must not invent data, and it must not take any action (read-only advisory).
- The system must call the LLM through a **provider-agnostic interface** (`internal/ai/`) so the model/provider is swappable without code changes.
- The system must store the AI provider **API key encrypted at rest** (AES-GCM, same mechanism as Telegram/SMTP secrets), configured from the **admin Settings panel** — never from an env var or baked into the binary.
- The system must let the operator set/change the **model name** and **endpoint URL** from the Settings panel (all AI config lives in one place).
- The system must **degrade gracefully when AI is not configured** (no key) — the panel behaves exactly as it does today.

## Success Criteria
- Given a configured provider and a real incident, when the operator requests an explanation, then they receive a plain-language probable-cause + remediation that references the actual metrics/logs used. Latency target ≤ ~10 s for a single explanation.
- Given no API key configured, when the operator uses the panel, then there are no AI-related errors and every existing behavior is unchanged.
- Given a key entered in Settings, then it is stored encrypted (never plaintext in the DB) and never written to logs.
- Start with a small/cheap swappable model (8–14B class), validated on 5–10 real cases before downsizing/upgrading; provider candidate is the operator's existing Ollama Cloud (GLM-4.6 as fallback tier).

## Touch Points
- **MODIFY** `internal/insights/` — add an AI-explanation path over existing insight/telemetry data.
- **MODIFY** Settings panel + handlers (`internal/server/` settings handlers, `web/` settings templates) — add AI config fields (key, model, endpoint).
- **MODIFY/REUSE** `internal/database/secrets.go` encrypted-secret pattern — add the AI provider key alongside Telegram/SMTP secrets.
- **ADD** new `internal/ai/` provider-agnostic package (the only purely-new module).
- Existing FRs touched: insights FR, FR-009 (UI), FR-007 (single-admin auth), the AES-GCM secrets-at-rest mechanism.

## Must Not Break (Regression Boundary)
- Existing rule-based insights output keeps working unchanged when AI is disabled.
- Existing Telegram/SMTP encrypted secrets and their Settings flow keep working unchanged.
- The Settings panel renders and saves all existing fields correctly when no AI key is set.
- NFR-001 (≤ 15 MB RAM on ARM) and NFR-002/003 (single Go binary, zero runtime deps): AI must be an **outbound network call to an external endpoint**, NOT an embedded/local model — so the binary stays standalone (same posture as FR-014 Tailscale: optional, gracefully degrading).

## Out of Scope
- (B) ChatOps / executing actions via the privileged helper.
- (C) AI-redacted alert notifications in `notify/`.
- Running/embedding a model locally on the Pi.
- Auto-remediation (the system taking actions on the operator's behalf) — advisory only.
- Multi-provider simultaneous use, fine-tuning, or training.
