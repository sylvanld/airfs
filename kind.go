// Package airfs presents the resources of many repositories as one merged,
// read-only view. See docs/specs/layered-resources.md for the model.
package airfs

// Kind is a category of resource, corresponding to one subdirectory name
// within a source.
type Kind string

// The kinds are fixed and built in; they are not configurable.
const (
	Agents   Kind = "agents"
	Skills   Kind = "skills"
	Commands Kind = "commands"
	Scripts  Kind = "scripts"
)

// Kinds lists every kind, in the order reports present them.
var Kinds = []Kind{Agents, Skills, Commands, Scripts}

func (k Kind) String() string { return string(k) }
