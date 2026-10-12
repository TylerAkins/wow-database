package database

import (
	"encoding/json"
	"sort"

	"github.com/TylerAkins/wow-database/internal/record"
)

func originalCandidates(rows []record.Record, field string, audit map[string]sourceAudit) []Candidate {
	out := []Candidate{}
	for _, row := range rows {
		sourceField := field
		if row.Source == "foreverclient" {
			if alias := clientAliases(row.EntityType)[field]; alias != "" {
				sourceField = alias
			}
		}
		if field == "spawnPresence" {
			sourceField = "spawns"
		}
		value, exists := row.Attributes[sourceField]
		state := row.FieldStates[sourceField]
		if !exists && state == "" {
			continue
		}
		out = append(out, Candidate{Source: row.Source, Revision: row.SourceRevision, Value: value, State: state, SourceField: sourceField, Path: row.Provenance[sourceField].Path, Reason: audit[row.Source].Issues[field]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Source < out[j].Source })
	return out
}

type Dispositions struct {
	Equivalent  int `json:"equivalent"`
	Selected    int `json:"selected"`
	Unresolved  int `json:"unresolved"`
	Invalid     int `json:"invalid"`
	Unsupported int `json:"unsupported"`
}
type ReconciliationReport struct {
	CompilationFormatVersion int                      `json:"compilationFormatVersion"`
	Totals                   Dispositions             `json:"totals"`
	ByEntity                 map[string]*Dispositions `json:"byEntity"`
	ByField                  map[string]*Dispositions `json:"byField"`
}

func reconciliationReport(rows []record.Record, conflicts []Conflict) ReconciliationReport {
	out := ReconciliationReport{CompilationFormatVersion: CompilationFormatVersion, ByEntity: map[string]*Dispositions{}, ByField: map[string]*Dispositions{}}
	add := func(entity, field, kind string) {
		if out.ByEntity[entity] == nil {
			out.ByEntity[entity] = &Dispositions{}
		}
		key := entity + "." + field
		if out.ByField[key] == nil {
			out.ByField[key] = &Dispositions{}
		}
		for _, count := range []*Dispositions{&out.Totals, out.ByEntity[entity], out.ByField[key]} {
			switch kind {
			case "equivalent":
				count.Equivalent++
			case "selected":
				count.Selected++
			case "unresolved":
				count.Unresolved++
			case "invalid":
				count.Invalid++
			case "unsupported":
				count.Unsupported++
			}
		}
	}
	for _, row := range rows {
		for field, prov := range row.Provenance {
			if len(prov.Sources) > 1 && row.Attributes[field] != nil {
				add(row.EntityType, field, "equivalent")
			}
		}
		var audits map[string]sourceAudit
		_ = json.Unmarshal(row.Attributes["sourceData"], &audits)
		for _, audit := range audits {
			for field, state := range audit.ExcludedStates {
				if state == "invalid" || state == "unsupported" {
					add(row.EntityType, field, state)
				}
			}
		}
	}
	for _, c := range conflicts {
		kind := "unresolved"
		if c.Selected != "" {
			kind = "selected"
		}
		add(c.EntityType, c.Field, kind)
	}
	return out
}
