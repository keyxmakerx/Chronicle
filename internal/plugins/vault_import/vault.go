package vault_import

import (
	"context"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// maxPageName is the entities service's own limit on a page name.
const maxPageName = 200

// notionIDRe matches the 32-character id Notion appends to every page and
// folder name in a Markdown export ("Phandalin 3f2a...").
var notionIDRe = regexp.MustCompile(`^(.*\S)\s+[0-9a-f]{32}$`)

// DisplayName is a file or folder name as a person would say it: without
// Notion's trailing id, without surrounding spaces.
func DisplayName(stem string) string {
	stem = strings.TrimSpace(stem)
	if m := notionIDRe.FindStringSubmatch(stem); m != nil {
		stem = m[1]
	}
	return stem
}

// clipName keeps a page name inside the entities limit on a rune boundary.
func clipName(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxPageName {
		return s
	}
	s = s[:maxPageName]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return strings.TrimSpace(s)
}

// Note is one Markdown file that becomes a page.
type Note struct {
	Entry Entry
	Dir   string
	// Stem is the file name without extension and without Notion's id; links
	// find a note by it.
	Stem  string
	Title string
	FM    FrontMatter
	// BadFrontMatter is true when the file opened with a YAML block that could
	// not be read; the block is dropped and the text kept.
	BadFrontMatter bool
	// HadTags / HadVisibility record front matter keys Chronicle does not carry
	// over, so the preview can say so once instead of per page.
	HadTags       bool
	HadVisibility bool
}

// Page is one page the import will create, in creation order (parents first).
type Page struct {
	Name string
	// Note indexes Vault.Notes, or is -1 for a folder that has no note of its own.
	Note int
	// Parent indexes Vault.Pages, or is -1 for the top of the Imported folder.
	Parent int
}

// TargetKind says what a reference points at.
type TargetKind int

const (
	// TargetNone is a reference to something that is not in the zip.
	TargetNone TargetKind = iota
	// TargetNote is another note (Index is into Vault.Notes).
	TargetNote
	// TargetFile is a picture or other file (Index is into Vault.Files).
	TargetFile
	// TargetSelf is a link to a heading in the same note.
	TargetSelf
)

// Target is what a reference resolved to.
type Target struct {
	Kind  TargetKind
	Index int
}

// Vault is the analysed archive: nothing in it has been written anywhere.
type Vault struct {
	Name  string
	Notes []*Note
	// Files are the pictures and other files, in path order.
	Files []Entry
	Pages []Page
	Stats Stats

	noteByPath map[string]int   // folded path (with and without Notion ids) -> note
	noteByStem map[string][]int // folded stem -> notes
	fileByPath map[string]int
	fileByName map[string][]int
	// noteBySuffix / fileBySuffix list the items whose folded path ends, at a
	// folder boundary, with the key ("b/c" for "a/b/c"). A link written with
	// folders finds its candidates here in one lookup instead of walking every
	// path in the vault.
	noteBySuffix map[string][]int
	fileBySuffix map[string][]int
	folderNoteD  map[int]string // note index -> folder it stands for
	notePage     map[int]int    // note index -> index in Pages
	// resolved memoises Resolve: the answer depends only on the linking
	// note's folder, the reference kind and the written target.
	resolved map[resolveKey]Target
}

type resolveKey struct {
	dir, target string
	wiki        bool
}

// addSuffixes records idx under key and under every shorter tail of it that
// starts after a "/".
func addSuffixes(m map[string][]int, key string, idx int) {
	for k := key; ; {
		if l := m[k]; len(l) == 0 || l[len(l)-1] != idx {
			m[k] = append(l, idx)
		}
		i := strings.IndexByte(k, '/')
		if i < 0 {
			return
		}
		k = k[i+1:]
	}
}

// Stats is everything the preview reports.
type Stats struct {
	Pages       int // notes
	FolderPages int // folders with no note of their own
	FolderNotes int // folders that reuse their own note as the folder page

	LinksToPages int // [[wiki]] and relative links that became page links
	LinksPlain   int // links to notes not in the zip, kept as plain text
	// UnresolvedNames lists some of the names behind LinksPlain, for the preview.
	UnresolvedNames []string

	Pictures        int // distinct pictures placed inside text
	PictureRefs     int
	MissingPictures int // pictures a note shows that are not in the zip
	Files           int // distinct other files that will be attached
	FileRefs        int // (page, file) attachments
	// SkippedFiles are files left out, with the reason.
	SkippedFiles []SkippedEntry

	BadFrontMatter int
	TagPages       int // pages whose tags are not carried over
	VisibilityKeys int // pages whose visibility key is ignored
	NameClashes    int // notes sharing a name with another note
	CaseClashes    int
	Ignored        int
}

// maxRefsTotal bounds the links and pictures read across a whole vault; real
// vaults hold a few per note.
const maxRefsTotal = 1_000_000

// AttachCheck reports whether a file of this name can be attached to a page.
// The pages service decides; this package only asks.
type AttachCheck func(name string) bool

// Analyze reads every note once and works out the page tree, the links and the
// attachments, without writing anything. attachable is asked about each file
// that a note refers to but that cannot sit inside text.
func Analyze(ctx context.Context, a *Archive, attachable AttachCheck) (*Vault, error) {
	v := &Vault{
		Name:        a.Name,
		noteByPath:  map[string]int{},
		noteByStem:  map[string][]int{},
		fileByPath:  map[string]int{},
		fileByName:  map[string][]int{},
		folderNoteD: map[int]string{},

		noteBySuffix: map[string][]int{},
		fileBySuffix: map[string][]int{},
		resolved:     map[resolveKey]Target{},
	}
	v.Stats.Ignored = a.Ignored
	v.Stats.CaseClashes = len(a.CaseClashes)
	v.Stats.SkippedFiles = append(v.Stats.SkippedFiles, a.Skipped...)

	for _, e := range a.Entries {
		if e.Kind == KindNote {
			n := &Note{Entry: e, Dir: dirOf(e.Path)}
			base := path.Base(e.Path)
			n.Stem = DisplayName(strings.TrimSuffix(base, path.Ext(base)))
			n.Title = n.Stem
			idx := len(v.Notes)
			v.Notes = append(v.Notes, n)
			for _, k := range pathKeys(e.Path) {
				if _, taken := v.noteByPath[k]; !taken {
					v.noteByPath[k] = idx
					addSuffixes(v.noteBySuffix, k, idx)
				}
			}
			for _, k := range stemKeys(e.Path) {
				if l := v.noteByStem[k]; len(l) == 0 || l[len(l)-1] != idx {
					v.noteByStem[k] = append(v.noteByStem[k], idx)
				}
			}
			continue
		}
		idx := len(v.Files)
		v.Files = append(v.Files, e)
		fk := foldKey(e.Path)
		v.fileByPath[fk] = idx
		addSuffixes(v.fileBySuffix, fk, idx)
		k := foldKey(path.Base(e.Path))
		v.fileByName[k] = append(v.fileByName[k], idx)
	}
	for _, ks := range v.noteByStem {
		if len(ks) > 1 {
			v.Stats.NameClashes += len(ks)
		}
	}

	usedPictures := map[int]bool{}
	usedFiles := map[int]bool{}
	attachPairs := map[[2]int]bool{}
	seenUnresolved := map[string]bool{}

	refsSeen := 0
	for i, n := range v.Notes {
		// The zip is attacker input: stop when the caller is gone or the
		// time budget set by the service runs out.
		if err := ctx.Err(); err != nil {
			return nil, refuse("Reading this vault is taking too long, so it was stopped. Import a smaller part of it.")
		}
		raw, err := a.Read(n.Entry, a.limits.MaxNoteBytes)
		if err != nil {
			return nil, err
		}
		fm, body, hadBlock, ferr := SplitFrontMatter(string(raw))
		n.FM = fm
		if ferr != nil {
			n.BadFrontMatter = true
			v.Stats.BadFrontMatter++
		}
		if hadBlock && ferr == nil {
			if len(fm.Tags) > 0 {
				n.HadTags = true
				v.Stats.TagPages++
			}
			if fm.Visibility != "" {
				n.HadVisibility = true
				v.Stats.VisibilityKeys++
			}
		}
		if fm.Name != "" {
			n.Title = fm.Name
		}

		body = Prepare(body)
		found := ScanRefs(body)
		if refsSeen += len(found); refsSeen > maxRefsTotal {
			return nil, refuse("This vault holds more than %d links and pictures in its notes, which is over the limit. Import a smaller part of it.", maxRefsTotal)
		}
		for ri, r := range found {
			if ri%1024 == 1023 && ctx.Err() != nil {
				return nil, refuse("Reading this vault is taking too long, so it was stopped. Import a smaller part of it.")
			}
			if r.Remote {
				continue
			}
			t := v.Resolve(i, r)
			switch t.Kind {
			case TargetNote:
				// An embedded note is shown as a link to it.
				v.Stats.LinksToPages++
			case TargetNone:
				name := refLabel(r)
				if isImageRef(r) {
					v.Stats.MissingPictures++
				} else {
					v.Stats.LinksPlain++
					if len(v.Stats.UnresolvedNames) < 12 && !seenUnresolved[name] {
						seenUnresolved[name] = true
						v.Stats.UnresolvedNames = append(v.Stats.UnresolvedNames, name)
					}
				}
			case TargetFile:
				f := v.Files[t.Index]
				if f.Kind == KindPicture && (r.Kind == RefEmbed || r.Kind == RefImage) {
					v.Stats.PictureRefs++
					usedPictures[t.Index] = true
					continue
				}
				usedFiles[t.Index] = true
				attachPairs[[2]int{i, t.Index}] = true
			}
		}
	}

	canAttach := func(name string) bool { return attachable == nil || attachable(name) }
	for idx := range usedFiles {
		f := v.Files[idx]
		if !canAttach(path.Base(f.Path)) {
			v.Stats.SkippedFiles = append(v.Stats.SkippedFiles, SkippedEntry{Path: f.Path, Reason: "this kind of file can't be attached to a page"})
			delete(usedFiles, idx)
			continue
		}
		v.Stats.Files++
	}
	for pair := range attachPairs {
		if usedFiles[pair[1]] {
			v.Stats.FileRefs++
		}
	}
	listed := map[string]bool{}
	for _, s := range v.Stats.SkippedFiles {
		listed[s.Path] = true
	}
	for idx, f := range v.Files {
		switch {
		case usedPictures[idx]:
			v.Stats.Pictures++
		case usedFiles[idx], listed[f.Path]:
		default:
			v.Stats.SkippedFiles = append(v.Stats.SkippedFiles, SkippedEntry{Path: f.Path, Reason: "not used by any page"})
		}
	}
	sort.Slice(v.Stats.SkippedFiles, func(i, j int) bool { return v.Stats.SkippedFiles[i].Path < v.Stats.SkippedFiles[j].Path })

	v.buildTree()
	v.Stats.Pages = len(v.Notes)
	for _, p := range v.Pages {
		if p.Note < 0 {
			v.Stats.FolderPages++
		}
	}
	v.Stats.FolderNotes = len(v.folderNoteD)
	return v, nil
}

func isImageRef(r Ref) bool {
	return (r.Kind == RefEmbed || r.Kind == RefImage) && IsPictureName(r.Target)
}

// refLabel is how a reference reads in plain text: the alias if there is one,
// else the last part of the name it was written with.
func refLabel(r Ref) string {
	if r.Text != "" && r.Kind != RefImage {
		return r.Text
	}
	name := r.Target
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSuffix(strings.TrimSuffix(name, ".md"), ".markdown")
	if name == "" {
		name = r.Fragment
	}
	return name
}

func dirOf(p string) string {
	d := path.Dir(p)
	if d == "." {
		return ""
	}
	return d
}

func stripExt(p string) string { return strings.TrimSuffix(p, path.Ext(p)) }

// displayPath removes Notion's id from every segment of a path.
func displayPath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		if i == len(segs)-1 {
			ext := path.Ext(s)
			segs[i] = DisplayName(strings.TrimSuffix(s, ext)) + ext
			continue
		}
		segs[i] = DisplayName(s)
	}
	return strings.Join(segs, "/")
}

// pathKeys are the folded forms a note's path can be looked up by.
func pathKeys(p string) []string {
	a := foldKey(stripExt(p))
	b := foldKey(stripExt(displayPath(p)))
	if a == b {
		return []string{a}
	}
	return []string{a, b}
}

func stemKeys(p string) []string {
	base := stripExt(path.Base(p))
	a := foldKey(base)
	b := foldKey(DisplayName(base))
	if a == b {
		return []string{a}
	}
	return []string{a, b}
}

// Resolve finds what a reference written in note src points at, the way
// Obsidian does: a bare name matches a file name anywhere in the vault, a
// name with folders matches the end of a path, and when several notes share a
// name the one in the same folder, then the shallowest, wins.
func (v *Vault) Resolve(src int, r Ref) Target {
	if r.Remote || r.Target == "" {
		return v.resolve(src, r)
	}
	key := resolveKey{dir: v.Notes[src].Dir, target: r.Target, wiki: r.Kind == RefWiki || r.Kind == RefEmbed}
	if t, ok := v.resolved[key]; ok {
		return t
	}
	t := v.resolve(src, r)
	v.resolved[key] = t
	return t
}

func (v *Vault) resolve(src int, r Ref) Target {
	if r.Remote {
		return Target{Kind: TargetNone}
	}
	if r.Target == "" {
		if r.Fragment != "" {
			return Target{Kind: TargetSelf}
		}
		return Target{Kind: TargetNone}
	}
	n := v.Notes[src]
	if r.Kind == RefWiki || r.Kind == RefEmbed {
		if i, ok := v.lookupByName(n, r.Target); ok {
			return Target{Kind: TargetNote, Index: i}
		}
		if j, ok := v.lookupFile(n, r.Target); ok {
			return Target{Kind: TargetFile, Index: j}
		}
		return Target{Kind: TargetNone}
	}
	// Markdown link: a path relative to this note, or to the vault root.
	rel := r.Target
	var full string
	if strings.HasPrefix(rel, "/") {
		full = path.Clean(strings.TrimPrefix(rel, "/"))
	} else {
		full = path.Clean(path.Join(n.Dir, rel))
	}
	if full == ".." || strings.HasPrefix(full, "../") || full == "." {
		return Target{Kind: TargetNone}
	}
	if i, ok := v.noteByPath[foldKey(stripExt(full))]; ok && isNoteExt(full) {
		return Target{Kind: TargetNote, Index: i}
	}
	if j, ok := v.fileByPath[foldKey(full)]; ok {
		return Target{Kind: TargetFile, Index: j}
	}
	if i, ok := v.noteByPath[foldKey(full)]; ok && !isNoteExt(full) {
		return Target{Kind: TargetNote, Index: i}
	}
	// Exporters rename things; fall back to the name alone.
	base := path.Base(full)
	if isNoteExt(base) || path.Ext(base) == "" {
		if i, ok := v.lookupByName(n, stripExt(base)); ok {
			return Target{Kind: TargetNote, Index: i}
		}
	}
	if j, ok := v.lookupFile(n, base); ok {
		return Target{Kind: TargetFile, Index: j}
	}
	return Target{Kind: TargetNone}
}

func isNoteExt(p string) bool {
	e := strings.ToLower(path.Ext(p))
	return e == ".md" || e == ".markdown"
}

func (v *Vault) lookupByName(src *Note, name string) (int, bool) {
	name = strings.Trim(strings.TrimSpace(name), "/")
	if isNoteExt(name) {
		name = stripExt(name)
	}
	if name == "" {
		return 0, false
	}
	key := foldKey(name)
	var cands []int
	if strings.Contains(name, "/") {
		cands = v.noteBySuffix[key]
	} else {
		cands = v.noteByStem[key]
	}
	return v.pick(src, cands)
}

func (v *Vault) lookupFile(src *Note, name string) (int, bool) {
	name = strings.Trim(strings.TrimSpace(name), "/")
	if name == "" || path.Ext(name) == "" {
		return 0, false
	}
	key := foldKey(name)
	var cands []int
	if strings.Contains(name, "/") {
		cands = v.fileBySuffix[key]
	} else {
		cands = v.fileByName[key]
	}
	if len(cands) == 0 {
		return 0, false
	}
	best := cands[0]
	for _, c := range cands[1:] {
		if closer(src.Dir, v.Files[c].Path, v.Files[best].Path) {
			best = c
		}
	}
	return best, true
}

func (v *Vault) pick(src *Note, cands []int) (int, bool) {
	if len(cands) == 0 {
		return 0, false
	}
	best := cands[0]
	for _, c := range cands[1:] {
		if closer(src.Dir, v.Notes[c].Entry.Path, v.Notes[best].Entry.Path) {
			best = c
		}
	}
	return best, true
}

// closer reports whether candidate a is a better match than b for a link
// written in folder srcDir: same folder first, then fewer folders, then the
// earlier path, so the answer never depends on map order.
func closer(srcDir, a, b string) bool {
	sa, sb := dirOf(a) == srcDir, dirOf(b) == srcDir
	if sa != sb {
		return sa
	}
	da, db := strings.Count(a, "/"), strings.Count(b, "/")
	if da != db {
		return da < db
	}
	return a < b
}

// buildTree decides the page for every folder and note. A folder whose name
// matches a note beside it ("People.md" next to "People/"), inside it
// ("People/People.md") or that is its index ("People/index.md") is that note,
// so the folder does not become an empty page next to the real one.
func (v *Vault) buildTree() {
	dirs := map[string]bool{}
	for _, n := range v.Notes {
		for d := n.Dir; d != ""; d = dirOf(d) {
			dirs[d] = true
		}
	}
	// Notes by folder and their folded stems, so each folder's candidates are
	// found without walking every note in the vault.
	inDir := map[string][]int{}
	stemFold := make([]string, len(v.Notes))
	for i, n := range v.Notes {
		inDir[n.Dir] = append(inDir[n.Dir], i)
		stemFold[i] = foldKey(n.Stem)
	}
	folderNote := map[string]int{}
	claimed := map[int]bool{}
	dirList := make([]string, 0, len(dirs))
	for d := range dirs {
		dirList = append(dirList, d)
	}
	sort.Strings(dirList)
	for _, d := range dirList {
		base := DisplayName(path.Base(d))
		for _, cand := range []struct{ dir, stem string }{
			{dirOf(d), base}, {d, base}, {d, "index"},
		} {
			found := -1
			want := foldKey(cand.stem)
			for _, i := range inDir[cand.dir] {
				if !claimed[i] && stemFold[i] == want {
					found = i
					break
				}
			}
			if found >= 0 {
				folderNote[d] = found
				claimed[found] = true
				v.folderNoteD[found] = d
				break
			}
		}
	}

	dirPage := map[string]int{}
	notePage := map[int]int{}
	v.notePage = notePage
	var pageOfDir func(d string) int
	pageOfNote := func(i int) int {
		if p, ok := notePage[i]; ok {
			return p
		}
		n := v.Notes[i]
		parentDir := n.Dir
		if d, ok := v.folderNoteD[i]; ok {
			parentDir = dirOf(d)
		}
		parent := pageOfDir(parentDir)
		name := clipName(n.Title)
		// An index.md has no name of its own worth showing; it is the folder.
		if d, ok := v.folderNoteD[i]; ok && n.FM.Name == "" && foldKey(n.Stem) == "index" {
			name = clipName(DisplayName(path.Base(d)))
		}
		if name == "" {
			name = "Untitled"
		}
		v.Pages = append(v.Pages, Page{Name: name, Note: i, Parent: parent})
		notePage[i] = len(v.Pages) - 1
		return notePage[i]
	}
	pageOfDir = func(d string) int {
		if d == "" {
			return -1
		}
		if p, ok := dirPage[d]; ok {
			return p
		}
		if i, ok := folderNote[d]; ok {
			p := pageOfNote(i)
			dirPage[d] = p
			return p
		}
		parent := pageOfDir(dirOf(d))
		name := clipName(DisplayName(path.Base(d)))
		if name == "" {
			name = "Untitled"
		}
		v.Pages = append(v.Pages, Page{Name: name, Note: -1, Parent: parent})
		dirPage[d] = len(v.Pages) - 1
		return dirPage[d]
	}
	for i := range v.Notes {
		pageOfNote(i)
	}
}

// NotePage returns the index in Pages of a note's page.
func (v *Vault) NotePage(note int) int {
	if p, ok := v.notePage[note]; ok {
		return p
	}
	return -1
}

// OutlineLine is one folder in the preview's page tree.
type OutlineLine struct {
	Depth int
	Name  string
	// Pages is how many pages sit anywhere beneath this one.
	Pages int
}

// Outline lists the pages that have pages beneath them, indented by depth, so
// the preview can show the tree the folders will become. It stops at limit
// lines and reports how many were left out.
func (v *Vault) Outline(limit int) (lines []OutlineLine, more int) {
	count := make([]int, len(v.Pages))
	depth := make([]int, len(v.Pages))
	for i, p := range v.Pages {
		if p.Parent >= 0 {
			depth[i] = depth[p.Parent] + 1
		}
		for a := p.Parent; a >= 0; a = v.Pages[a].Parent {
			count[a]++
		}
	}
	for i, p := range v.Pages {
		if count[i] == 0 {
			continue
		}
		if len(lines) >= limit {
			more++
			continue
		}
		lines = append(lines, OutlineLine{Depth: depth[i], Name: p.Name, Pages: count[i]})
	}
	return lines, more
}
