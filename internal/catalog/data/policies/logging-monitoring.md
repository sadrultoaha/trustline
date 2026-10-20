---
id: logging-monitoring
title: Logging & Monitoring Policy
category: Operations
summary: Defines what the organization must log, how logs are protected and retained, and how security events are detected and triaged.
controls: [LOG-01, LOG-02, LOG-03, LOG-04, LOG-05, LOG-06]
review_cycle_days: 365
---
# Logging & Monitoring Policy

**Effective date:** {{effective_date}}
**Policy owner:** {{security_officer}}

## Purpose

Logs are how {{org_name}} reconstructs what happened, detects misuse, and proves to customers and auditors that systems are operating as intended. This policy sets the requirements for generating, protecting, retaining, and reviewing logs, and for responding to security alerts.

## Scope

This policy applies to all production systems, cloud accounts, identity providers, SaaS applications holding company data, network and security devices, and the personnel who operate them.

## Roles and responsibilities

- **Security Officer ({{security_officer}})** owns this policy, defines alerting rules, and oversees triage of security alerts.
- **Infrastructure / Operations** configures log sources, centralized logging, and retention settings.
- **On-call responders** acknowledge and triage alerts within the timelines below.
- **System owners** ensure their systems produce the logs required by this policy and that logs do not contain secrets.

## Policy statements

### Events to log

1. Audit logging must be enabled for all cloud accounts and regions in use, capturing management and control-plane activity.
2. At a minimum, the following events must be logged where the system supports it:
   - Successful and failed authentication attempts.
   - Creation, modification, and deletion of user accounts, roles, and permissions.
   - Use of privileged or administrative accounts.
   - Changes to security configuration, including logging configuration itself.
   - Access to Confidential and Restricted data in production systems.
   - Application errors and security-relevant exceptions.
3. Each log entry must include a timestamp, the source system, the identity performing the action, the action, and its outcome.
4. System clocks must be synchronized to a reliable time source so that logs can be correlated.
5. Logs must not contain passwords, secret keys, full payment card numbers, or other secrets. Personal data in logs must be minimized.

### Storage access logging

1. Access logging must be enabled on object storage buckets and file stores that hold Confidential or Restricted data.
2. Storage access logs must be delivered to a separate location that the logged resource cannot modify.

### Centralization and integrity

1. Security-relevant logs must be forwarded to a centralized logging platform or a dedicated logging account.
2. Log integrity must be protected through immutability or write-once storage, log file validation, or equivalent tamper-evident mechanisms.
3. Access to modify or delete logs must be limited to a small number of authorized administrators, and such actions must themselves be logged.
4. Disabling or reducing audit logging must generate an alert.

### Retention

1. Security and audit logs must be retained for at least 12 months, with at least the most recent 90 days immediately available for search.
2. Longer retention must apply where required by law, contract, or the legal and regulatory obligations register.
3. Logs must be deleted securely once the retention period has elapsed, unless subject to a legal hold.

### Threat detection

1. Managed threat detection services must be enabled for all production cloud accounts.
2. Alerts must be configured for at least: root or break-glass account use, repeated failed logins, privilege escalation, disabling of security controls, and unusual data access or exfiltration patterns.
3. Alerts must be routed to a monitored channel or on-call rotation, not to an individual mailbox only.

### Alert triage

1. Security alerts must be acknowledged and triaged within the following timeframes:

| Severity | Acknowledge within | Triage decision within |
|---|---|---|
| Critical | 1 hour | 4 hours |
| High | 4 hours | 1 business day |
| Medium | 1 business day | 5 business days |
| Low | 5 business days | 30 days |

2. Each triaged alert must be recorded with its outcome: false positive, benign, or escalated.
3. Alerts confirmed as security incidents must be escalated under the Incident Response Plan.
4. Alert rules must be tuned at least quarterly to reduce noise and close detection gaps.

### Review

1. {{security_officer}} must review a summary of alert volumes, triage times, and logging coverage at least quarterly.

## Exceptions

Systems that cannot meet a logging requirement must be documented with compensating controls and approved by {{security_officer}}. Exceptions must be reviewed at least every 12 months.

## Enforcement

Disabling, tampering with, or deleting logs without authorization is a serious violation and may result in disciplinary action up to and including termination, as well as legal action where appropriate.

## Review

This policy is reviewed by {{security_officer}} at least annually and after any incident where logging gaps were identified.
