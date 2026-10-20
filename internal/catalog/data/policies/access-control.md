---
id: access-control
title: Access Control Policy
category: Identity & Access
summary: Defines how access to systems and data is requested, approved, authenticated, reviewed, and removed.
controls: [IAM-01, IAM-02, IAM-03, IAM-04, IAM-05, IAM-06, IAM-07, IAM-08, IAM-09, IAM-10, IAM-11, IAM-12]
review_cycle_days: 365
---
# Access Control Policy

**Organization:** {{org_name}}
**Effective date:** {{effective_date}}
**Policy owner:** {{security_officer}}

## Purpose

This policy ensures that only authorized people and services can access {{org_name}} systems and data, that they receive only the access they need, and that access is removed promptly when it is no longer required.

## Scope

This policy covers every account that can access {{org_name}} information: workforce accounts, administrator and privileged accounts, service accounts, API keys, cloud provider identities, and accounts held by contractors and vendors. It applies to production and non-production environments, internal tools, and third-party SaaS applications used for business.

## Roles and responsibilities

- **Security officer ({{security_officer}})** owns this policy, defines access standards, and oversees access reviews.
- **System owners** approve access to their systems, perform access reviews, and ensure their systems meet the authentication requirements below.
- **Managers** request access for their staff, confirm continuing need during reviews, and notify People Operations of role changes and departures.
- **People Operations** triggers onboarding and offboarding workflows and confirms termination dates.
- **IT / platform administrators** provision and deprovision accounts and maintain the identity provider.
- **All users** protect their credentials and must not share accounts.

## Policy statements

### Account management

1. Every user must have a unique account. Shared or generic accounts must not be used for interactive access; where a system makes a shared account unavoidable, it must have a named owner and be documented as an exception.
2. Access must be requested through the approved ticketing or access-request workflow and approved by the user's manager and the system owner before it is granted. Approval records must be retained for at least one year.
3. Access must follow the principle of least privilege: users and services must receive the minimum permissions needed for their role. Role-based groups must be used rather than individual permission grants where the system supports them.
4. Business applications must be connected to the central identity provider using single sign-on where the application supports it.

### Authentication

5. Multi-factor authentication (MFA) must be enforced for all workforce accounts on the identity provider, email, source code hosting, cloud provider consoles, and any system holding confidential or restricted data. Phishing-resistant MFA (security keys or platform authenticators) must be used for privileged accounts.
6. Passwords must be at least {{password_min_length}} characters long and must meet the requirements of the Password & Authentication Policy.
7. Interactive sessions on systems holding confidential data must time out after no more than 30 minutes of inactivity, and must require reauthentication after no more than 24 hours.

### Privileged access

8. Administrative and privileged access must be limited to personnel whose role requires it, must be approved by the security officer or delegate, and must be granted through separate privileged accounts or just-in-time elevation rather than everyday accounts.
9. Cloud IAM policies must grant only the actions and resources required. Wildcard administrator permissions must not be attached to users or workloads except for documented break-glass accounts.
10. Root and break-glass accounts must be protected with hardware MFA, must have credentials stored in a restricted vault, must not be used for routine work, and every use must generate an alert and be reviewed.

### Access reviews

11. User access to production systems, the identity provider, cloud accounts, and systems holding confidential data must be reviewed every {{access_review_days}} days by the system owner or manager.
12. Privileged access must be reviewed at least as often as user access, and reviewers must confirm each privileged grant individually.
13. Access found to be unnecessary during a review must be removed within 5 business days. Review evidence, including the reviewer, date, and decisions, must be retained for at least one year.

### Changes and removal

14. When a user changes role, access no longer needed for the new role must be removed within 5 business days of the change.
15. When personnel leave {{org_name}}, all access must be revoked within one business day of their last working day. For involuntary terminations, access must be revoked no later than the time the person is notified.
16. Accounts that have not been used for 90 days must be disabled automatically or through a documented monthly process.

### Credentials and keys

17. Service account credentials, API keys, and access keys must be stored in an approved secrets manager and must never be committed to source code.
18. Long-lived credentials must be rotated at least every 365 days and immediately upon suspected compromise or the departure of a person who had access to them. Short-lived, automatically issued credentials should be preferred.

## Exceptions

Exceptions must be requested in writing, approved by the security officer, recorded in the risk register with compensating controls, and must expire after no more than 12 months.

## Enforcement

Unauthorized access, credential sharing, or attempts to bypass access controls may lead to disciplinary action up to termination, and may be reported to authorities. Compliance is verified through access reviews, identity provider reports, and automated configuration checks.

## Review

The security officer must review this policy at least annually and after any significant change to the identity platform or an incident involving unauthorized access.
