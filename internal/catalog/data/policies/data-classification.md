---
id: data-classification
title: Data Classification & Handling Policy
category: Data
summary: Establishes the four data classification levels used at the organization and the handling rules required for each.
controls: [AST-02, DAT-01]
review_cycle_days: 365
---
# Data Classification & Handling Policy

**Effective date:** {{effective_date}}
**Policy owner:** {{security_officer}}

## Purpose

Not all information needs the same protection. This policy gives {{org_name}} a shared vocabulary for how sensitive information is, so that personnel apply the right safeguards consistently and resources are focused on the data that matters most.

## Scope

This policy applies to all information created, received, stored, or processed by {{org_name}}, in any form (electronic, paper, or verbal), including customer data entrusted to {{org_name}}. It applies to all employees, contractors, and third parties who handle that information.

## Roles and responsibilities

- **Security Officer ({{security_officer}})** owns this policy and resolves questions about classification.
- **Data owners** are accountable for classifying the data sets they are responsible for, approving access, and reviewing classifications at least annually.
- **System owners** ensure that systems apply the controls required for the highest classification of data they hold.
- **All personnel** must handle information according to its classification and, when unsure, treat it as Confidential until confirmed.

## Policy statements

### Classification levels

| Level | Description | Examples |
|---|---|---|
| **Public** | Approved for public release. Disclosure causes no harm. | Marketing website, published documentation, press releases |
| **Internal** | For use within {{org_name}}. Disclosure would cause minor harm. | Internal procedures, org charts, non-sensitive project notes |
| **Confidential** | Sensitive business or customer information. Disclosure could cause significant harm to {{org_name}}, customers, or individuals. | Customer data, personal data, source code, contracts, financial results |
| **Restricted** | The most sensitive information. Disclosure could cause severe harm or breach legal obligations. | Credentials and encryption keys, payment card data, special-category personal data, security vulnerability details |

1. All information must be classified. Information that has not been explicitly classified must be treated as Internal.
2. When data of different levels is combined, the combined set must be classified at the highest level present.
3. Data owners must record the classification of major data stores and systems in the asset inventory.
4. Classifications must be reviewed at least annually and when the nature of the data changes.

### Handling requirements

| Requirement | Public | Internal | Confidential | Restricted |
|---|---|---|---|---|
| Labeling | Not required | Optional | Should be labeled | Must be labeled |
| Access | Anyone | All personnel | Need-to-know, approved by data owner | Named individuals only, approved by data owner and {{security_officer}} |
| Encryption at rest | Not required | Required on company systems | Required | Required, with field-level encryption where feasible |
| Encryption in transit | Recommended | Required over public networks | Required (TLS 1.2+) | Required (TLS 1.2+) |
| External sharing | Permitted | With approval of manager | Only under NDA or contract and with approved tools | Only with written approval of data owner and {{security_officer}} |
| Storage locations | Any | Approved company systems | Approved systems inventoried for Confidential data | Approved systems specifically designated for Restricted data |
| Removable media and personal devices | Permitted | Not recommended | Prohibited unless encrypted and approved | Prohibited |
| Printing | Permitted | Permitted | Minimize; collect immediately | Prohibited unless approved |
| Disposal | Normal | Normal deletion | Secure deletion; cross-cut shredding | Secure deletion with verification; cross-cut shredding |

### Storage exposure

1. Storage services such as object storage buckets, file shares, and databases must not be publicly accessible unless they contain only Public data and public access has been approved by the data owner.
2. Public access blocking must be enabled at the account level for cloud storage where the provider supports it.
3. Sharing links to Internal, Confidential, or Restricted documents must be limited to named individuals or to {{org_name}}'s domain. "Anyone with the link" sharing is prohibited for these classes.
4. Automated checks must detect publicly accessible storage, and any unapproved exposure must be remediated within 24 hours and assessed as a potential security incident.

### Use of non-production environments and tools

1. Confidential and Restricted data must not be copied into development, test, or analytics environments unless it is anonymized or the environment meets the same controls as production.
2. Confidential and Restricted data must not be entered into external tools, including AI services, unless the tool is approved for that classification.

## Exceptions

Exceptions must be requested in writing, approved by the data owner and {{security_officer}}, and reviewed at least every 12 months.

## Enforcement

Mishandling of Confidential or Restricted information may result in disciplinary action up to and including termination, and may need to be reported under breach notification obligations.

## Review

{{security_officer}} reviews this policy at least annually and when new categories of data or legal obligations are introduced.
