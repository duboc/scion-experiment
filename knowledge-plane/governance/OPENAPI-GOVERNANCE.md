# Enterprise OpenAPI & API Contract Governance

* **Patterns Covered:** #3 (Automated API Contract Enforcement), #6 (Architectural Drift Detection)

## Mandatory Contract Rules
1. **Versioning:** All paths must be prefixed with `/api/v{major}/...` (e.g., `/api/v1/accounts`).
2. **Headers:**
   - `X-Correlation-ID` (UUIDv7/UUIDv4, required on all requests and echoed in responses).
   - `Idempotency-Key` (required on all state-mutating `POST`/`PATCH` operations).
3. **Zero PII in URLs/Query Strings (LGPD Rule):**
   - Never pass `cpf`, `cnpj`, `email`, or `account_number` in URL path segments or query parameters (they leak into proxy/access logs). Use opaque UUIDs or POST request bodies.
4. **Standard Error Envelope (RFC 7807):**
   ```json
   {
     "type": "https://errors.itau.example.com/insufficient-funds",
     "title": "Insufficient Funds",
     "status": 422,
     "detail": "Account balance does not cover requested transfer amount.",
     "correlation_id": "01926c84-..."
   }
   ```
