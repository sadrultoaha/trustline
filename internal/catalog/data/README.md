# Catalog data

Loaded on boot into global, read-only catalog tables. Edit data here, not code.

* `controls.yaml` — framework-neutral controls (code, name, domain, description, evidence mode, keywords, evidence TTL).
* `resources.yaml` — resource schemas per connector. Snapshots, test rules and the no-code builder use these field paths.
* `frameworks/<id>.yaml` — one framework each. Requirements list the control codes that satisfy them. **This mapping is the crosswalk**: a control mapped from several frameworks satisfies all of them at once.
* `tests/<connector>.yaml` — automated tests (declarative rules) per connector.

## Framework file

```yaml
id: soc2                      # stable id, lowercase
name: SOC 2
version: "2017 Trust Services Criteria (revised points of focus 2022)"
publisher: AICPA
description: One paragraph.
sections:                     # optional ordered list of section names for display
  - CC1 Control Environment
requirements:
  - code: CC6.1               # as the framework writes it
    section: CC6 Logical and Physical Access Controls
    title: Short title (<= 90 chars)
    description: The requirement text, paraphrased in one to three sentences (no copyrighted verbatim text).
    controls: [IAM-01, IAM-05] # one or more control codes from controls.yaml
```

Rules: every requirement maps to at least one existing control code; codes are unique within a framework.

## Test file

```yaml
- id: aws.s3.block_public_access      # <connector>.<area>.<check>, unique
  name: S3 buckets block public access # imperative, reads as the passing condition
  description: Why this matters, one or two sentences.
  connector: aws                       # key in resources.yaml
  resource: s3_bucket                  # resource type of that connector
  control: DAT-01                      # control code it provides evidence for
  severity: high                       # low | medium | high | critical
  quantifier: all                      # all | any | none | count  (count also takes min/max)
  where: {field: archived, op: eq, value: false}   # optional filter applied before quantifier
  assert:
    and:
      - {field: public_access_block.block_public_acls, op: eq, value: true}
      - {field: public_access_block.restrict_public_buckets, op: eq, value: true}
  remediation:
    summary: One sentence.
    steps:
      - Concrete console or CLI step.
    iac_hint: terraform:aws_s3_bucket_public_access_block   # optional: resource type the fix touches
```

Expression grammar: `{and: [expr...]}`, `{or: [expr...]}`, `{not: expr}`, or a condition
`{field, op, value}`. Fields are dotted paths from resources.yaml. A path containing `[*]`
yields several values; the condition holds when **every** value satisfies it (vacuously true
when there are none) unless the condition sets `match: any`.

Operators: `eq neq gt gte lt lte in nin contains not_contains exists not_exists matches len_gt len_lt`.
`in/nin` take a list; `contains/not_contains` work on strings (substring) and string lists
(membership); `matches` is a Go RE2 regex; `len_gt/len_lt` compare the length of a list or string.

Quantifiers: `all` — every (filtered) resource satisfies `assert`; `any` — at least one does;
`none` — no resource does; `count` — the number satisfying `assert` is within `min`/`max`.
With `all` and an empty filtered set the test passes.
