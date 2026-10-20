---
id: change-management
title: Change Management Policy
category: Engineering
summary: Defines how changes to code, infrastructure, and production systems are reviewed, tested, approved, and deployed.
controls: [CHG-01, CHG-02, CHG-03, CHG-04, CHG-05, CHG-06]
review_cycle_days: 365
---
# Change Management Policy

**Organization:** {{org_name}}
**Effective date:** {{effective_date}}
**Policy owner:** {{security_officer}}

## Purpose

Uncontrolled changes are a leading cause of outages and security weaknesses. This policy ensures that changes to {{org_name}} software and infrastructure are reviewed by someone other than the author, tested before release, traceable to a reason, and reversible if something goes wrong.

## Scope

This policy applies to all changes to:

- Application source code for products and internal tools.
- Infrastructure as code, cloud configuration, and network configuration.
- Database schemas and production data migrations.
- CI/CD pipelines and deployment tooling.
- Configuration of production SaaS systems that affect security or customer data (for example, identity provider settings).

## Roles and responsibilities

- **Security officer ({{security_officer}})** owns this policy and monitors compliance.
- **Engineering leadership** ensures teams follow this policy and that repository and pipeline settings enforce it.
- **Change author** describes, tests, and deploys the change, and monitors it after release.
- **Reviewer** independently evaluates the change for correctness, security, and operational impact.
- **Code owners** are the designated reviewers for sensitive areas of the codebase.

## Policy statements

### Change procedure

1. Every change must be linked to a ticket, issue, or pull request description that states what is changing and why.
2. Changes must be classified as:
   - **Standard:** routine, low-risk, follows the normal review and pipeline process.
   - **Significant:** affects security controls, authentication, customer data handling, or carries high outage risk. Significant changes must include a rollback plan and be approved by an engineering lead in addition to the normal review.
   - **Emergency:** needed immediately to restore service or fix an actively exploited vulnerability (see statement 14).
3. Changes must be deployed through the approved CI/CD pipeline. Manual changes directly in production must not be made except under the emergency procedure or a documented break-glass process.

### Peer review

4. Every change to production code or infrastructure must be reviewed and approved by at least one person other than the author before it is merged.
5. Changes to paths designated in code owner files (such as authentication, cryptography, infrastructure, and CI configuration) must be approved by a designated code owner.
6. Reviewers must check for correctness, security implications, test coverage, and operational risk. Rubber-stamp approvals without review are prohibited.

### Branch protection

7. Default and release branches of all production repositories must be protected so that:
   - Direct pushes are blocked.
   - Merges require at least one approving review.
   - Required status checks must pass before merge.
   - Force pushes and branch deletion are blocked.
   - Administrators are subject to the same rules, or bypasses are logged and reviewed.
8. When new commits are pushed to a pull request after approval, existing approvals must be dismissed automatically so the final change is reviewed.

### Automated checks

9. Every pull request to a production repository must run automated checks before merge, including at a minimum: build, unit tests, and dependency and secret scanning. Static analysis must run where tooling exists for the language.
10. Failing required checks must block merge. Checks must not be disabled to force a merge without approval from an engineering lead, and any such override must be recorded.

### Environments

11. Development, staging, and production environments must be separated. Production must run in separate cloud accounts or projects from non-production environments.
12. Production customer data must not be copied into non-production environments unless it has been anonymized or the security officer has approved the copy in writing.
13. Developers' everyday credentials must not have write access to production; production changes must flow through the pipeline.

### Emergency changes

14. Emergency changes may bypass pre-merge review only when delay would cause significant harm. The change must be approved verbally or in chat by an engineering lead, recorded, and retrospectively reviewed through a normal pull request within 2 business days.

### Post-deployment

15. Authors must monitor changes after deployment and roll back promptly if errors or alerts increase.
16. Deployment history must be retained for at least one year so that any production change can be traced to its approval.

## Exceptions

Repositories or systems that cannot meet these requirements, such as third-party platforms without review features, must be documented with compensating controls and approved by the security officer. Exceptions must be reviewed at least annually.

## Enforcement

Bypassing review or pipeline controls without authorization may result in loss of production access and disciplinary action. Compliance is verified by automated checks on repository settings and periodic sampling of merged changes for evidence of review.

## Review

This policy must be reviewed at least annually by the security officer and engineering leadership.
