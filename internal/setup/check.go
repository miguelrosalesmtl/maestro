package setup

import (
	"fmt"
	"slices"

	"maestro/internal/config"
	"maestro/internal/tracker/jira"
)

// CheckAgainst validates the configuration against what Jira reports
// (configuration spec 18.2–18.6). Offline checks
// are in config.Validate.
func CheckAgainst(cfg *config.Config, d *jira.Discovery) config.Problems {
	var ps config.Problems
	errf := func(path, f string, a ...any) {
		ps = append(ps, config.Problem{Severity: config.SeverityError, Path: path, Message: fmt.Sprintf(f, a...)})
	}
	warnf := func(path, f string, a ...any) {
		ps = append(ps, config.Problem{Severity: config.SeverityWarning, Path: path, Message: fmt.Sprintf(f, a...)})
	}

	// 18.2: projects exist.
	for i, p := range cfg.Projects {
		found := false
		for _, pd := range d.Projects {
			if pd.Project.Key == p.Key && pd.Project.ID != "" {
				found = true
			}
		}
		if !found {
			errf(fmt.Sprintf("projects[%d].key", i), "project %s does not exist or you have no access", p.Key)
		}
	}

	// 18.3: mapped statuses exist.
	statuses := map[string]bool{}
	for _, pd := range d.Projects {
		for _, s := range pd.Statuses {
			statuses[s.Name], statuses[s.ID] = true, true
		}
	}
	for name := range cfg.Workflow.States {
		if len(statuses) > 0 && !statuses[name] {
			warnf("workflow.states."+name, "no status %q in the projects in scope", name)
		}
	}

	// Custom fields exist; fields shown in create must be editable.
	for i, f := range cfg.Fields.Custom {
		path := fmt.Sprintf("fields.custom[%d]", i)
		field := d.FieldByIDOrName(f.Ref())
		if field == nil {
			errf(path, "Jira has no field %q", f.Ref())
			continue
		}
		if !field.Custom {
			warnf(path, "%q is a system field; Maestro already shows it", field.Name)
		}
		if f.Shows("create") && !jira.KindOf(field.Schema).Editable() {
			errf(path+".show", "%q has type %q, which Maestro cannot edit yet; remove \"create\"", field.Name, jira.KindOf(field.Schema))
		}
	}

	// The default fix version exists, unreleased, in each project.
	for _, k := range []struct{ path, v string }{
		{"defaults.create.fixVersion", cfg.Defaults.Create.FixVersion},
		{"defaults.create.affectsVersion", cfg.Defaults.Create.AffectsVersion},
	} {
		if k.v == "none" || k.v == "next" || k.v == "" {
			continue
		}
		for _, pd := range d.Projects {
			ok := slices.ContainsFunc(pd.Versions, func(v jira.Version) bool { return v.Name == k.v && !v.Released })
			if !ok && pd.Project.ID != "" {
				errf(k.path, "%q is not an unreleased version of %s", k.v, pd.Project.Key)
			}
		}
	}

	// The story points field agrees with the board (warn only), and the site
	// does not have two story point fields.
	std := d.Standard()
	sp := cfg.Fields.StoryPoints
	if sp != "" && sp != config.StoryPointsAuto && std.StoryPoints != "" && sp != std.StoryPoints {
		warnf("fields.storyPoints", "the board estimates with %s, not %s; totals may not match the board", std.StoryPoints, sp)
	}
	if std.StoryPointsAlt != "" {
		warnf("fields.storyPoints", "this site has two story point fields (%s and %s); Maestro uses %s",
			std.StoryPoints, std.StoryPointsAlt, std.StoryPoints)
	}
	if std.StoryPoints == "" && sp == config.StoryPointsAuto {
		warnf("fields.storyPoints", "no story points field found; totals are hidden")
	}

	return ps
}
