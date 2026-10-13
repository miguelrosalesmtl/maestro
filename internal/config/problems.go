package config

import (
	"fmt"
	"strings"
)

type Severity string

const (
	SeverityError   Severity = "error"   // the file is rejected
	SeverityWarning Severity = "warning" // shown, but the file is accepted (e.g. rule 18.7)
)

// Problem is one finding about a configuration file.
type Problem struct {
	Severity Severity `json:"severity"`
	File     string   `json:"file,omitempty"`
	Line     int      `json:"line,omitempty"`
	Path     string   `json:"path"` // YAML key path, e.g. "projects[1].repos[0].name"
	Message  string   `json:"message"`
}

func (p Problem) String() string {
	var loc strings.Builder
	if p.File != "" {
		loc.WriteString(p.File)
		if p.Line > 0 {
			fmt.Fprintf(&loc, ":%d", p.Line)
		}
		loc.WriteString(": ")
	}
	if p.Path != "" {
		loc.WriteString(p.Path + ": ")
	}
	return fmt.Sprintf("%s%s (%s)", loc.String(), p.Message, p.Severity)
}

// Problems is the full list of findings. Rule 18.9: report all, not the first.
type Problems []Problem

func (ps Problems) Errors() Problems   { return ps.filter(SeverityError) }
func (ps Problems) Warnings() Problems { return ps.filter(SeverityWarning) }
func (ps Problems) HasErrors() bool    { return len(ps.Errors()) > 0 }

func (ps Problems) filter(s Severity) Problems {
	var out Problems
	for _, p := range ps {
		if p.Severity == s {
			out = append(out, p)
		}
	}
	return out
}

// InvalidError is returned when a configuration has at least one error.
// It carries every problem, including warnings, so the UI can show them together.
type InvalidError struct{ Problems Problems }

func (e *InvalidError) Error() string {
	errs := e.Problems.Errors()
	lines := make([]string, len(errs))
	for i, p := range errs {
		lines[i] = "  - " + p.String()
	}
	return fmt.Sprintf("configuration has %d error(s):\n%s", len(errs), strings.Join(lines, "\n"))
}

// collector accumulates problems with a shared file name.
type collector struct {
	file string
	ps   Problems
}

func (c *collector) errorf(path, format string, args ...any) {
	c.ps = append(c.ps, Problem{Severity: SeverityError, File: c.file, Path: path, Message: fmt.Sprintf(format, args...)})
}

func (c *collector) warnf(path, format string, args ...any) {
	c.ps = append(c.ps, Problem{Severity: SeverityWarning, File: c.file, Path: path, Message: fmt.Sprintf(format, args...)})
}

func (c *collector) at(line int, path, format string, args ...any) {
	c.ps = append(c.ps, Problem{Severity: SeverityError, File: c.file, Line: line, Path: path, Message: fmt.Sprintf(format, args...)})
}
