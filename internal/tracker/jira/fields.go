package jira

import (
	"strconv"
	"strings"
)

// Field catalog. Custom field IDs differ on every site, so a
// field is found by its schema type (schema.custom) first, then by its name.
// No code in Maestro may use a fixed customfield_NNNNN ID.

const (
	SchemaSprint          = "com.pyxis.greenhopper.jira:gh-sprint"
	SchemaStoryPointEst   = "com.pyxis.greenhopper.jira:jsw-story-points"
	SchemaFloat           = "com.atlassian.jira.plugin.system.customfieldtypes:float"
	SchemaRank            = "com.pyxis.greenhopper.jira:gh-lexo-rank"
	SchemaMultiCheckboxes = "com.atlassian.jira.plugin.system.customfieldtypes:multicheckboxes"
	SchemaDatePicker      = "com.atlassian.jira.plugin.system.customfieldtypes:datepicker"
	SchemaTargetStart     = "com.atlassian.jpo:jpo-custom-field-baseline-start"
	SchemaTargetEnd       = "com.atlassian.jpo:jpo-custom-field-baseline-end"
	SchemaTeam            = "com.atlassian.jira.plugin.system.customfieldtypes:atlassian-team"
	SchemaParentLink      = "com.atlassian.jpo:jpo-custom-field-parent"
	SchemaEpicLink        = "com.pyxis.greenhopper.jira:gh-epic-link"
	SchemaEpicName        = "com.pyxis.greenhopper.jira:gh-epic-label"
	SchemaEpicStatus      = "com.pyxis.greenhopper.jira:gh-epic-status"
	SchemaEpicColor       = "com.pyxis.greenhopper.jira:gh-epic-color"
	SchemaDevelopment     = "com.atlassian.jira.plugins.jira-development-integration-plugin:devsummarycf"
	SchemaIssueColor      = "com.pyxis.greenhopper.jira:jsw-issue-color"
	schemaTextarea        = "com.atlassian.jira.plugin.system.customfieldtypes:textarea"
	schemaURL             = "com.atlassian.jira.plugin.system.customfieldtypes:url"
	schemaCascadingSelect = "com.atlassian.jira.plugin.system.customfieldtypes:cascadingselect"
	schemaLabels          = "com.atlassian.jira.plugin.system.customfieldtypes:labels"
)

// StandardFields are the IDs of Atlassian-provided custom fields on this site
// ("" if absent).
type StandardFields struct {
	Sprint         string `json:"sprint"`
	StoryPoints    string `json:"storyPoints"`    // by rule 1
	StoryPointsAlt string `json:"storyPointsAlt"` // the other story points field, if both exist
	Rank           string `json:"rank"`           // by rule 2
	Flagged        string `json:"flagged"`
	StartDate      string `json:"startDate"`
	TargetStart    string `json:"targetStart"`
	TargetEnd      string `json:"targetEnd"`
	Team           string `json:"team"`
	ParentLink     string `json:"parentLink"`
	EpicLink       string `json:"epicLink"`
	EpicName       string `json:"epicName"`
	EpicStatus     string `json:"epicStatus"`
	EpicColor      string `json:"epicColor"`
	Development    string `json:"development"`
	IssueColor     string `json:"issueColor"`
}

// IDs lists the non-empty standard field IDs.
func (s StandardFields) IDs() []string {
	var out []string
	for _, id := range []string{s.Sprint, s.StoryPoints, s.StoryPointsAlt, s.Rank, s.Flagged, s.StartDate, s.TargetStart,
		s.TargetEnd, s.Team, s.ParentLink, s.EpicLink, s.EpicName, s.EpicStatus, s.EpicColor, s.Development, s.IssueColor} {
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

// Standard resolves the catalog against this site's fields and boards.
func (d *Discovery) Standard() StandardFields {
	// bySchema finds a field by schema type; with a name, both must match
	// (generic types such as datepicker are used by many fields).
	bySchema := func(custom string, name ...string) string {
		for _, f := range d.Fields {
			if f.Schema.Custom == custom && (len(name) == 0 || strings.EqualFold(f.Name, name[0])) {
				return f.ID
			}
		}
		return ""
	}
	s := StandardFields{
		Sprint:      bySchema(SchemaSprint),
		Flagged:     bySchema(SchemaMultiCheckboxes, "Flagged"),
		StartDate:   bySchema(SchemaDatePicker, "Start date"),
		TargetStart: bySchema(SchemaTargetStart),
		TargetEnd:   bySchema(SchemaTargetEnd),
		Team:        bySchema(SchemaTeam),
		ParentLink:  bySchema(SchemaParentLink),
		EpicLink:    bySchema(SchemaEpicLink),
		EpicName:    bySchema(SchemaEpicName),
		EpicStatus:  bySchema(SchemaEpicStatus),
		EpicColor:   bySchema(SchemaEpicColor),
		Development: bySchema(SchemaDevelopment),
		IssueColor:  bySchema(SchemaIssueColor),
	}

	// Story points, rule 1: the board's estimation field, else, else.
	estimate := bySchema(SchemaStoryPointEst)
	classic := bySchema(SchemaFloat, "Story Points")
	board := ""
	for _, p := range d.Projects {
		for _, b := range p.Boards {
			if b.Config != nil && b.Config.Estimation != nil && strings.HasPrefix(b.Config.Estimation.Field.FieldID, "customfield_") {
				board = b.Config.Estimation.Field.FieldID
				break
			}
		}
		if board != "" {
			break
		}
	}
	switch {
	case board != "":
		s.StoryPoints = board
	case estimate != "":
		s.StoryPoints = estimate
	default:
		s.StoryPoints = classic
	}
	for _, id := range []string{estimate, classic} {
		if id != "" && id != s.StoryPoints {
			s.StoryPointsAlt = id
		}
	}

	// Rank, rule 2: the board's rank field, else by schema.
	for _, p := range d.Projects {
		for _, b := range p.Boards {
			if b.Config != nil && b.Config.Ranking.RankCustomFieldID != 0 {
				s.Rank = "customfield_" + strconv.Itoa(b.Config.Ranking.RankCustomFieldID)
				break
			}
		}
		if s.Rank != "" {
			break
		}
	}
	if s.Rank == "" {
		s.Rank = bySchema(SchemaRank)
	}
	return s
}

// FieldByIDOrName finds a field by ID ("customfield_10050") or name ("Severity").
func (d *Discovery) FieldByIDOrName(idOrName string) *Field {
	for i := range d.Fields {
		if d.Fields[i].ID == idOrName {
			return &d.Fields[i]
		}
	}
	for i := range d.Fields {
		if strings.EqualFold(d.Fields[i].Name, idOrName) {
			return &d.Fields[i]
		}
	}
	return nil
}
