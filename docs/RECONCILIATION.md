# Compiled database format 2

The Forever compiler normalizes shared consumer fields before comparing sources.
It selects disagreements using explicit field priorities. Selection is a policy,
not proof that a source is correct. Source snapshots and compressed client tables
remain unchanged.

## Version and migration

The record envelope stays at `schemaVersion: 1`. Compilation manifests now carry
`compilationFormatVersion: 2`. An absent marker identifies the previous format;
readers reject other explicit versions and markers on source manifests. Check the
manifest before interpreting shared fields. The attributes contract is described
by `schemas/compilation-attributes.schema.json`; the manifest contract is in
`schemas/compilation-manifest.schema.json`.

Consumers should read normalized fields from `attributes`, check `fieldStates`
before using absent values, and inspect `provenance` for selections. Use
`attributes.sourceData[source]` for original representations. Each original
observation retains `revision`, `attributes`, `fieldStates`, and `provenance`
including paths. `normalizationIssues` explains excluded interpretations;
`excludedStates` records invalid, unsupported, or unavailable normalized fields.
These observations remain present when sources agree after normalization.

QuestieDB `preQuestGroup` members retain their signs from its signed storage type.
They are not converted to ordinary positive completed-quest IDs.

Source-native fields without a shared normalization rule retain their original
shape. They are not interchangeable merely because their names resemble another
field. Unrecognized disagreements have no default source priority.

The legacy ATT export and Wowhead zone bundles retain their existing contracts.
Reverting the reconciliation change restores the previous compilation and reader.

## Shared fields

| Fields | Consumer representation |
|---|---|
| Quest `startedBy`, `finishedBy` | Arrays of `{type, id}`, where type is `npc`, `object`, or `item` |
| Quest `objectives` | Named groups: `creature`, `object`, `item`, `reputation`, `killCredit`, `spell` |
| NPC/object `spawns` | Arrays of `{zoneId, x, y}`, using AreaTable IDs and 0–100 percentages |
| NPC/object `spawnPresence` | Area IDs where QuestieDB reports instance presence without a coordinate |
| Quest prerequisite, alternative, group, and exclusion ID lists | Sorted unique IDs, retaining the separate field names and their distinct semantics |
| `name` | Unicode NFC text, preserving case, punctuation, whitespace, and literal HTML-like text |
| Levels, IDs, flags, race/class masks | Exact JSON integers; quest level, zone/sort, and QuestieDB spell requirements may be signed |

Provider arrays, ID sets, and spawn arrays are sorted by their canonical JSON
representation and deduplicated exactly. Numeric representations such as `20`
and `20.0` compare equally without rounding. Objective lists and objective text
retain source order. Coordinates with any actual numeric difference remain
separate observations.

Creature, object, and item objectives contain `id` and any supplied `text` or
`iconFile`. Reputation entries contain faction `id` and `value`. Kill-credit
entries contain `ids`, with supplied `baseId`, `text`, and `iconFile`. Spell
entries contain spell `id`, with supplied `text` and `itemId`. Unknown counts are
omitted; the compiler does not infer them from text. Empty objectives become `{}`.

Provider coordinates and UI map metadata are preserved separately in the
original `sourceData` observation. They do not affect provider identity or spawn
point equality. Client POI coordinates remain native DB2 fields and never become
NPC spawn percentages.

A QuestieDB `{-1,-1}` tuple means instance presence. It becomes `spawnPresence`,
not a coordinate. If no real points exist, `spawns` is `unavailable`, not a known
empty array. `{0,0}` remains a real point. Partial sentinels and out-of-range
points are invalid. A malformed spawn field is excluded as a whole; valid points
are not silently salvaged from an invalid field.

## Source semantics and priority

| Shared field group | Priority among eligible observations |
|---|---|
| Quest mechanics, requirements, objectives, flags, relationships | QuestieDB, Wowhead, ATT |
| Quest/object names | Wowhead, QuestieDB, ATT |
| NPC/object spawns and instance presence | QuestieDB, Wowhead, ATT |
| Direct client definitions and supported NPC/item names | Client, QuestieDB, Wowhead, ATT |

`schemas/field-resolution.json` lists each governed entity/field explicitly.
There is no wildcard priority. Supported client aliases are `Name_lang` for NPCs,
spells, and factions; `Display_lang` for items; `AreaName_lang` for zones;
`MapName_lang` for maps; and `Title_lang` for achievements. Item `ItemLevel` and
`RequiredLevel` also map to the corresponding shared fields. Other DB2 fields
remain literal client definitions, including QuestV2 UI metadata.

ATT exports `lvl` as minimum `requiredLevel`, preserves profession gates and
literal inherited restrictions, and separates item starters, provided items, and
objective items. It retains `altQuests`, completion `lockCriteria`, and breadcrumb
flags as source-native evidence rather than guessing equivalent Questie fields.
Duplicate declarations retain source paths and observations; conflicting fields
and unresolved expressions remain explicit. The adapter does not invent finishers,
quest levels, quest area/sort, or objective text from catalog commentary. Format 2
still excludes those interpretations in older snapshots as `unsupported`.
Supported ATT starting providers, objectives, and prerequisites remain eligible.

Only eligible values participate in selection. Missing, unsupported, unavailable,
invalid, and conflicting source observations cannot win, even when they are the
only observation. An eligible explicit empty value remains distinct from missing
and can be selected by policy. A cleared scalar uses `null` with an `empty` field
state; it remains eligible and prevents fallback to another source’s stale value.
No objectives or spawn sets are unioned across
disagreeing sources.

For example, a QuestieDB starting-provider value `[[9]]` and an ATT value
`[{"type":"npc","id":9,"spawns":[[130,20,30]]}]` both normalize to:

```json
[{"id":9,"type":"npc"}]
```

Both sources appear in provenance and their original representations remain in
`sourceData`. If QuestieDB instead identifies NPC 10, its whole field is selected
by policy and both candidates remain in the conflict report.

## Reports and regeneration

`conflicts.json` retains normalized candidates, original `observations`, source
paths, source-field names, selected source, and selection rule. `conflicts.md`
separates selected and unresolved counts. Unsupported-versus-missing states alone
are not value disagreements; their original states remain in the audit. When no
eligible value exists, state precedence is invalid, conflicting, unsupported,
unavailable, then missing.

`reconciliation.json` counts dispositions by entity and entity/field. Equivalent
counts describe compiled fields with multiple agreeing sources, including an
agreeing group chosen against another candidate. Selected and unresolved counts
describe field conflicts. Invalid and unsupported counts describe excluded source
field observations. These units differ and must not be added into a coverage
percentage. Single-source fields are not counted as equivalent.

The compilation revision includes input manifests, the effective selection
policy, and normalization version 2. Policy or normalization version changes
therefore change the revision without requiring a source refresh. Semantic change detection ignores revision
metadata in original observations, while retaining changes to original values,
paths, states, normalization issues, and selection rules.

```sh
go run ./cmd/database compile --flavor forever
go run ./cmd/database validate --root data
python3 tools/coverage_guard.py --base origin/main
```

The compiler writes and reads back a staged dataset, validates references, and
then replaces the compilation directory. A failed compilation preserves the
previous published output. A leftover `.old` backup requires inspection before
retrying. Only the compilation is replaced; source datasets are not regenerated.

The frozen migration baseline is 104,840 records, including 7,410 quests, with
7,814 original field conflicts. Identity preservation is required; a lower
conflict count alone does not establish correctness or complete game coverage.
Encrypted client sections, partial adapters, unevaluated QuestieDB runtime stages,
and server-side availability rules remain coverage limits.
