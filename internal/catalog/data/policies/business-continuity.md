---
id: business-continuity
title: Business Continuity & Disaster Recovery Policy
category: Resilience
summary: Defines how critical services are kept running or restored after disruption, including recovery objectives, backups, and testing.
controls: [BCP-01, BCP-02, BCP-03, RSK-04, DAT-03, DAT-04]
review_cycle_days: 365
---
# Business Continuity & Disaster Recovery Policy

**Organization:** {{org_name}}
**Effective date:** {{effective_date}}
**Policy owner:** {{security_officer}}

## Purpose

This policy ensures that {{org_name}} can continue delivering its critical services, or restore them within agreed time frames, after a disruption such as a cloud region outage, data corruption, cyberattack, loss of a key vendor, or loss of access to offices or staff.

## Scope

This policy covers all customer-facing services, the infrastructure and data they depend on, internal systems essential to operations (such as identity, communication, and finance), and the people and vendors needed to run them.

## Roles and responsibilities

- **Security officer ({{security_officer}})** owns this policy and ensures the business impact analysis, plans, and tests are completed.
- **Executive leadership** approves recovery objectives and has authority to invoke the continuity plan.
- **Engineering / infrastructure leads** design systems to meet recovery objectives, maintain backups, and run recovery tests.
- **Service owners** maintain recovery runbooks for their services and participate in tests.
- **All personnel** must know how to reach the team and where to find continuity instructions during a disruption.

## Policy statements

### Business impact analysis

1. {{org_name}} must perform a business impact analysis (BIA) at least annually and whenever a significant new service or dependency is introduced.
2. The BIA must identify critical business processes and systems, their dependencies (including vendors), the impact of disruption over time, and the maximum tolerable downtime and data loss for each.
3. Recovery objectives must be derived from the BIA and approved by leadership.

### Recovery objectives

4. Systems must be assigned to a recovery tier and designed to meet the objectives for that tier.

| Tier | Examples | Recovery time objective (RTO) | Recovery point objective (RPO) |
|---|---|---|---|
| Tier 1 – Critical | Production application, customer database, authentication | 4 hours | 1 hour |
| Tier 2 – Important | Customer support tools, internal APIs, billing | 24 hours | 24 hours |
| Tier 3 – Standard | Internal wikis, analytics, development environments | 72 hours | 24 hours |
| Tier 4 – Deferrable | Archives, non-essential tools | 7 days | 7 days |

5. If a system cannot meet its tier objectives, the gap must be recorded as a risk with a treatment plan.

### Continuity and recovery plan

6. {{org_name}} must maintain a documented business continuity and disaster recovery plan that includes: invocation criteria and authority, communication and escalation trees, recovery procedures for each Tier 1 and Tier 2 system, alternative working arrangements, and contacts for key vendors.
7. The plan must be stored in a location that remains accessible if primary systems are unavailable, such as an offline copy or a separate provider.

### Resilient architecture

8. Production databases supporting Tier 1 systems must be deployed with high availability across at least two availability zones, with automatic failover.
9. Production infrastructure must be defined as code where practical so it can be redeployed in another region.
10. Single points of failure in Tier 1 systems must be identified and either removed or accepted as risks.

### Backups

11. All production data stores must be backed up automatically. Backup frequency must support the RPO of the system's tier.
12. Backups must be encrypted, stored in a separate account or location from the primary data, and protected from deletion or modification by the same credentials that manage production.
13. Backups must be retained for at least 30 days unless the Data Retention & Disposal Policy requires otherwise.
14. Backup jobs must be monitored, and failed backups must be investigated within one business day.

### Testing

15. Restoration from backup must be tested at least quarterly for Tier 1 data stores and at least annually for other tiers. Each test must confirm data integrity and record the time taken.
16. The disaster recovery plan must be tested at least annually through a failover exercise or a realistic simulation covering Tier 1 systems.
17. Test results, including whether RTO and RPO were met, issues found, and corrective actions, must be documented and reported to leadership. Corrective actions must be tracked to completion.

## Exceptions

Exceptions, such as a system that cannot be made highly available, must be approved by the security officer and leadership, recorded in the risk register with compensating controls, and reviewed at least annually.

## Enforcement

Service owners who do not maintain runbooks, backups, or tests as required will be escalated to leadership. Compliance is verified through backup monitoring, test records, and configuration checks.

## Review

This policy, the BIA, and the continuity plan must be reviewed at least annually and after any invocation of the plan or major change to infrastructure.
