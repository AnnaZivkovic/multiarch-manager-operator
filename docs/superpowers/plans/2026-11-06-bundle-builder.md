# Bundle Builder Preservation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans. Steps use checkbox tracking.

**Goal:** Preserve Konflux-nudged builder values through SDK regeneration.
**Architecture:** Save Dockerfile before SDK runs. Patch generated Dockerfile with saved builder stage, retaining generated labels and metadata.
**Tech Stack:** Bash, GNU Make, operator-sdk.
**Spec:** docs/superpowers/specs/2026-11-06-bundle-builder-design.md

## Global Constraints
No new dependencies. Dockerfile owns builder values. Generated metadata and labels still refresh.

## Review Focus
- SDK removes builder before patching: test actual regeneration.
- Multiple runs: assert byte-for-byte idempotency.
- Missing builder: fail without modifying Dockerfile.
- Updated custom labels: assert requested VERSION.
- Builder COPY paths: assert --from=builder paths.

## Task 1: Preserve builder across generation
- [x] Replace shell regression fixtures with saved-builder/SDK regeneration coverage; observed both nudged images lost before fix.
- [x] Update Makefile recipe to snapshot Dockerfile using temporary file with cleanup and failure propagation.
- [x] Remove hardcoded builder from patch script; restore builder from optional saved Dockerfile (default current file), fail if absent.
- [x] Run shell regression, syntax checks, full bundle verification, and project tests when available. Review diff. No automatic commit.

## Verification record
- Shell regression: PASS, including idempotency, updated labels, missing builder and missing bundle rejection. Missing-bundle test also observed RED before adding validation.
- `bash -n` and `git diff --check`: PASS.
- `make bundle-verify`: PASS in temporary clean checkout, both original digest and simulated Konflux updates to builder base and operator digest.
- `make test`: exceeded time limit during `go vet`; terminated remaining processes. Full suite not completed.
- Self-review: source stage copied unchanged; SDK metadata/core labels retained; custom labels refreshed; no hardcoded builder fallback. No independent reviewer dispatched.

