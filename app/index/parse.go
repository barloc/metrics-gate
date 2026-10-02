package index

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

type rawTop struct {
	Metadata  rawMeta         `json:"metadata"`
	Macros    json.RawMessage `json:"macros"`
	Metrics   json.RawMessage `json:"metrics"`
	Exposures json.RawMessage `json:"exposures"`
}

type rawMeta struct {
	GeneratedAt string `json:"generated_at"`
}

type rawMacro struct {
	UniqueID         string `json:"unique_id"`
	Name             string `json:"name"`
	Path             string `json:"path"`
	OriginalFilePath string `json:"original_file_path"`
	Description      string `json:"description"`
	ResourceType     string `json:"resource_type"`
}

type rawSemanticMetric struct {
	UniqueID         string         `json:"unique_id"`
	Name             string         `json:"name"`
	Label            string         `json:"label"`
	Path             string         `json:"path"`
	OriginalFilePath string         `json:"original_file_path"`
	Description      string         `json:"description"`
	ResourceType     string         `json:"resource_type"`
	Meta             map[string]any `json:"meta"`
}

var (
	sqlFenceRE    = regexp.MustCompile("(?s)```sql\\s*(.*?)```")
	titleRE       = regexp.MustCompile(`(?m)^###\s+(.+)$`)
	macroLinkRE   = regexp.MustCompile(`#!/macro/macro\.[^./]+\.([A-Za-z0-9_]+)`)
	macroUIDRE    = regexp.MustCompile(`^macro\.[^.]+\.([A-Za-z0-9_]+)$`)
	metricUIDRE   = regexp.MustCompile(`^metric\.[^.]+\.([A-Za-z0-9_]+)$`)
	dimTableRowRE = regexp.MustCompile(`\|\s*\*\*([A-Za-z0-9_]+)\*\*\s*\|([^|]*)\|([^|]*)\|`)
)

type rawExposure struct {
	Name        string     `json:"name"`
	UniqueID    string     `json:"unique_id"`
	Description string     `json:"description"`
	DependsOn   rawDepends `json:"depends_on"`
}

type rawDepends struct {
	Macros []string `json:"macros"`
	Nodes  []string `json:"nodes"`
}

// Build parses dbt manifest.json into MetricCards (ManifestSource).
// catalog.json is never read. Overlay aliases/apply_column are merged after parse.
// Metrics from:
//   - macros under macros/metrics/ (case-insensitive path; overridable via BuildOpts),
//     optionally filtered to exposure analytics_metrics_core;
//   - top-level semantic metrics (MetricFlow), filtered to exposure
//     depends_on.nodes metric.* — never load all metrics blindly.
// Dimensions: catalog rows from exposure analytics_dimensions; macros under
// macros/dimensions/ enrich methodology only (never define the catalog).
func Build(body []byte, product, checksum, etag string, overlay Overlay, opts BuildOpts) (*Index, error) {
	opts = opts.withDefaults()
	var top rawTop
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, fmt.Errorf("index: parse top: %w", err)
	}

	idx := &Index{
		Meta: Meta{
			Product:             product,
			Checksum:            checksum,
			ManifestGeneratedAt: top.Metadata.GeneratedAt,
			ETag:                etag,
			DocsSQLDialect:      opts.DocsSQLDialect,
			ApplyWarehouse:      opts.ApplyWarehouse,
		},
		ByID:       make(map[string]*MetricCard),
		ByAlias:    make(map[string]string),
		DimByID:    make(map[string]*MetricCard),
		DimByAlias: make(map[string]string),
	}

	macroIDs, metricIDs, err := coreCatalogIDs(top.Exposures, opts.MetricsExposure)
	if err != nil {
		return nil, err
	}
	if err := idx.loadMacros(top.Macros, macroIDs, opts); err != nil {
		return nil, err
	}
	if err := idx.loadSemanticMetrics(top.Metrics, metricIDs, opts); err != nil {
		return nil, err
	}
	if err := idx.loadDimensions(top.Exposures, top.Macros, opts); err != nil {
		return nil, err
	}

	idx.applyOverlay(overlay)
	sort.Slice(idx.Cards, func(i, j int) bool { return idx.Cards[i].ID < idx.Cards[j].ID })
	sort.Slice(idx.DimCards, func(i, j int) bool { return idx.DimCards[i].ID < idx.DimCards[j].ID })
	idx.buildAliasIndex()
	idx.buildDimAliasIndex()
	idx.buildSearchDocs()
	return idx, nil
}

// coreCatalogIDs returns macro filter (nil = all metrics macros) and semantic
// metric filter (nil/empty = load no semantic metrics).
func coreCatalogIDs(raw json.RawMessage, metricsExposure string) (macroIDs, metricIDs map[string]struct{}, err error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil, nil
	}
	var exposures map[string]rawExposure
	if err := json.Unmarshal(raw, &exposures); err != nil {
		return nil, nil, fmt.Errorf("index: parse exposures: %w", err)
	}
	var exp *rawExposure
	for _, e := range exposures {
		if e.Name == metricsExposure || strings.HasSuffix(e.UniqueID, "."+metricsExposure) {
			cp := e
			exp = &cp
			break
		}
	}
	if exp == nil {
		return nil, nil, nil
	}
	macroIDs = make(map[string]struct{})
	metricIDs = make(map[string]struct{})
	for _, uid := range exp.DependsOn.Macros {
		if id := macroNameFromUID(uid); id != "" {
			macroIDs[id] = struct{}{}
		}
	}
	for _, m := range macroLinkRE.FindAllStringSubmatch(exp.Description, -1) {
		if len(m) == 2 && m[1] != "" {
			macroIDs[m[1]] = struct{}{}
		}
	}
	for _, uid := range exp.DependsOn.Nodes {
		if id := metricNameFromUID(uid); id != "" {
			metricIDs[id] = struct{}{}
		}
	}
	if len(macroIDs) == 0 {
		macroIDs = nil // fallback: keep all macros under metrics_macro_prefix
	}
	if len(metricIDs) == 0 {
		metricIDs = nil // do not load semantic metrics without explicit refs
	}
	return macroIDs, metricIDs, nil
}

func macroNameFromUID(uid string) string {
	if m := macroUIDRE.FindStringSubmatch(uid); len(m) == 2 {
		return m[1]
	}
	return ""
}

func metricNameFromUID(uid string) string {
	if m := metricUIDRE.FindStringSubmatch(uid); len(m) == 2 {
		return m[1]
	}
	return ""
}

func pathHasPrefixFold(p, prefix string) bool {
	if len(p) < len(prefix) {
		return false
	}
	return strings.EqualFold(p[:len(prefix)], prefix)
}

func stripPrefixFold(p, prefix string) string {
	if !pathHasPrefixFold(p, prefix) {
		return p
	}
	return p[len(prefix):]
}

func (idx *Index) loadMacros(raw json.RawMessage, coreIDs map[string]struct{}, opts BuildOpts) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var macros map[string]rawMacro
	if err := json.Unmarshal(raw, &macros); err != nil {
		return fmt.Errorf("index: parse macros: %w", err)
	}
	for uid, rm := range macros {
		p := rm.Path
		if p == "" {
			p = rm.OriginalFilePath
		}
		if !pathHasPrefixFold(p, opts.MetricsMacroPrefix) {
			continue
		}
		id := rm.Name
		if id == "" {
			continue
		}
		if coreIDs != nil {
			if _, ok := coreIDs[id]; !ok {
				continue
			}
		}
		card := newPublishedMetricCard(
			id,
			firstNonEmpty(rm.UniqueID, uid),
			extractTitle(rm.Description, id),
			categoryFromPath(p, opts.MetricsMacroPrefix),
			rm.Description,
			rm.Description,
			p,
			opts,
		)
		idx.ByID[id] = card
		idx.Cards = append(idx.Cards, card)
	}
	return nil
}

func (idx *Index) loadSemanticMetrics(raw json.RawMessage, coreIDs map[string]struct{}, opts BuildOpts) error {
	if len(coreIDs) == 0 {
		return nil
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var metrics map[string]rawSemanticMetric
	if err := json.Unmarshal(raw, &metrics); err != nil {
		return fmt.Errorf("index: parse metrics: %w", err)
	}
	for uid, rm := range metrics {
		id := rm.Name
		if id == "" {
			id = metricNameFromUID(firstNonEmpty(rm.UniqueID, uid))
		}
		if id == "" {
			continue
		}
		if _, ok := coreIDs[id]; !ok {
			continue
		}
		if _, exists := idx.ByID[id]; exists {
			continue // macros win on id clash
		}
		p := rm.Path
		if p == "" {
			p = rm.OriginalFilePath
		}
		name := strings.TrimSpace(rm.Label)
		if name == "" {
			name = extractTitle(rm.Description, id)
		}
		card := newPublishedMetricCard(
			id,
			firstNonEmpty(rm.UniqueID, uid),
			name,
			categoryFromSemanticPath(p),
			rm.Description,
			composeSemanticMethodology(rm),
			p,
			opts,
		)
		if sn, ok := rm.Meta["short_name"].(string); ok && strings.TrimSpace(sn) != "" {
			card.Aliases = appendUnique(card.Aliases, strings.TrimSpace(sn))
		}
		idx.ByID[id] = card
		idx.Cards = append(idx.Cards, card)
	}
	return nil
}

func newPublishedMetricCard(id, uniqueID, name, category, description, methodologySrc, path string, opts BuildOpts) *MetricCard {
	return &MetricCard{
		ResourceType: ResourceMetric,
		ID:           id,
		UniqueID:     uniqueID,
		Name:         name,
		Category:     category,
		Methodology:  capMethodology(methodologySrc),
		ExampleSQL: ExampleSQL{
			Dialect:    opts.DocsSQLDialect,
			SQL:        extractSQL(description),
			Executable: false,
		},
		Caveat: opts.caveat(),
		Status: "published",
		Path:   path,
		Short:  TruncateDesc(firstParagraph(description)),
	}
}

func composeSemanticMethodology(rm rawSemanticMetric) string {
	var b strings.Builder
	b.WriteString(rm.Description)
	if len(rm.Meta) == 0 {
		return b.String()
	}
	if f, ok := rm.Meta["formula"].(string); ok && strings.TrimSpace(f) != "" {
		b.WriteString("\n\n**Formula:** ")
		b.WriteString(strings.TrimSpace(f))
	}
	if u, ok := rm.Meta["unit"].(string); ok && strings.TrimSpace(u) != "" {
		b.WriteString("\n\n**Unit:** ")
		b.WriteString(strings.TrimSpace(u))
	}
	switch p := rm.Meta["pitfalls"].(type) {
	case string:
		if strings.TrimSpace(p) != "" {
			b.WriteString("\n\n**Pitfalls:** ")
			b.WriteString(strings.TrimSpace(p))
		}
	case []any:
		var parts []string
		for _, x := range p {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				parts = append(parts, strings.TrimSpace(s))
			}
		}
		if len(parts) > 0 {
			b.WriteString("\n\n**Pitfalls:** ")
			b.WriteString(strings.Join(parts, "; "))
		}
	}
	return b.String()
}

func categoryFromSemanticPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	dir := path.Dir(p)
	base := path.Base(dir)
	if base == "." || base == "/" || base == "" {
		return ""
	}
	return base
}

// loadDimensions builds dimension cards from exposure dimensions table rows.
// Macros under macros/dimensions/ enrich methodology when linked or named
// get_dim_<id>; they never define the catalog alone.
func (idx *Index) loadDimensions(exposuresRaw, macrosRaw json.RawMessage, opts BuildOpts) error {
	exp, err := findExposure(exposuresRaw, opts.DimensionsExposure)
	if err != nil {
		return err
	}
	if exp == nil {
		return nil
	}

	dimMacros, err := dimensionMacros(macrosRaw, opts.DimensionsMacroPrefix)
	if err != nil {
		return err
	}

	caveat := opts.caveat()
	rows := parseDimensionTableRows(exp.Description)
	for _, row := range rows {
		id := row.ID
		if id == "" || shouldSkipDimensionID(id) {
			continue
		}
		short := strings.TrimSpace(row.Short)
		methodSrc := short
		uniqueID := "dimension." + id
		path := "exposure:" + opts.DimensionsExposure

		if m, ok := pickDimEnrichMacro(dimMacros, row.MacroName, id); ok {
			methodSrc = m.Description
			uniqueID = firstNonEmpty(m.UniqueID, uniqueID)
			path = firstNonEmpty(m.Path, m.OriginalFilePath, path)
		}

		method := capMethodology(methodSrc)
		card := &MetricCard{
			ResourceType: ResourceDimension,
			ID:           id,
			UniqueID:     uniqueID,
			Name:         id, // catalog id is the stable display key (do not scrape ### from long docs)
			Methodology:  method,
			ExampleSQL: ExampleSQL{
				Dialect:    opts.DocsSQLDialect,
				SQL:        extractSQL(methodSrc),
				Executable: false,
			},
			Caveat: caveat,
			Status: "published",
			Path:   path,
			Short:  TruncateDesc(firstNonEmpty(short, firstParagraph(methodSrc))),
		}
		idx.DimByID[id] = card
		idx.DimCards = append(idx.DimCards, card)
	}
	return nil
}

// pickDimEnrichMacro returns a dimensions macro with non-empty docs to enrich
// methodology. Prefer #!/macro link from the exposure row; else get_dim_<id>.
// Empty descriptions (common for SQL-only helpers) must not override path/text.
func pickDimEnrichMacro(dimMacros map[string]rawMacro, linkName, id string) (rawMacro, bool) {
	try := func(name string) (rawMacro, bool) {
		if name == "" {
			return rawMacro{}, false
		}
		m, ok := dimMacros[name]
		if !ok || strings.TrimSpace(m.Description) == "" {
			return rawMacro{}, false
		}
		return m, true
	}
	if m, ok := try(linkName); ok {
		return m, true
	}
	return try("get_dim_" + id)
}

type dimTableRow struct {
	ID        string
	Short     string
	MacroName string // from #!/macro link, if any
}

func parseDimensionTableRows(desc string) []dimTableRow {
	layout := detectDimTableLayout(desc)
	matches := dimTableRowRE.FindAllStringSubmatch(desc, -1)
	out := make([]dimTableRow, 0, len(matches))
	seen := make(map[string]struct{})
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		id := strings.TrimSpace(m[1])
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		col2 := strings.TrimSpace(m[2])
		col3 := strings.TrimSpace(m[3])
		macroName := ""
		if mm := macroLinkRE.FindStringSubmatch(col3); len(mm) == 2 {
			macroName = mm[1]
		}
		short := dimShortFromCols(layout, col2, col3, macroName)
		out = append(out, dimTableRow{ID: id, Short: short, MacroName: macroName})
	}
	return out
}

func shouldSkipDimensionID(id string) bool {
	// Skip docs noise keys if they ever appear as **id** cells.
	if id == "dimension" || id == "description" || id == "разрез" {
		return true
	}
	if strings.HasPrefix(id, "dimension_table_header") {
		return true
	}
	if strings.HasPrefix(id, "dimension_description_") {
		return true
	}
	return false
}

func findExposure(raw json.RawMessage, name string) (*rawExposure, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var exposures map[string]rawExposure
	if err := json.Unmarshal(raw, &exposures); err != nil {
		return nil, fmt.Errorf("index: parse exposures: %w", err)
	}
	for _, e := range exposures {
		if e.Name == name || strings.HasSuffix(e.UniqueID, "."+name) {
			cp := e
			return &cp, nil
		}
	}
	return nil, nil
}

func dimensionMacros(raw json.RawMessage, dimensionsPrefix string) (map[string]rawMacro, error) {
	out := make(map[string]rawMacro)
	if len(raw) == 0 || string(raw) == "null" {
		return out, nil
	}
	var macros map[string]rawMacro
	if err := json.Unmarshal(raw, &macros); err != nil {
		return nil, fmt.Errorf("index: parse macros: %w", err)
	}
	for uid, rm := range macros {
		p := rm.Path
		if p == "" {
			p = rm.OriginalFilePath
		}
		if !pathHasPrefixFold(p, dimensionsPrefix) {
			continue
		}
		name := rm.Name
		if name == "" {
			continue
		}
		if rm.UniqueID == "" {
			rm.UniqueID = uid
		}
		if rm.Path == "" {
			rm.Path = p
		}
		out[name] = rm
	}
	return out, nil
}

func (idx *Index) applyOverlay(ov Overlay) {
	for _, m := range ov.Metrics {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		card, ok := idx.ByID[id]
		if !ok {
			continue
		}
		if col := strings.TrimSpace(m.ApplyColumn); col != "" {
			card.ApplyColumn = col
		}
		for _, a := range m.Aliases {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			card.Aliases = appendUnique(card.Aliases, a)
		}
	}
}

func (idx *Index) buildAliasIndex() {
	for id, card := range idx.ByID {
		idx.ByAlias[normalizeKey(id)] = id
		idx.ByAlias[normalizeKey(card.Name)] = id
		for _, a := range card.Aliases {
			idx.ByAlias[normalizeKey(a)] = id
		}
	}
}

func (idx *Index) buildDimAliasIndex() {
	for id, card := range idx.DimByID {
		idx.DimByAlias[normalizeKey(id)] = id
		idx.DimByAlias[normalizeKey(card.Name)] = id
		for _, a := range card.Aliases {
			idx.DimByAlias[normalizeKey(a)] = id
		}
	}
}

func (idx *Index) buildSearchDocs() {
	docs := make([]SearchDoc, 0, len(idx.Cards)+len(idx.DimCards))
	appendCardDocs := func(cards []*MetricCard) {
		for _, card := range cards {
			parts := []string{card.ID, card.Name, card.Category, card.ApplyColumn, card.Short, card.Caveat}
			parts = append(parts, card.Aliases...)
			parts = append(parts, TruncateDesc(card.Methodology))
			docs = append(docs, SearchDoc{
				ResourceType: card.ResourceType,
				ID:           card.ID,
				UniqueID:     card.UniqueID,
				Name:         card.Name,
				Aliases:      append([]string(nil), card.Aliases...),
				Category:     card.Category,
				ApplyColumn:  card.ApplyColumn,
				Short:        card.Short,
				SearchText:   strings.ToLower(strings.Join(parts, " ")),
			})
		}
	}
	appendCardDocs(idx.Cards)
	appendCardDocs(idx.DimCards)
	idx.SearchDocs = docs
}

func categoryFromPath(p, metricsPrefix string) string {
	rel := stripPrefixFold(p, metricsPrefix)
	dir := path.Dir(rel)
	if dir == "." || dir == "" {
		return ""
	}
	return dir
}

func extractTitle(desc, fallback string) string {
	if m := titleRE.FindStringSubmatch(desc); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return fallback
}

func extractSQL(desc string) string {
	m := sqlFenceRE.FindStringSubmatch(desc)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func firstParagraph(desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return ""
	}
	// Skip leading ### title line for short blurb.
	lines := strings.Split(desc, "\n")
	var b strings.Builder
	started := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if !started {
			if trim == "" || strings.HasPrefix(trim, "###") {
				continue
			}
			started = true
		}
		if trim == "" && b.Len() > 0 {
			break
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(trim)
	}
	return b.String()
}

func capMethodology(s string) string {
	if len(s) <= methodologyCapBytes {
		return s
	}
	// Cut on rune boundary.
	for len(s) > methodologyCapBytes {
		_, size := utf8.DecodeLastRuneInString(s[:methodologyCapBytes+1])
		s = s[:methodologyCapBytes+1-size]
	}
	return s
}

// TruncateDesc shortens text for search hits.
func TruncateDesc(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= descHitCap {
		return s
	}
	runes := []rune(s)
	return string(runes[:descHitCap]) + "…"
}

func normalizeKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func appendUnique(ss []string, v string) []string {
	for _, s := range ss {
		if strings.EqualFold(s, v) {
			return ss
		}
	}
	return append(ss, v)
}
