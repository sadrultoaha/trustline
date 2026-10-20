---
id: asset-management
title: Asset Management Policy
category: Operations
summary: Governs how the organization inventories, classifies, secures, and disposes of hardware, software, cloud, and data assets.
controls: [AST-01, AST-02, AST-03, AST-04, AST-05, AST-06]
review_cycle_days: 365
---
# Asset Management Policy

**Effective date:** {{effective_date}}
**Policy owner:** {{security_officer}}

## Purpose

{{org_name}} cannot protect what it does not know it has. This policy requires that information assets are identified, owned, classified, protected according to their value, and disposed of securely at the end of their life.

## Scope

This policy covers all assets that store, process, or provide access to {{org_name}} information, including:

- End-user devices: laptops, desktops, tablets, and mobile phones that are company-owned or used for company work.
- Infrastructure: cloud accounts, virtual machines, containers, databases, storage, and network devices.
- Software: SaaS applications, licensed software, and internally developed systems.
- Data stores and removable media.

It applies to all employees, contractors, and third parties who use or administer these assets.

## Roles and responsibilities

- **Security Officer ({{security_officer}})** owns this policy, defines endpoint security baselines, and reviews compliance reports.
- **IT / Operations** maintains the asset inventory, enrolls devices in management tooling, and performs secure disposal.
- **Asset owners** are accountable for the classification, access, and lifecycle of the assets assigned to them.
- **All personnel** must use assets in accordance with this policy, keep devices updated, and report lost or stolen devices immediately.

## Policy statements

### Asset inventory

1. An asset inventory must be maintained covering endpoints, servers and cloud resources, SaaS applications, and significant data stores.
2. Each inventory entry must record at least: asset identifier, type, owner, location or environment, data classification, and status (active, in repair, retired).
3. Cloud resources should be discovered automatically through integrations with cloud providers to keep the inventory current.
4. The inventory must be reviewed for accuracy at least quarterly, and discrepancies must be resolved within 30 days.
5. New assets must be added to the inventory before they are used for company data. Retired assets must be marked as such when they are decommissioned.

### Ownership and classification

1. Every asset must have a named owner.
2. Each asset must be assigned a data classification consistent with the Data Classification & Handling Policy, based on the most sensitive data it stores or processes.
3. Protection measures must be proportionate to the classification of the asset.

### Endpoint security baseline

All company-managed endpoints used to access company systems must meet the following baseline before they are given access, and must continue to meet it while in use:

| Requirement | Standard |
|---|---|
| Disk encryption | Full-disk encryption (e.g. FileVault, BitLocker, LUKS) must be enabled |
| Screen lock | Automatic screen lock must activate after no more than 15 minutes of inactivity and require a password, PIN, or biometric to unlock |
| Anti-malware | Anti-malware or endpoint detection and response software must be installed, running, and receiving updates |
| Operating system | A vendor-supported OS version with security updates applied within the timelines of the Vulnerability Management Policy |
| Management | The device must be enrolled in device management so compliance can be verified |
| Firewall | The host firewall must be enabled |

1. Compliance with the baseline must be monitored centrally, and non-compliant devices must be remediated within 14 days or have their access to company systems revoked.
2. Users must not disable or tamper with encryption, screen lock, anti-malware, or management agents.
3. Personal devices may access company email and collaboration tools only where permitted by the Acceptable Use Policy and must meet the screen lock and OS update requirements.

### Lost or stolen assets

1. Lost or stolen devices must be reported to IT and {{security_officer}} within 24 hours of discovery.
2. IT must revoke sessions and credentials associated with the device and, where possible, remotely lock or wipe it.

### Return of assets

1. Personnel must return all company assets on or before their last working day.
2. Returned devices must be reviewed, wiped, and reissued or retired.

### Secure disposal and media sanitization

1. Assets that have stored company data must be sanitized before reuse, resale, donation, or disposal, using methods appropriate to the media (cryptographic erase, secure overwrite, or physical destruction).
2. Media that cannot be reliably sanitized must be physically destroyed.
3. Third-party disposal vendors must provide a certificate of destruction, which must be retained for at least three years.
4. Disposal must be recorded in the asset inventory, including date, method, and person responsible.

## Exceptions

Exceptions, such as a lab device that cannot run anti-malware, must be documented with a business justification and compensating controls, approved by {{security_officer}}, and reviewed at least every 12 months.

## Enforcement

Non-compliant devices may be blocked from company systems without notice. Personnel who violate this policy may be subject to disciplinary action, up to and including termination of employment or contract.

## Review

{{security_officer}} reviews this policy at least annually and after material changes to the technology environment. Updated versions must be communicated to all personnel.
