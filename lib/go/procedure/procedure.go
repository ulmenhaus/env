// Package procedure parses procedure bodies, which are markdown lists of
// hyphenated bullets, into a sequence of steps.
package procedure

import "strings"

// Step is a single actionable item parsed from a procedure's body.
type Step struct {
	Description string
	Details     string
}

// ParseSteps splits a procedure body into its component Steps. Each "\n- "
// delimited item becomes one Step, whose Description is the item's first
// line and whose Details are the remaining lines, sanitized by trimming
// whitespace and dropping blank lines and code fences.
func ParseSteps(text string) []Step {
	items := strings.Split(text, "\n- ")
	steps := []Step{}
	for _, item := range items {
		lines := strings.Split(item, "\n")
		if lines[0] == "" {
			continue
		}
		details := ""
		for _, line := range lines[1:] {
			if strings.HasPrefix(line, "```") || line == "" {
				continue
			}
			details += strings.TrimSpace(line) + "\n"
		}
		steps = append(steps, Step{
			Description: lines[0],
			Details:     details,
		})
	}
	return steps
}
