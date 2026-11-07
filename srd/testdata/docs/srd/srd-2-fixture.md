---
unit: S2
title: 'Fixture: every field'
parent: null
children: [S2.1]
---

<!--
SRD S2 -- Fixture (GH-1).
-->

<!-- field: section_goal -->
## Section goal

<!-- lock -->
show every field in one document
<!-- /lock -->

<!-- field: goals -->
## Goals

<!-- lock -->
- **G1.1** The first goal.
- **G2.1** The second goal, folded over two lines.
<!-- /lock -->

<!-- field: objective -->
## Objective

First paragraph of the objective.

Second paragraph, with Section 2.1 kept.

<!-- field: claims_posture -->
## Claims posture

Demonstrated only.

<!-- field: prior_material -->
## Prior material

- `../prior/02-model.md`: the old model chapter

<!-- field: evidence -->
## Evidence

- exp01-baseline

<!-- field: citations -->
## Citations

<!--
Citations carry a role.
-->

- **lee-2026** (anchor): why it is cited
- **kim-2025** (evidence)

<!-- field: patterns -->
## Patterns

<!-- field: content -->
## Content

The first thing to say. [@lee-2026; @kim-2025]

A planned table goes here.
<!-- artifact: Table 2 (planned) -->

An unsupported claim, flagged (GH-12).

<!-- field: tables -->
## Tables

<!-- table | status: ready | wide: false -->
Table: the comparison

| Name | Value \| unit |
|---|---|
| alpha | 1 |
| beta | a \| b |

<!-- table | status: planned -->
Table: a header and no rows

| A | B |
|---|---|

<!-- table | wide: true -->
Table: no columns yet

<!-- field: figures -->
## Figures

- `d2/overview.d2` (status: ready, wide: true): the whole system
- `fig/levels.pdf` (status: planned): the levels

<!-- field: links -->
## Links

- requires:
- supports: S3, S4

<!-- field: acceptance -->
## Acceptance

<!-- lock -->
- The section states the goal.
<!-- /lock -->

<!-- field: gaps -->
## Gaps
