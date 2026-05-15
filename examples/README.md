# `redline` example prompt configs

This directory has working `prompts.yaml` files for three representative verticals. Each file is self-contained: clone the repo (or just copy a file out), drop your real canonical messaging into the `canonical_messaging` block, and run:

```bash
redline scan --site https://example.com --prompts examples/saas-product.yaml
```

| Example | Vertical | Prompts | Best for |
|---|---|---|---|
| [`saas-product.yaml`](./saas-product.yaml) | B2B SaaS product page audit | 14 | Marketing site for a multi-product SaaS — pricing pages, product landings, comparison pages |
| [`dev-tool.yaml`](./dev-tool.yaml) | Open-source / developer tool | 12 | Docs sites, OSS project sites, dev-tool marketing |
| [`security-vendor.yaml`](./security-vendor.yaml) | Security tooling vendor | 16 | Security/compliance product sites with technical buyers + analyst coverage |

## Anatomy of a `prompts.yaml`

Every config has two top-level blocks:

```yaml
prompts:                 # questions you want LLMs to answer correctly about your brand
  - id: <snake_case_id>
    text: "<the prompt>"
    weight: 1.0          # 1.5 = critical, 1.0 = standard, 0.7 = tactical
    tags: [optional, free-form]

canonical_messaging:     # your messaging house — what your brand IS and IS NOT
  - title: "What we are"
    body: |
      <2-4 sentence positioning statement>
  - title: "What we are NOT"
    body: |
      <list of categories you do NOT play in>
  - title: "Product taxonomy (YYYY)"
    body: |
      - Product A — <one-liner>
      - Product B — <one-liner>
```

## Tips for writing your own prompts

- **Start with the 5-10 questions your buyers actually ask.** Watch your sales discovery calls or read your top inbound queries.
- **Weight strategically.** Set `weight: 1.5` only on the prompts where losing GEO ground hurts. Most prompts are 1.0.
- **Be explicit about retired names.** If your company used to call a product "Foo" and now calls it "Bar," add that to the "What we are NOT" block. The judge will flag pages still using the old name as `Stale` or `Contradictory`.
- **Iterate.** Run a scan, read the report, look for patterns in `Unclear` pages — those usually mean the canonical messaging is missing a topic.

## What about the eight labels?

Every page in the report gets one **primary label**:

| Label | What it means | Default action |
|---|---|---|
| `Aligned` | Supports the canonical answer for ≥1 prompt | KEEP |
| `Stale` | Was correct once, factually outdated now | UPDATE |
| `OffBrand` | Tone/voice doesn't match canonical messaging | UPDATE |
| `Contradictory` | Actively contradicts canonical messaging | REWRITE |
| `Redundant` | Duplicates a stronger newer page | DELETE |
| `Thin` | < ~200 words of substantive content | DELETE |
| `Zombie` | Thin + no traffic + no recent updates | DELETE |
| `Unclear` | Judge couldn't classify confidently | MANUAL_REVIEW |

`Contradictory` is the highest-priority label — these pages actively mislead LLM answer engines about your brand and should be fixed first.
