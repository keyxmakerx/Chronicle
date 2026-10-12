package vault_import

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/pagelink"
)

// rootBaseName is the folder every import lands in, so nothing mixes into the
// existing page tree.
const rootBaseName = "Imported"

// unfinishedSuffix marks the folder of an import that stopped partway, so no
// one mistakes it for a complete copy.
const unfinishedSuffix = " (unfinished)"

type pictureResult struct {
	id  string
	err string
}

// runner carries one import from start to finish. It is the only code that
// writes to the campaign.
type runner struct {
	s   *Service
	job *Job
	up  *upload
	v   *Vault
	arc *Archive

	campaignID, userID string
	rootID, rootName   string
	pageIDs            []string
	kinds              map[string]PageKind
	defaultKind        PageKind
	sum                Summary
	pictures           map[int]pictureResult
	created            int
}

// run executes a job and always leaves it finished: done, or failed with a
// message that says what was and was not written.
func (s *Service) run(job *Job, up *upload) {
	defer s.finish(job)
	defer func() { _ = os.Remove(up.path) }()

	ctx, cancel := context.WithTimeout(s.d.Lifecycle, 3*time.Hour)
	defer cancel()

	r := &runner{s: s, job: job, up: up, v: up.vault, campaignID: job.CampaignID, userID: job.UserID,
		pictures: map[int]pictureResult{}, pageIDs: make([]string, len(up.vault.Pages))}
	r.sum.VaultName = up.vault.Name
	r.sum.SourceFileName = up.fileName

	var err error
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("vault import: panic", slog.Any("panic", rec), slog.String("campaign_id", job.CampaignID))
				err = errors.New("something went wrong inside the importer")
			}
		}()
		err = r.execute(ctx)
	}()
	if err != nil {
		r.fail(err)
		return
	}
	s.audit(job, r, "campaign.vault_import.finished")
}

func (s *Service) audit(job *Job, r *runner, action string) {
	if s.d.Audit == nil {
		return
	}
	s.d.Audit.LogCampaignEvent(context.Background(), job.CampaignID, action, map[string]any{
		"pages":          r.sum.Pages,
		"pages_failed":   r.sum.PagesFailed,
		"texts_failed":   r.sum.TextsFailed,
		"links":          r.sum.LinksMade,
		"pictures":       r.sum.Pictures,
		"files_attached": r.sum.FilesAttached,
		"root_page":      r.rootID,
	})
}

func (r *runner) execute(ctx context.Context) error {
	arc, err := OpenArchive(r.up.path, r.up.fileName, r.s.d.Limits)
	if err != nil {
		return err
	}
	defer func() { _ = arc.Close() }()
	r.arc = arc

	if err := r.loadKinds(ctx); err != nil {
		return err
	}

	r.job.set(func(j *Job) { j.phase = "Creating the Imported folder" })
	name := rootBaseName
	if taken, err := r.s.d.Pages.NameTaken(ctx, r.campaignID, name); err != nil {
		return fmt.Errorf("checking for an earlier import: %w", err)
	} else if taken {
		// A second import is a second folder, told apart by when it ran.
		name = rootBaseName + " " + r.s.d.Now().Format("2006-01-02 15:04")
	}
	rootID, err := r.s.d.Pages.CreatePage(ctx, r.campaignID, r.userID, NewPage{Name: name, KindID: r.defaultKind.ID})
	if err != nil {
		return fmt.Errorf("creating the Imported folder: %w", err)
	}
	r.rootID, r.rootName = rootID, name
	r.job.set(func(j *Job) { j.rootID, j.rootName = rootID, name })
	r.writeRootNote(ctx, "This import has not finished. If this page still says so after a while, the server stopped partway through: delete this folder and import again.")

	if err := r.createPages(ctx); err != nil {
		return err
	}
	if err := r.writeTexts(ctx); err != nil {
		return err
	}
	r.finishRoot(ctx)
	r.job.set(func(j *Job) {
		j.state = StateDone
		j.done = j.total
		s := r.sum
		j.summary = &s
		j.finished = r.s.d.Now()
	})
	return nil
}

func (r *runner) loadKinds(ctx context.Context) error {
	kinds, err := r.s.d.Pages.Kinds(ctx, r.campaignID)
	if err != nil {
		return fmt.Errorf("reading the page types: %w", err)
	}
	r.kinds = map[string]PageKind{}
	found := false
	for _, k := range kinds {
		if !k.Enabled {
			continue
		}
		r.kinds[strings.ToLower(k.Slug)] = k
		if !found {
			r.defaultKind, found = k, true
		}
	}
	if note, ok := r.kinds["note"]; ok {
		r.defaultKind = note
		found = true
	}
	if !found {
		return errors.New("this campaign has no page type to create pages as")
	}
	return nil
}

// createPages makes every folder and note page, parents first, empty and GM
// only. Text comes in a second pass because a [[link]] can point at a page
// that is created later in the tree.
func (r *runner) createPages(ctx context.Context) error {
	r.job.set(func(j *Job) { j.phase = "Creating pages" })
	for i, p := range r.v.Pages {
		if err := ctx.Err(); err != nil {
			return err
		}
		parent := r.rootID
		if p.Parent >= 0 && r.pageIDs[p.Parent] != "" {
			parent = r.pageIDs[p.Parent]
		}
		kind, label := r.defaultKind, ""
		if p.Note >= 0 {
			n := r.v.Notes[p.Note]
			if k, ok := r.kinds[n.FM.Type]; ok && n.FM.Type != "" {
				kind = k
			}
			label = n.FM.Subcategory
		}
		id, err := r.s.d.Pages.CreatePage(ctx, r.campaignID, r.userID, NewPage{Name: p.Name, KindID: kind.ID, Label: label, ParentID: parent})
		if err != nil {
			slog.Warn("vault import: creating a page failed", slog.String("campaign_id", r.campaignID), slog.Any("error", err))
			r.sum.PagesFailed++
			r.sum.problem(p.Name, "the page could not be created: "+safeMessage(err))
			continue
		}
		r.pageIDs[i] = id
		r.created++
		r.sum.Pages++
	}
	if r.created == 0 {
		return errors.New("no page could be created")
	}
	return nil
}

func (r *runner) writeTexts(ctx context.Context) error {
	r.job.set(func(j *Job) { j.phase = "Writing pages" })
	for i, p := range r.v.Pages {
		if err := ctx.Err(); err != nil {
			return err
		}
		if p.Note >= 0 && r.pageIDs[i] != "" {
			r.writeNote(ctx, p.Note, i)
		}
		r.job.set(func(j *Job) { j.done = i + 1 })
	}
	return nil
}

func (r *runner) writeNote(ctx context.Context, noteIdx, pageIdx int) {
	n := r.v.Notes[noteIdx]
	pageID := r.pageIDs[pageIdx]
	raw, err := r.arc.Read(n.Entry, r.s.d.Limits.MaxNoteBytes)
	if err != nil {
		r.textFailed(n, "the note could not be read from the zip")
		return
	}
	_, body, _, _ := SplitFrontMatter(string(raw))
	body = Prepare(body)
	attached := map[int]bool{}
	text := Rewrite(body, ScanRefs(body), func(ref Ref) (string, bool) {
		return r.rewriteRef(ctx, noteIdx, pageID, ref, attached)
	})
	if strings.TrimSpace(text) == "" {
		return
	}
	htmlOut, editorJSON, err := r.s.renderMarkdown(text, r.campaignID)
	if err != nil {
		slog.Warn("vault import: converting a note failed", slog.String("campaign_id", r.campaignID), slog.Any("error", err))
		r.textFailed(n, "the text could not be converted")
		return
	}
	if err := r.s.d.Pages.SetBody(ctx, pageID, editorJSON, htmlOut); err != nil {
		slog.Warn("vault import: saving a page's text failed", slog.String("campaign_id", r.campaignID), slog.Any("error", err))
		r.textFailed(n, "the page was created but its text could not be saved: "+safeMessage(err))
	}
}

func (r *runner) textFailed(n *Note, why string) {
	r.sum.TextsFailed++
	r.sum.problem(n.Title, why)
}

func safeMessage(err error) string {
	if m := apperror.SafeMessage(err); m != "" {
		return m
	}
	return "an unexpected error"
}

// lastSegment is the part of a written name after its last "/", without a note
// extension.
func lastSegment(name string) string {
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	return strings.TrimSuffix(strings.TrimSuffix(name, ".md"), ".markdown")
}

// rewriteRef turns one link or picture in a note into the Markdown that
// renders to Chronicle's own link and picture markup, or into plain words when
// the target is not here. ok=false leaves the text as written.
func (r *runner) rewriteRef(ctx context.Context, noteIdx int, pageID string, ref Ref, attached map[int]bool) (string, bool) {
	if ref.Remote {
		if ref.Kind != RefImage {
			return "", false
		}
		return remotePicture(ref), true
	}
	t := r.v.Resolve(noteIdx, ref)
	label := ref.Text
	if label == "" || ref.Kind == RefImage {
		label = lastSegment(ref.Target)
	}
	switch t.Kind {
	case TargetSelf:
		if ref.Text != "" {
			return escapeMD(ref.Text), true
		}
		return escapeMD(ref.Fragment), true

	case TargetNone:
		if isImageRef(ref) {
			return "*picture not in the zip: " + escapeMD(lastSegment(ref.Target)) + "*", true
		}
		r.sum.LinksPlain++
		return r.plainLabel(ref), true

	case TargetNote:
		id := r.pageIDs[r.v.NotePage(t.Index)]
		if id == "" {
			r.sum.LinksPlain++
			return r.plainLabel(ref), true
		}
		text := escapeMD(label)
		if ref.Kind == RefLink && ref.Text != "" {
			text = ref.Text
		} else if ref.Fragment != "" && !strings.HasPrefix(ref.Fragment, "^") && ref.Text == "" {
			text += " > " + escapeMD(ref.Fragment)
		}
		if strings.TrimSpace(text) == "" {
			text = escapeMD(r.v.Notes[t.Index].Title)
		}
		r.sum.LinksMade++
		return "[" + text + "](" + pagelink.Href(r.campaignID, id) + ")", true

	case TargetFile:
		f := r.v.Files[t.Index]
		if f.Kind == KindPicture && (ref.Kind == RefEmbed || ref.Kind == RefImage) {
			alt := ref.Text
			if alt == "" {
				alt = strings.TrimSuffix(path.Base(f.Path), path.Ext(f.Path))
			}
			id := r.picture(ctx, t.Index)
			if id == "" {
				return "*picture not imported: " + escapeMD(path.Base(f.Path)) + "*", true
			}
			title := ""
			if w := widthFromHint(ref.Hint); w < 100 {
				title = fmt.Sprintf(` "w%d"`, w)
			}
			return "![" + escapeMD(alt) + "](/media/" + id + title + ")", true
		}
		r.attach(ctx, pageID, t.Index, attached)
		if ref.Kind == RefLink && ref.Text != "" {
			return ref.Text, true
		}
		return escapeMD(path.Base(f.Path)), true
	}
	return "", false
}

// plainLabel is a link whose target is not in the zip, as the words it showed.
func (r *runner) plainLabel(ref Ref) string {
	if ref.Kind == RefLink && ref.Text != "" {
		return ref.Text
	}
	if ref.Text != "" {
		return escapeMD(ref.Text)
	}
	return escapeMD(lastSegment(ref.Target))
}

// remotePicture keeps a picture hosted elsewhere as a link: the editor never
// stores another site's picture as a hotlink.
func remotePicture(ref Ref) string {
	alt := ref.Text
	dest := ref.Target
	lower := strings.ToLower(dest)
	safe := (strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://")) && !strings.ContainsAny(dest, "<>\n\r ")
	if alt == "" {
		alt = dest
	}
	if !safe {
		return escapeMD(ref.Text)
	}
	return "[" + escapeMD(alt) + "](<" + dest + ">)"
}

// picture stores a picture once per import and returns its media id, or ""
// when it could not be stored (the reason is in the summary).
func (r *runner) picture(ctx context.Context, fileIdx int) string {
	if res, ok := r.pictures[fileIdx]; ok {
		return res.id
	}
	f := r.v.Files[fileIdx]
	res := pictureResult{}
	data, err := r.arc.Read(f, r.s.d.Limits.MaxFileBytes)
	if err == nil {
		res.id, err = r.s.d.Pictures.StorePicture(ctx, r.campaignID, r.userID, path.Base(f.Path), data)
	}
	if err != nil {
		res.id, res.err = "", safeMessage(err)
		r.sum.PictureFails++
		r.sum.problem(f.Path, "the picture was not imported: "+res.err)
	} else {
		r.sum.Pictures++
	}
	r.pictures[fileIdx] = res
	return res.id
}

// attach puts a file in a page's Files section, once per page, GM only.
func (r *runner) attach(ctx context.Context, pageID string, fileIdx int, attached map[int]bool) {
	if attached[fileIdx] {
		return
	}
	attached[fileIdx] = true
	f := r.v.Files[fileIdx]
	skip := func(why string) {
		r.sum.FilesSkipped++
		r.sum.problem(f.Path, "the file was not attached: "+why)
	}
	if r.s.d.Files == nil || !r.s.d.Files.CanAttach(path.Base(f.Path)) {
		skip("pages don't accept this kind of file")
		return
	}
	data, err := r.arc.Read(f, r.s.d.Limits.MaxFileBytes)
	if err != nil {
		skip("it could not be read from the zip")
		return
	}
	if err := r.s.d.Files.AttachGMOnly(ctx, r.campaignID, pageID, r.userID, path.Base(f.Path), data); err != nil {
		skip(safeMessage(err))
		return
	}
	r.sum.FilesAttached++
}

// writeRootNote puts a short plain note on the Imported folder page.
func (r *runner) writeRootNote(ctx context.Context, text string) {
	htmlOut, editorJSON, err := r.s.renderMarkdown(escapeMD(text), r.campaignID)
	if err != nil {
		return
	}
	if err := r.s.d.Pages.SetBody(ctx, r.rootID, editorJSON, htmlOut); err != nil {
		slog.Warn("vault import: writing the Imported folder note failed", slog.Any("error", err))
	}
}

// finishRoot replaces the in-progress note with the summary, which stays on
// the folder as the permanent record of what the import did.
func (r *runner) finishRoot(ctx context.Context) {
	var b strings.Builder
	fmt.Fprintf(&b, "Imported from **%s** on %s.\n\n", escapeMD(r.up.fileName), r.s.d.Now().Format("2 January 2006"))
	b.WriteString("Every imported page starts GM only. Share them when you're ready.\n\n")
	fmt.Fprintf(&b, "- %d pages\n- %d page links\n- %d pictures\n- %d files attached\n", r.sum.Pages, r.sum.LinksMade, r.sum.Pictures, r.sum.FilesAttached)
	if len(r.sum.Problems) > 0 {
		b.WriteString("\nNot imported:\n\n")
		for _, p := range r.sum.Problems {
			fmt.Fprintf(&b, "- %s: %s\n", escapeMD(p.Where), escapeMD(p.What))
		}
		if r.sum.MoreProblems > 0 {
			fmt.Fprintf(&b, "- and %d more\n", r.sum.MoreProblems)
		}
	}
	htmlOut, editorJSON, err := r.s.renderMarkdown(b.String(), r.campaignID)
	if err != nil {
		return
	}
	if err := r.s.d.Pages.SetBody(ctx, r.rootID, editorJSON, htmlOut); err != nil {
		slog.Warn("vault import: writing the Imported folder summary failed", slog.Any("error", err))
	}
}

// fail ends a job that could not finish. If the Imported folder exists it is
// kept, renamed "(unfinished)" and given a note saying so: the pages already
// created are real GM-only pages the owner may keep or delete with the folder.
// Without a folder nothing was written at all.
func (r *runner) fail(cause error) {
	slog.Error("vault import: failed", slog.String("campaign_id", r.campaignID), slog.Any("error", cause))
	msg := "The import could not be completed."
	if IsArchiveError(cause) {
		msg = cause.Error()
	}
	partial := r.rootID != ""
	if partial {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		name := r.rootName + unfinishedSuffix
		if err := r.s.d.Pages.Rename(ctx, r.rootID, name); err == nil {
			r.rootName = name
		}
		r.writeRootNote(ctx, fmt.Sprintf("This import stopped before it finished. %d pages were created and are in this folder, GM only. Delete this folder to remove them, then import again.", r.sum.Pages))
		msg += fmt.Sprintf(" %d of %d pages were created before it stopped; they are in the folder “%s”. Delete that folder to start over.", r.sum.Pages, r.job.View().Total, r.rootName)
	} else {
		msg += " Nothing was imported."
	}
	r.job.set(func(j *Job) {
		j.state = StateFailed
		j.partial = partial
		j.errMsg = msg
		j.rootName = r.rootName
		s := r.sum
		j.summary = &s
		j.finished = r.s.d.Now()
	})
	r.s.audit(r.job, r, "campaign.vault_import.failed")
}
