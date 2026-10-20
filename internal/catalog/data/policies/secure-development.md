---
id: secure-development
title: Secure Software Development Policy
category: Engineering
summary: Sets the security requirements the organization applies throughout the software development lifecycle, from design and coding to review and release.
controls: [DEV-01, DEV-02, DEV-03, VUL-01, VUL-05, VUL-06, CHG-01]
review_cycle_days: 365
---
# Secure Software Development Policy

**Effective date:** {{effective_date}}
**Policy owner:** {{security_officer}}

## Purpose

This policy makes security part of how {{org_name}} builds software rather than a step added at the end. It defines the practices engineers must follow so that defects that could compromise confidentiality, integrity, or availability are prevented, detected early, and fixed before release.

## Scope

This policy applies to all software developed, maintained, or configured by {{org_name}}, including application code, infrastructure-as-code, scripts, and build pipelines, and to all employees and contractors who contribute to that code.

## Roles and responsibilities

- **Security Officer ({{security_officer}})** owns this policy, defines security tooling standards, and tracks remediation of findings.
- **Engineering leadership** ensures teams have the time, training, and tooling to meet this policy.
- **Code owners** are accountable for the security and quality of the repositories or components assigned to them and for reviewing changes to them.
- **Engineers** must follow secure coding practices, address findings in their code, and never bypass required controls.

## Policy statements

### Secure design and coding

1. Engineers must complete secure development training on hire and at least annually, covering common vulnerability classes such as those in the OWASP Top 10.
2. New features that handle authentication, authorization, payments, personal data, or cryptography must undergo a lightweight threat review before implementation, documented in the design or ticket.
3. Code must validate all external input, use parameterized queries, encode output appropriately, and enforce authorization on the server side.
4. Security-relevant functions such as authentication, session management, and cryptography must use well-maintained frameworks and libraries rather than custom implementations.
5. Errors and logs must not expose secrets, credentials, or unnecessary personal data.

### Code ownership

1. Every repository must have a documented owner (a team or individual), recorded in a CODEOWNERS file or equivalent.
2. Ownership must be updated when teams change, and orphaned repositories must be archived or reassigned within 30 days.

### Source code confidentiality

1. Source code is classified as Confidential and must be stored only in {{org_name}}'s approved version control system.
2. Repositories must be private by default. Publishing code publicly requires approval from {{security_officer}} and engineering leadership.
3. Access to repositories must follow least privilege, be granted through company identities with multi-factor authentication, and be removed when no longer needed.
4. Source code must not be copied to personal accounts, personal devices, or unapproved third-party tools.

### Peer review

1. All changes to production code must be reviewed and approved by at least one person other than the author before merge.
2. Changes to security-sensitive areas should be reviewed by a code owner of that area.
3. Reviewers must consider security impact, not only functionality. Authors must not approve their own changes.

### Automated security testing

1. **Dependency scanning:** All repositories must run software composition analysis to detect known vulnerabilities in third-party dependencies. Findings must be remediated within the timelines set by the Vulnerability Management Policy.
2. **Secret scanning:** Secret scanning must be enabled on all repositories, and push protection should be enabled where supported. A detected secret must be treated as compromised: it must be revoked and rotated within 24 hours, and then removed from the code.
3. **Static analysis:** Static application security testing must run on every pull request for supported languages. High and critical findings must be fixed or formally triaged as false positives before merge.
4. Required security checks must be configured as merge blockers where the platform supports it.

### Third-party components

1. Dependencies must come from trusted registries and should be pinned to specific versions using lock files.
2. Unmaintained components or those with no security fixes available must be replaced or approved as an exception.

### Secrets in development

1. Secrets must be stored in an approved secrets manager and injected at runtime.
2. Production data must not be used in development or test environments unless it has been anonymized or approval has been granted by {{security_officer}}.

## Exceptions

Exceptions to any requirement, including merging over an unresolved finding, must be documented with justification, risk acceptance, and a remediation date, and must be approved by {{security_officer}}. Emergency fixes may be merged with a single reviewer if review follows within two business days.

## Enforcement

Changes that bypass required review or security checks may be reverted. Engineers who repeatedly or deliberately violate this policy may be subject to disciplinary action, up to and including termination of employment or contract.

## Review

This policy is reviewed by {{security_officer}} and engineering leadership at least annually, and after significant security incidents or major changes to the development toolchain.
