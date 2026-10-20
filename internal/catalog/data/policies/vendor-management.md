---
id: vendor-management
title: Vendor Management Policy
category: Vendors
summary: Defines how third-party vendors and subprocessors are selected, assessed, contracted, monitored, and offboarded.
controls: [VEN-01, VEN-02, VEN-03, VEN-04]
review_cycle_days: 365
---
# Vendor Management Policy

**Organization:** {{org_name}}
**Effective date:** {{effective_date}}
**Policy owner:** {{security_officer}}

## Purpose

{{org_name}} relies on vendors for infrastructure, software, and services, and many of them handle company or customer data. This policy ensures that vendor risks are understood before a vendor is engaged, that appropriate contractual protections are in place, and that vendors continue to meet expectations throughout the relationship.

## Scope

This policy applies to all third parties that provide goods or services to {{org_name}} and that access, store, process, or transmit {{org_name}} information, connect to its systems, or support critical services. It includes SaaS providers, cloud infrastructure, contractors, consultants, and subprocessors of customer personal data. Free tools and trials are in scope if company data is entered into them.

## Roles and responsibilities

- **Security officer ({{security_officer}})** owns this policy, defines the assessment process, and approves high-risk vendors.
- **Vendor owner** is the employee who requests and manages the vendor relationship. The vendor owner must complete intake, provide information for assessment, and monitor the vendor.
- **Legal / procurement** ensures contracts include required security and privacy terms.
- **Privacy lead** maintains the subprocessor list and customer notifications.
- **Finance** must not pay new vendors that have not been approved through this process.

## Policy statements

### Inventory and tiering

1. {{org_name}} must maintain an inventory of all in-scope vendors recording the vendor name, owner, services provided, data types accessed, criticality, tier, contract dates, and last assessment date.
2. Each vendor must be assigned a risk tier at intake based on data sensitivity and business criticality:

| Tier | Criteria | Assessment depth | Reassessment |
|---|---|---|---|
| High | Accesses restricted or customer personal data, or supports a Tier 1 service | Security questionnaire plus independent audit report (e.g. SOC 2 Type II or ISO 27001 certificate) | Annually |
| Medium | Accesses internal or confidential data, or supports important operations | Security questionnaire or audit report | Every 2 years |
| Low | No access to non-public data and not business-critical | Basic review of security and privacy terms | At contract renewal |

3. The inventory must be reviewed at least annually to confirm accuracy and remove unused vendors.

### Due diligence

4. Vendors must be assessed according to their tier before any company data is shared with them or before they are granted system access.
5. Assessments must consider, as applicable: security certifications and audit reports, data location, encryption, access control, incident history, breach notification commitments, subprocessors, business continuity, and financial stability.
6. Findings that represent unacceptable risk must be remediated by the vendor, mitigated by {{org_name}}, or formally accepted by the security officer before onboarding. Accepted risks must be recorded in the risk register.

### Contracts

7. Contracts with medium and high tier vendors must include, as applicable: confidentiality obligations, security requirements, the right to receive audit reports, breach notification within a defined period (no more than 72 hours for vendors processing personal data), limits on subprocessing, data return and deletion at termination, and a data processing agreement where personal data is processed.
8. Contracts must be signed before data is shared or access is granted.

### Subprocessors

9. {{org_name}} must maintain a current list of subprocessors that process customer personal data, including their purpose and location.
10. The subprocessor list must be made available to customers and updated before a new subprocessor begins processing. Customers must be notified of changes in line with contractual commitments.

### Ongoing monitoring

11. Vendors must be reassessed at the frequency set by their tier and whenever there is a material change in the services, data shared, or the vendor's security posture.
12. Vendor owners must report vendor security incidents, service failures, or contract breaches to the security officer within one business day of learning of them.

### Offboarding

13. When a vendor relationship ends, the vendor owner must ensure that access is revoked, credentials are rotated, and the vendor returns or deletes {{org_name}} data. Written confirmation of deletion must be obtained for high tier vendors.

## Exceptions

Exceptions, such as urgent onboarding before an assessment is complete, must be approved by the security officer, limited in time to no more than 90 days, and recorded in the risk register.

## Enforcement

Engaging vendors outside this process, or sharing data with unapproved vendors, may result in disciplinary action. Compliance is verified by reconciling the vendor inventory against financial records and SSO application lists.

## Review

This policy must be reviewed at least annually by the security officer.
