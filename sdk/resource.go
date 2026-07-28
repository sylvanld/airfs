// Package airfs presents the resources of many repositories as one merged,
// read-only view. See docs/specs/layered-resources.md for the model.
//
// It holds what every other package shares: the vocabulary of resource kinds,
// and the failure protocol saying which failures a caller is meant to act on.
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
