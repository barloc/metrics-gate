package index

import "strings"

// dimTableLayout selects which exposure-table column holds the short description.
type dimTableLayout int

const (
	// dimLayoutShortDesc: | id | short | optional link |
	dimLayoutShortDesc dimTableLayout = iota
	// dimLayoutValuesDesc: | id | values | description | sources |
	dimLayoutValuesDesc
)

// detectDimTableLayout infers column roles from markdown header lines in the
// exposure description (not from cell content heuristics).
func detectDimTableLayout(desc string) dimTableLayout {
	for _, line := range strings.Split(desc, "\n") {
		trim := strings.TrimSpace(strings.ToLower(line))
		if !strings.HasPrefix(trim, "|") {
			continue
		}
		// 4-col TOC: | dimension | values | description | source | (or localized)
		if strings.Contains(trim, "значен") || strings.Contains(trim, "разрез") {
			return dimLayoutValuesDesc
		}
		if strings.Contains(trim, "values") && strings.Contains(trim, "description") {
			return dimLayoutValuesDesc
		}
	}
	return dimLayoutShortDesc
}

// dimShortFromCols picks methodology short text for one data row.
func dimShortFromCols(layout dimTableLayout, col2, col3, macroName string) string {
	if macroName != "" {
		return col2
	}
	switch layout {
	case dimLayoutValuesDesc:
		if col3 != "" {
			return col3
		}
		return col2
	default:
		if col2 != "" {
			return col2
		}
		return col3
	}
}
