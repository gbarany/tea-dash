package data

// MergeCapabilities describes the merge controls that should be offered for a
// pull request. Strategy fields are usually repository-scoped settings from
// Gitea/Forgejo; ForceMerge and AutoMerge are server/action capabilities.
type MergeCapabilities struct {
	Merge           bool
	Squash          bool
	Rebase          bool
	RebaseMerge     bool
	FastForwardOnly bool
	ForceMerge      bool
	AutoMerge       bool
}

// DefaultMergeCapabilities preserves the optimistic behavior used when no
// repository capability probe is available.
func DefaultMergeCapabilities() MergeCapabilities {
	return MergeCapabilities{
		Merge:           true,
		Squash:          true,
		Rebase:          true,
		RebaseMerge:     true,
		FastForwardOnly: true,
		ForceMerge:      true,
		AutoMerge:       true,
	}
}

// SupportsStyle reports whether a merge strategy should be offered.
func (c MergeCapabilities) SupportsStyle(style MergeStyle) bool {
	switch style {
	case MergeStyleMerge:
		return c.Merge
	case MergeStyleSquash:
		return c.Squash
	case MergeStyleRebase:
		return c.Rebase
	case MergeStyleRebaseMerge:
		return c.RebaseMerge
	case MergeStyleFastForwardOnly:
		return c.FastForwardOnly
	default:
		return false
	}
}
