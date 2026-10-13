// Package setup turns Jira discovery into configuration proposals.
// The user confirms them in the first-start
// wizard; nothing here talks to Jira.
package setup

import (
	"sort"
	"strings"

	"maestro/internal/config"
	"maestro/internal/tracker/jira"
)

// Confidence tells the wizard which proposals need a human look.
type Confidence string

const (
	Certain  Confidence = "certain"  // from Jira's own data (category, hierarchy level)
	Guess    Confidence = "guess"    // from the name; the user must confirm (10.2 rule 1)
	Fallback Confidence = "fallback" // nothing matched; the default was used
)

type RoleProposal struct {
	IssueType  jira.IssueType
	Role       config.Role
	Confidence Confidence
}

// ProposeRole is.3.1.
func ProposeRole(t jira.IssueType) RoleProposal {
	p := RoleProposal{IssueType: t, Confidence: Certain}
	name := strings.ToLower(t.Name)
	switch {
	case t.Subtask || t.HierarchyLevel < 0:
		p.Role = config.RoleSubtask
	case t.HierarchyLevel == 1:
		p.Role = config.RoleEpic
	case t.HierarchyLevel >= 2:
		p.Role = config.RoleInitiative
	case name == "bug" || name == "defect":
		p.Role = config.RoleBug
	case name == "task" || name == "spike" || name == "chore" || name == "improvement":
		p.Role = config.RoleTask
	case name == "story" || name == "user story":
		p.Role = config.RoleStory
	default:
		p.Role, p.Confidence = config.RoleStory, Fallback
	}
	return p
}

type StateProposal struct {
	Status     jira.StatusDef
	State      config.State
	Confidence Confidence
}

var (
	reviewWords  = []string{"review", "qa", "test", "verify", "verif", "pr ", "pull request", "approval"}
	blockedWords = []string{"block", "hold", "wait", "imped", "stuck"}
)

// ProposeState is.3.2.
func ProposeState(s jira.StatusDef) StateProposal {
	p := StateProposal{Status: s, Confidence: Certain}
	name := " " + strings.ToLower(s.Name) + " "
	switch strings.ToUpper(s.StatusCategory) {
	case "TODO", "NEW":
		p.State = config.StateTodo
	case "DONE":
		p.State = config.StateDone
	default: // IN_PROGRESS / indeterminate: Jira cannot tell review or blocked apart
		p.Confidence = Guess
		switch {
		case containsAny(name, blockedWords):
			p.State = config.StateBlocked
		case containsAny(name, reviewWords):
			p.State = config.StateInReview
		default:
			p.State = config.StateInProgress
		}
	}
	return p
}

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// Proposal is what the wizard shows after discovery.
type Proposal struct {
	Roles  []RoleProposal
	States []StateProposal
	Fields config.Fields
	Order  []string // WFL-03, from the first board's columns
	Boards map[string]int
	// NeedsConfirmation lists statuses whose state is a guess (rule).
	NeedsConfirmation []string
}

// Propose builds the configuration proposal from a discovery result.
func Propose(d *jira.Discovery) *Proposal {
	p := &Proposal{Boards: map[string]int{}}
	seenType, seenStatus := map[string]bool{}, map[string]bool{}
	statusName := map[string]string{}

	for _, pd := range d.Projects {
		for _, t := range pd.IssueTypes {
			if !seenType[t.ID] {
				seenType[t.ID] = true
				p.Roles = append(p.Roles, ProposeRole(t))
			}
		}
		for _, s := range pd.Statuses {
			statusName[s.ID] = s.Name
			if seenStatus[s.Name] {
				continue
			}
			seenStatus[s.Name] = true
			sp := ProposeState(s)
			p.States = append(p.States, sp)
			if sp.Confidence == Guess {
				p.NeedsConfirmation = append(p.NeedsConfirmation, s.Name)
			}
		}
		if b := pickBoard(pd); b != nil {
			p.Boards[pd.Project.Key] = b.Board.ID
			if len(p.Order) == 0 && b.Config != nil {
				p.Order = columnOrder(b.Config, statusName)
			}
		}
	}
	sort.Slice(p.Roles, func(i, j int) bool { return p.Roles[i].IssueType.Name < p.Roles[j].IssueType.Name })
	p.Fields = config.Fields{
		StoryPoints: d.StoryPointsField(),
		Sprint:      d.SprintField(),
	}
	return p
}

// pickBoard is rule: the first Scrum board of the project, else the first board.
func pickBoard(pd jira.ProjectDiscovery) *jira.BoardDiscovery {
	for i := range pd.Boards {
		if pd.Boards[i].Board.Type == "scrum" {
			return &pd.Boards[i]
		}
	}
	if len(pd.Boards) > 0 {
		return &pd.Boards[0]
	}
	return nil
}

func columnOrder(bc *jira.BoardConfig, statusName map[string]string) []string {
	var out []string
	for _, col := range bc.ColumnConfig.Columns {
		for _, s := range col.Statuses {
			if n := statusName[s.ID]; n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

// Apply writes the accepted proposal into a personal config layer.
func (p *Proposal) Apply(cfg *config.Config) {
	cfg.Hierarchy.Types = nil
	for _, r := range p.Roles {
		cfg.Hierarchy.Types = append(cfg.Hierarchy.Types, config.IssueTypeRole{ID: r.IssueType.ID, Name: r.IssueType.Name, Role: r.Role})
	}
	cfg.Workflow.States = map[string]config.State{}
	for _, s := range p.States {
		cfg.Workflow.States[s.Status.Name] = s.State
	}
	if len(p.Order) > 0 {
		cfg.Workflow.Order = p.Order
	}
	if p.Fields.StoryPoints != "" {
		cfg.Fields.StoryPoints = p.Fields.StoryPoints
	}
	if p.Fields.Sprint != "" {
		cfg.Fields.Sprint = p.Fields.Sprint
	}
	for i := range cfg.Projects {
		if id, ok := p.Boards[cfg.Projects[i].Key]; ok && cfg.Projects[i].Board == 0 {
			cfg.Projects[i].Board = id
		}
	}
}
