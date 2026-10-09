# ADR-001: Standardized Enterprise Banking Microservice Chassis

* **Status:** Accepted
* **Patterns Covered:** #1 (Spec-to-Code Synthesis), #16 (Standardized Microservice Chassis Bootstrapping), #18 (Automated Telemetry & Tracing Insertion)

## Context & Mandatory Requirements
Every backend service generated or modernized in this workspace must include:
1. **Observability & Tracing (#18):**
   - Emit structured JSON logs containing `timestamp`, `severity`, `service`, `correlation_id` (`X-Correlation-ID`), and `trace_id`.
   - Never log raw PII (CPF, CNPJ, card PAN, full account numbers, passwords, or tokens).
   - Expose `/healthz` (liveness) and `/readyz` (readiness) endpoints.
2. **Security Headers & Input Validation (#4, #16):**
   - Enforce strict schema validation on all request payloads.
   - Return `Content-Type: application/problem+json` (RFC 7807) on errors without leaking internal stack traces.
3. **Financial Precision (#1, #2):**
   - Monetary values MUST use exact decimal types (`BigDecimal` in Java, `Decimal` in Python, integer cents or decimal structs in Go) with `ROUND_HALF_EVEN` (banker's rounding) — NEVER IEEE 754 floating-point (`float`/`double`).
4. **Idempotency:**
   - All mutating financial endpoints (`POST`, `PATCH`) must require and honor an `Idempotency-Key` header.
