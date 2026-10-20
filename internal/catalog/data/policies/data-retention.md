---
id: data-retention
title: Data Retention & Disposal Policy
category: Data
summary: Defines how long information is kept and how it is securely deleted or destroyed when no longer needed.
controls: [DAT-02, AST-06]
review_cycle_days: 365
---
# Data Retention & Disposal Policy

**Organization:** {{org_name}}
**Effective date:** {{effective_date}}
**Policy owner:** {{security_officer}}

## Purpose

Keeping data longer than necessary increases legal, privacy, and security risk; deleting it too early can breach legal obligations. This policy sets out how long {{org_name}} keeps different kinds of information and how that information must be securely disposed of at the end of its life.

## Scope

This policy applies to all information created, received, or held by {{org_name}} in any format, including customer data, personal data, business records, logs, backups, and paper records. It covers information stored in company systems, vendor systems, and on physical media such as laptops, drives, and printed documents.

## Roles and responsibilities

- **Security officer ({{security_officer}})** owns this policy and oversees secure disposal.
- **Data owners** ensure the retention schedule is applied to the data they own and approve deletion.
- **Legal / privacy lead** determines legal retention requirements, issues and lifts legal holds, and approves changes to the retention schedule.
- **Engineering** implements automated retention and deletion in production systems.
- **IT** manages disposal of devices and storage media.
- **All personnel** must not keep data beyond its retention period, including in personal folders, downloads, or email.

## Policy statements

### Retention

1. Information must be retained for no longer than the period in the retention schedule below, unless a longer period is required by law, contract, or a legal hold.
2. Where the schedule is silent, data owners must agree a retention period with the legal / privacy lead and add it to the schedule.
3. Retention periods must be enforced automatically in production systems where technically feasible. Where manual deletion is required, data owners must perform it at least quarterly.

### Retention schedule

| Data category | Retention period | Trigger |
|---|---|---|
| Customer content and account data | Duration of contract plus 30 days | Contract termination |
| Customer data in backups | 35 days | Backup creation |
| Personal data of prospects and marketing contacts | 2 years | Last interaction |
| Employee personnel records | 7 years | End of employment |
| Job applicant data (unsuccessful) | 12 months | Hiring decision |
| Financial and tax records | 7 years | End of financial year |
| Contracts and legal agreements | 7 years | Contract expiry |
| Security and audit logs | At least 1 year | Log creation |
| Application and debug logs | 90 days | Log creation |
| Incident records | 5 years | Incident closure |
| Policy versions and acknowledgements | 3 years after supersession or end of employment | Supersession / departure |
| Source code | Life of the product | — |

4. The legal / privacy lead must confirm the schedule against applicable law in each jurisdiction where {{org_name}} operates and adjust it where local law requires.

### Customer data on termination

5. When a customer contract ends, customer data must be deleted from production systems within 30 days, unless the customer requests export first or the contract requires otherwise. Copies in backups must expire through normal backup rotation.
6. Customers requesting written confirmation of deletion must receive it within 30 days of request.

### Legal holds

7. When litigation, investigation, or regulatory inquiry is reasonably anticipated, the legal / privacy lead must issue a legal hold. Data subject to a hold must not be deleted until the hold is lifted in writing, regardless of the schedule.

### Secure disposal

8. Electronic data must be deleted using methods that prevent practical recovery: cryptographic erasure, secure deletion features of the storage platform, or overwriting.
9. Devices and storage media (laptops, drives, phones, USB media) must be securely wiped using a recognized sanitization standard before reuse, resale, or return to a lessor. Media that cannot be wiped must be physically destroyed.
10. Physical destruction must be performed by IT or a certified disposal vendor. A certificate of destruction must be obtained for every batch handled by a vendor.
11. A disposal record must be kept for every device or media item, including asset identifier, method, date, and the person or vendor responsible. Records must be retained for at least 3 years.
12. Paper documents containing confidential or personal information must be cross-cut shredded or placed in locked confidential waste bins.
13. Vendors that hold {{org_name}} data must delete or return it at contract end in accordance with the Vendor Management Policy.

## Exceptions

Requests to keep data longer than the schedule allows must state the reason and duration and be approved by the data owner and the legal / privacy lead. Approved exceptions must be documented and reviewed at least annually.

## Enforcement

Retaining data beyond its permitted period, or disposing of data or devices insecurely, may result in disciplinary action. Compliance is verified through retention configuration checks, disposal records, and periodic audits.

## Review

This policy and the retention schedule must be reviewed at least annually by the security officer and the legal / privacy lead.
