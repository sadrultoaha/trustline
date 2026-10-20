---
id: password
title: Password & Authentication Policy
category: Identity & Access
summary: Sets requirements for passwords, multi-factor authentication, and session management across all systems.
controls: [IAM-06, IAM-01, IAM-11]
review_cycle_days: 365
---
# Password & Authentication Policy

**Organization:** {{org_name}}
**Effective date:** {{effective_date}}
**Policy owner:** {{security_officer}}

## Purpose

Credentials are the most common target for attackers. This policy defines how {{org_name}} personnel create, store, and use passwords and other authenticators, and how systems must enforce strong authentication, so that a single stolen or guessed secret is not enough to compromise an account.

## Scope

This policy applies to all personnel and to every system that authenticates users on behalf of {{org_name}}, including the identity provider, workstations, mobile devices, SaaS applications, cloud consoles, databases, network equipment, and internally built applications. It also applies to service accounts where passwords are used.

## Roles and responsibilities

- **Security officer ({{security_officer}})** owns this policy and approves authentication standards and exceptions.
- **IT / platform administrators** configure the identity provider and systems to technically enforce these requirements.
- **Engineering teams** ensure applications built by {{org_name}} meet the authentication requirements in this policy.
- **All personnel** create strong, unique passwords, protect their authenticators, and report suspected compromise immediately.

## Policy statements

### Password construction

1. Passwords must be at least {{password_min_length}} characters long. Systems must technically enforce this minimum wherever configuration allows.
2. Passphrases made of several unrelated words are encouraged. Systems must allow passwords of at least 64 characters and must accept spaces and all printable characters.
3. New passwords must be checked against a list of known breached, commonly used, and context-specific values (such as the company name or the user's name) and must be rejected if they match.
4. Composition rules that force specific character types, and password hints, must not be used, as they lead to predictable passwords.
5. Default or vendor-supplied passwords must be changed before a system is connected to any {{org_name}} network or put into use.

### Password use and storage

6. Each password must be unique to one account. Personnel must not reuse a {{org_name}} password on any other system or personal account.
7. Personnel must use the company-approved password manager to generate and store passwords. Passwords must not be written down, stored in plain-text files, spreadsheets, chat messages, tickets, or source code.
8. Passwords must never be shared, including with colleagues, managers, or IT staff. IT staff will never ask for a user's password.
9. Passwords must not be changed on an arbitrary schedule. They must be changed immediately if there is any indication of compromise, if they were disclosed, or if they appear in a breach notification.
10. Applications built by {{org_name}} must store passwords only as salted hashes using a modern, slow hashing algorithm (such as Argon2id, scrypt, or bcrypt). Passwords must never be logged or stored in reversible form.

### Multi-factor authentication

11. Multi-factor authentication must be enabled for all workforce accounts on the identity provider and for any system that is not behind single sign-on but holds internal, confidential, or restricted data.
12. Administrator and privileged accounts must use phishing-resistant factors, such as hardware security keys or platform passkeys. SMS and voice codes must not be used for privileged accounts.
13. Lost or stolen authenticators must be reported to IT within 24 hours so they can be revoked. Re-enrollment must require identity verification through a channel independent of the lost factor.

### Login protection and sessions

14. Systems must limit failed login attempts, for example by locking or throttling the account after no more than 10 consecutive failures.
15. Interactive sessions on systems holding confidential data must end or lock after no more than 30 minutes of inactivity and must require full reauthentication after no more than 24 hours.
16. Workstations must lock the screen after no more than 15 minutes of inactivity and require a password, PIN, or biometric to unlock.
17. Password reset processes must verify the user's identity before issuing a new credential, and reset links or temporary passwords must expire within 24 hours and be single-use.

### Service accounts

18. Service account passwords must be generated randomly with at least 32 characters, stored in the approved secrets manager, and rotated when anyone with access to them leaves or when compromise is suspected.

## Exceptions

Systems that cannot technically enforce these requirements must be documented with the specific limitation and compensating controls (for example, network restrictions or placement behind single sign-on). Exceptions must be approved by the security officer and reviewed at least annually.

## Enforcement

Failure to comply with this policy may result in suspension of access and disciplinary action up to termination. Compliance is monitored through identity provider configuration reports, password manager adoption metrics, and periodic audits.

## Review

This policy must be reviewed at least annually by the security officer, and whenever authentication technology or guidance changes significantly.
