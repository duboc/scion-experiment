# Legacy Mainframe (COBOL/JCL) & Database Modernization Playbook

* **Patterns Covered:** #2 (Legacy Mainframe Modernization), #8 (Dependency & Framework Modernization), #12 (Database Schema Migration Automation), #20 (Latency Profiling & Refactoring)

## 1. COBOL Data Type Mapping Rules (#2)
| COBOL PIC Clause | Target Java / Python / Go Type | Notes |
| :--- | :--- | :--- |
| `PIC 9(n)V9(m) COMP-3` | `BigDecimal` / `Decimal` (scale `m`, `ROUND_HALF_EVEN`) | Never map packed decimal currency to `double`/`float`. |
| `PIC X(n)` | `String` (trimmed, UTF-8 normalized) | Preserve fixed-width padding only at mainframe boundary adapters. |
| `PIC 9(8)` (YYYYMMDD) | `LocalDate` / ISO-8601 `YYYY-MM-DD` | Validate leap years and 88-level sentinel values (`00000000`, `99999999`). |
| `88`-level condition names | Explicit Enum or Domain Predicate methods | Preserve exact boundary semantics in unit tests. |

## 2. Database Schema Migration Rules (#12)
- All SQL migrations must follow the **Expand-Contract** pattern for zero-downtime deployment:
  - Always provide both `V<N>__<description>.up.sql` and `V<N>__<description>.down.sql` (rollback script).
  - Never drop or rename an active column in the same release that deploys new application code.
