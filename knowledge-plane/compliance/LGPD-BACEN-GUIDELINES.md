# LGPD Data Privacy, OWASP Security & BACEN Compliance Guidelines

* **Patterns Covered:** #4 (Agentic Security & Compliance Review), #11 (Regulatory & Compliance Lineage Audit), #14 (LGPD-Compliant Synthetic Data Generation), #19 (Zero-Day Vulnerability Auto-Patching)

## 1. LGPD (Lei Geral de Proteção de Dados) Rules
- **PII Classification:** `cpf`, `cnpj`, `full_name`, `birth_date`, `email`, `phone`, `pix_key`, `card_pan`, `biometric_hash`.
- **Log Masking:** Any PII displayed in audit logs or UI summaries must be masked (e.g., CPF `***.456.789-**`, email `j***@domain.com`).
- **Synthetic Test Data (#14):** Test fixtures and integration tests MUST use synthetically generated, mathematically valid check-digit CPFs/CNPJs with `.example.com` emails and clearly marked synthetic flags (`"synthetic": true`). Never copy production dumps.

## 2. BACEN Resolution 4.893 & Traceability (#11)
- Every code change must produce or update `/workspace/PROVENANCE.json` and `/workspace/COMPLIANCE_LINEAGE.md` mapping:
  1. Business Epic / Regulatory Mandate ID
  2. Design Specification SHA-256 (`/workspace/DESIGN.md`)
  3. Implementing & Reviewing Agent IDs + Model Harnesses
  4. Automated Test & Security Scan Evidence
  5. Explicit Human-in-the-Loop Sign-Off Status
