# Policy templates

One Markdown file per policy, `<id>.md`, with YAML front matter. Loaded into the policy library;
an org instantiates a template into its own editable, versioned policy.

```markdown
---
id: access-control                 # kebab-case, equals the file name
title: Access Control Policy
category: Identity & Access        # Governance | Identity & Access | Operations | Data | People | Vendors | Engineering | Resilience | AI
summary: One sentence describing what the policy governs.
controls: [IAM-01, IAM-03, IAM-04] # control codes from ../controls.yaml this policy supports
review_cycle_days: 365
---
# Access Control Policy

Body in Markdown: headings (##), paragraphs, bullet lists, numbered lists, bold, tables.
```

Placeholders (replaced when an org instantiates the template; use them where the value matters,
because Trustline checks policy text against these settings):

| Placeholder | Meaning |
|---|---|
| `{{org_name}}` | Organization name |
| `{{effective_date}}` | Date the policy takes effect (YYYY-MM-DD) |
| `{{security_officer}}` | Name of the designated security officer |
| `{{access_review_days}}` | Access review cadence in days (setting) |
| `{{password_min_length}}` | Minimum password length (setting) |
| `{{policy_acceptance_threshold}}` | % of staff that must acknowledge policies (setting) |

Structure every policy with: Purpose, Scope, Roles and responsibilities, Policy statements (the
substance, specific and auditable: "must"/"at least", concrete numbers), Exceptions,
Enforcement, Review. 500–1100 words each. Write original text; do not copy published templates.
State numeric commitments in the form "every {{access_review_days}} days" or
"at least {{password_min_length}} characters" so consistency checks can find them.
