package kernel

// JournalPrefix is the cortex's note-file directory: the registered
// field, or "journal" for a registered cortex with an empty field.
// A nil cortex has no prefix.
func JournalPrefix(c *Cortex) string {
	if c == nil {
		return ""
	}
	if c.JournalPrefix != "" {
		return c.JournalPrefix
	}
	return "journal"
}

func effectiveJournalPrefix(c *Cortex) string {
	return JournalPrefix(c)
}

// CortexNamed returns the registered cortex with name, or nil.
func CortexNamed(cs []Cortex, name string) *Cortex {
	if name == "" {
		return nil
	}
	for i := range cs {
		if cs[i].Name == name {
			return &cs[i]
		}
	}
	return nil
}
