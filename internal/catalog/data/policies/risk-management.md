---
id: risk-management
title: Risk Management Policy
category: Governance
summary: Defines how information security risks are identified, scored, treated, accepted, and monitored.
controls: [RSK-01, RSK-02, RSK-03, RSK-04]
review_cycle_days: 365
---
# Risk Management Policy

**Organization:** {{org_name}}
**Effective date:** {{effective_date}}
**Policy owner:** {{security_officer}}

## Purpose

This policy establishes a consistent way for {{org_name}} to identify, evaluate, and respond to risks to its information, systems, and operations, so that security investment is directed at the risks that matter most and leadership makes informed decisions about the risks it accepts.

## Scope

This policy covers risks to the confidentiality, integrity, and availability of {{org_name}} information and services, including technology, people, process, vendor, legal, fraud, and business continuity risks. It applies to all business units, systems, and vendors.

## Roles and responsibilities

- **Executive leadership** sets risk appetite, approves acceptance of high and critical risks, and reviews the risk register.
- **Security officer ({{security_officer}})** owns this policy, runs the risk assessment process, maintains the risk register, and reports risk status to leadership.
- **Risk owners** are accountable for specific risks, select and implement treatment, and report progress.
- **All personnel** must report newly identified risks to the security officer.

## Policy statements

### Risk assessment

1. A formal risk assessment must be performed at least annually and whenever there is a significant change, such as a new product, major architecture change, acquisition, new regulatory obligation, or serious incident.
2. The assessment must identify assets and processes in scope, threats and vulnerabilities, existing controls, and the resulting risks.
3. The annual assessment must include a fraud risk assessment that considers opportunities for fraud by employees, vendors, or outsiders (including financial fraud, misuse of privileged access, and manipulation of records) and the controls that address them.
4. The assessment must draw on the business impact analysis to understand the consequences of disruption to critical processes.

### Risk scoring

5. Each risk must be scored on a 5×5 matrix as **likelihood × impact**, giving a score from 1 to 25.

| Score | Likelihood | Impact |
|---|---|---|
| 1 | Rare – not expected within 5 years | Negligible – no customer effect, trivial cost |
| 2 | Unlikely – could occur within 5 years | Minor – limited internal disruption, small cost |
| 3 | Possible – could occur within 2 years | Moderate – limited customer impact or reportable event |
| 4 | Likely – expected within 1 year | Major – significant customer impact, regulatory action, or material cost |
| 5 | Almost certain – expected within months | Severe – threatens the business, large-scale breach |

6. Risk ratings are derived from the score:

| Score | Rating | Required response |
|---|---|---|
| 20–25 | Critical | Treatment plan within 10 business days; leadership informed immediately |
| 12–19 | High | Treatment plan within 30 days; acceptance only by executive leadership |
| 6–11 | Medium | Treatment plan within 90 days; acceptance by security officer |
| 1–5 | Low | Monitor; may be accepted by the risk owner |

7. Each risk must be scored both before (inherent) and after (residual) taking existing controls into account.

### Risk register

8. All identified risks must be recorded in the risk register with: a description, owner, inherent and residual scores, treatment decision, planned actions, due dates, and status.
9. Each risk must have a single named owner.
10. The risk register must be reviewed by the security officer at least quarterly and by executive leadership at least annually.

### Risk treatment

11. For every risk rated medium or above, the owner must choose one of the following treatments and document the rationale:
    - **Mitigate:** implement controls to reduce likelihood or impact.
    - **Transfer:** shift part of the risk to another party, for example through insurance or contract.
    - **Avoid:** stop the activity that creates the risk.
    - **Accept:** knowingly retain the risk within risk appetite.
12. Risk acceptance must be documented with the approver, rationale, and an expiry date no more than 12 months away, after which the risk must be reassessed.
13. Treatment actions must be tracked to completion. Overdue actions for high and critical risks must be escalated to leadership.

### Risk appetite

14. Leadership must define and document {{org_name}}'s risk appetite at least annually. Residual risks above appetite must not be accepted without explicit executive approval.

## Exceptions

There are no exceptions to the requirement to record and score risks. Deviations from treatment timelines must be approved by the security officer and recorded in the register.

## Enforcement

Failure to report known risks or to complete assigned treatment actions will be escalated to leadership and may result in disciplinary action. Compliance is verified through the risk register, assessment records, and leadership meeting minutes.

## Review

This policy must be reviewed at least annually by the security officer and approved by executive leadership.
