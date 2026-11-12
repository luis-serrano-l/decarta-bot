package menu

import (
	"fmt"
	"strings"
)

// maxIssues bounds how many issues one error carries. A hostile document
// could otherwise produce one issue per byte.
const maxIssues = 50

// Issue is one problem found in a submitted menu, written for the person
// who has to fix it.
type Issue struct {
	// Path locates the problem: a JSON field path such as
	// "categories[0].items[2].price", or "line 7" for the list format and
	// for JSON syntax errors.
	Path string
	// Problem says what is wrong at Path.
	Problem string
	// Fix says how to correct it.
	Fix string
}

// ValidationError is returned by Parse, ParseJSON, ParseList and Validate
// when the input is not an acceptable menu. Its text is safe to show to the
// user.
type ValidationError struct {
	// Issues holds the first problems found, in document order.
	Issues []Issue
	// Total is how many problems were found; it exceeds len(Issues) when
	// the list was cut short.
	Total int
}

// Error lists the retained issues on one line, as "path: problem".
func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Issues))
	for i, is := range e.Issues {
		parts[i] = is.Path + ": " + is.Problem
	}
	s := strings.Join(parts, "; ")
	if more := e.Total - len(e.Issues); more > 0 {
		s += fmt.Sprintf("; and %d more", more)
	}
	return s
}

// issues accumulates problems while a document is walked.
type issues struct {
	list  []Issue
	total int
}

func (v *issues) add(path, problem, fix string) {
	v.total++
	if len(v.list) < maxIssues {
		v.list = append(v.list, Issue{Path: path, Problem: problem, Fix: fix})
	}
}

// err returns nil when nothing was found. The return type is the error
// interface so that a nil result compares equal to nil.
func (v *issues) err() error {
	if v.total == 0 {
		return nil
	}
	return &ValidationError{Issues: v.list, Total: v.total}
}

// single builds an error carrying one issue.
func single(path, problem, fix string) error {
	return &ValidationError{Issues: []Issue{{Path: path, Problem: problem, Fix: fix}}, Total: 1}
}
