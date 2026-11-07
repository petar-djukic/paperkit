# SRD markdown format

A section requirements document (SRD) states what one section of a paper is committed to saying: its goal, the subgoals it serves, the points it makes, the tables and figures it carries. A paper's `docs/VISION` states the goals of the whole paper. Both are written as markdown in the form below, which `srd.Load` reads into a plain map of fields. genoutline, the tutorial's specs check, and anything else built on `Load` work on that map.

The form follows the convention the declarative-agents specification corpus settled on: YAML front matter carries identity, and an HTML comment of the form `<!-- field: name -->` above each heading names the field the section holds. Headings are for people. The reader keys on the comment.

## Example

```markdown
---
unit: S5
title: 'Use case 2: change management'
parent: root
level: L4
---

<!--
SRD S5 -- Use case 2: change management (GH-100).
-->

<!-- field: section_goal -->
## Section goal

<!-- lock -->
demonstrate the guided agent at high autonomy
<!-- /lock -->

<!-- field: goals -->
## Goals

<!-- lock -->
- **G4.6** The guided agent generates code under staged approval.
<!-- /lock -->

<!-- field: objective -->
## Objective

Operates a core-network change-management use case.

<!-- field: prior_material -->
## Prior material

- `../specification-driven-development`: the spec-driven experiments

<!-- field: evidence -->
## Evidence

- exp05-change-plan

<!-- field: citations -->
## Citations

- **surana-structured-program-s-2025** (anchor): the structured-program baseline

<!-- field: patterns -->
## Patterns

- staged-approval

<!-- field: content -->
## Content

The agent drafts the change plan and the person approves it. [@surana-structured-program-s-2025]

A planned table compares the three approval stages.
<!-- artifact: Table 5 (planned) -->

<!-- field: tables -->
## Tables

<!-- table | status: ready | wide: false -->
Table: the approval stages

| Stage | Who approves |
|---|---|
| plan | the operator |
| apply | policy |

<!-- field: figures -->
## Figures

- `d2/change-flow.d2` (status: planned): the change flow

<!-- field: links -->
## Links

- requires: S3
- supports: S7

<!-- field: gaps -->
## Gaps

- The rollback path is not yet specified.
```

## Rules

**Front matter** is the document's `meta` mapping in YAML: `unit` (or `subsection`), `title`, `parent`, `children`, `level`, `letter`, `agent_kind`, `id` for an SRD; `name`, `started`, `venue`, `artifact`, `origin`, `supersedes`, `companion` for VISION. Values keep their YAML types (a null parent, a date).

**Comments.** A comment the YAML carried lands in an HTML comment under the heading of the field it sat in; the file's header comment lands right after the front matter. The reader skips every HTML comment except the field, table and artifact tags.

**Locks.** `section_goal`, `goals` and `acceptance` are contract values, and their bodies sit inside a block lock (`<!-- lock -->` and `<!-- /lock -->`, each alone on its line). Prose tooling treats a lock region as opaque, which is the protection the YAML backend gave these keys by excluding them.

**Text.** A value is its words: the reader collapses runs of ASCII whitespace, so line wrapping is free. Two documents are equal when their maps are equal after that collapse. Unicode spaces are content: the narrow no-break space (U+202F) the papers set before numbers survives.

### SRD fields

| Field | Form |
|---|---|
| `section_goal`, `objective`, `claims_posture` | paragraphs |
| `goals` | `- **G1.2** goal text`, one per goal |
| `prior_material` | ``- `path`: what it offers`` |
| `evidence`, `patterns`, `acceptance`, `gaps` | `- item`, one per entry |
| `citations` | `- **id** (role): note`; the role and the note are each optional |
| `content` | one paragraph per say. Its `carried_by` ids ride inline as a pandoc citation group, `[@a; @b]`, which the reader collects from anywhere in the paragraph, so a pass that moves the group loses nothing. An `artifact` is an `<!-- artifact: text -->` line directly under the paragraph. |
| `tables` | per table, a `<!-- table \| status: s \| wide: true -->` tag, a `Table: shows` line, and a pipe table with the columns as its header and the rows as its body. `\|` escapes a pipe in a cell. A table with no columns stops at the `Table:` line; one with columns and no rows key has a header and no body. An explicitly empty list is an attribute (`rows: []`). |
| `figures` | ``- `file` (status: s, wide: true): shows`` |
| `links` | `- supports: S2, S3` and `- requires: S1` |

### VISION fields

| Field | Form |
|---|---|
| `thesis`, `purpose`, `audience`, `abstract`, `direction`, `paper_model`, `software`, `claims_posture` | paragraphs |
| `goals` | as in an SRD, locked |
| `non_goals`, `success_criteria` | `- item` |
| `inputs` | `- name: text`, one per input |

A field the schema does not name is an error in both directions, and so is a record key outside its field's list. A new field is added to `format.go` and to this table, not tolerated silently.

### What a say may not be

A say that holds its own `[@...]` group would have it absorbed into `carried_by`, and a say opening with a list, heading, quote, table or comment marker would not read back as a paragraph. Write neither; a content item with no citations simply has no group.

## History

The papers' SRDs were first written in YAML. paperkit v0.20261107.5's `cmd/srd2md` converted every corpus, verifying each document before it wrote, and later releases read only the markdown form: a `.yaml` SRD or VISION left in a paper is an error, not silently skipped. Convert a straggler with that release.
