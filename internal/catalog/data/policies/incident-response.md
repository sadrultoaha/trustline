---
id: incident-response
title: Incident Response Plan
category: Operations
summary: Defines how security incidents are detected, classified, contained, communicated, and learned from.
controls: [IR-01, IR-02, IR-03, IR-04, LOG-06]
review_cycle_days: 365
---
# Incident Response Plan

**Organization:** {{org_name}}
**Effective date:** {{effective_date}}
**Policy owner:** {{security_officer}}

## Purpose

This plan describes how {{org_name}} responds to security incidents so that harm is limited, systems are restored quickly, affected parties and regulators are informed on time, and lessons are fed back into the security program.

## Scope

This plan applies to any actual or suspected event that threatens the confidentiality, integrity, or availability of {{org_name}} information or systems, including those operated by vendors on its behalf. Examples include unauthorized access, malware, data leaks, lost devices containing company data, denial of service, and misuse of privileged access.

## Roles and responsibilities

- **Security officer ({{security_officer}})** owns this plan, ensures it is tested, and has authority to declare incidents and direct the response.
- **Incident commander** leads the response to a specific incident, coordinates responders, and makes time-critical decisions. The security officer acts as incident commander unless they delegate the role.
- **Technical responders** investigate, contain, and remediate under the incident commander's direction.
- **Communications lead** prepares internal updates and external communications to customers and the public.
- **Legal / privacy lead** assesses legal obligations, including breach notification, and engages external counsel when needed.
- **Executive sponsor** approves major business decisions, such as taking services offline or notifying customers.
- **All personnel** must report suspected incidents immediately.

## Policy statements

### Detection and reporting

1. Personnel must report suspected incidents to the security team through the designated channel as soon as they become aware, and in any case within one hour.
2. Security alerts from monitoring and detection tools must be triaged by an on-call responder. Critical and high alerts must be triaged within 1 hour and other alerts within 1 business day.
3. Every triaged alert must be recorded with its outcome: false positive, benign, or escalated to an incident.

### Classification

4. The incident commander must assign a severity to each incident when it is declared and revise it as facts emerge.

| Severity | Description | Examples | Response target |
|---|---|---|---|
| SEV-1 Critical | Confirmed compromise of restricted data or production, or a full outage of customer-facing services | Customer data exfiltration, ransomware, compromised admin account | Responders engaged within 30 minutes, 24/7 |
| SEV-2 High | Likely compromise or significant partial outage with limited data exposure | Malware on a workstation with data access, exposed secret in a public repository | Responders engaged within 2 hours |
| SEV-3 Medium | Contained event with low impact | Phishing email clicked without credential entry, lost encrypted laptop | Response started within 1 business day |
| SEV-4 Low | Policy violation or suspicious activity with no evident impact | Unapproved software, minor misconfiguration | Response started within 5 business days |

### Response phases

5. **Containment:** responders must act to stop ongoing harm, for example by isolating hosts, disabling accounts, or rotating credentials, while preserving evidence.
6. **Investigation:** responders must establish scope, root cause, affected systems and data, and timeline. Logs, images, and other evidence must be preserved and access to them restricted.
7. **Eradication and recovery:** the cause must be removed and systems restored from known-good states, with monitoring increased until the incident is closed.
8. **Post-incident review:** a written review must be completed within 10 business days of closing every SEV-1 and SEV-2 incident. It must document the timeline, root cause, impact, what went well, and corrective actions with owners and due dates.

### Tracking

9. Every incident must be recorded in the incident tracker with a unique identifier, severity, timeline, affected assets, actions taken, and status.
10. Corrective actions must be tracked to completion and reported to leadership as part of regular security reporting.

### Notification

11. The legal / privacy lead must assess every incident that may involve personal data to determine whether it is a personal data breach requiring notification.
12. Where required by law, the relevant supervisory authority must be notified of a personal data breach within 72 hours of {{org_name}} becoming aware of it. If full details are not yet available, an initial notification must be made within that window and supplemented later.
13. Affected individuals must be notified without undue delay when the breach is likely to result in a high risk to their rights and freedoms.
14. Customers must be notified within the time frames set out in their contracts. Where a contract is silent, affected customers must be notified without undue delay and within 72 hours of confirmation.
15. All notifications, and decisions not to notify, must be documented with the reasoning.

### Testing and readiness

16. This plan must be tested at least annually through a tabletop exercise or simulation involving the roles above. Findings must be recorded and the plan updated.
17. Contact details for responders, legal counsel, and key vendors must be kept current and reviewed at least every six months.

## Exceptions

Deviation from this plan during a live incident is permitted when the incident commander judges it necessary to limit harm. Deviations must be recorded in the incident record and discussed in the post-incident review.

## Enforcement

Failure to report a known incident, or interference with an investigation, may result in disciplinary action up to termination. Compliance is verified through incident records, alert triage metrics, and exercise reports.

## Review

This plan must be reviewed at least annually and after every SEV-1 incident or exercise that identifies material gaps.
