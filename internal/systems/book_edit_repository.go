package systems

import (
	"context"
	"database/sql"
	"errors"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// BookEditRepository is the data access contract for a campaign's edits to a
// system's book. All SQL lives in the implementation. Every method is scoped
// by campaign and system, so one campaign can never read or write another's
// rows by guessing an id.
type BookEditRepository interface {
	// Load returns everything the campaign has changed for the system.
	Load(ctx context.Context, campaignID, systemID string) (*BookEdits, error)

	// SaveCopy creates or replaces the campaign's copy of a package page.
	SaveCopy(ctx context.Context, p StoredBookPage) error
	// AddOwn appends a page the campaign adds and returns its row id.
	AddOwn(ctx context.Context, p StoredBookPage) (int64, error)
	// UpdateOwn replaces the content of a page the campaign added.
	UpdateOwn(ctx context.Context, campaignID, systemID, chapterID string, id int64, pageJSON, userID string) error
	// DeleteCopy drops the copy of a package page; false when there was none.
	DeleteCopy(ctx context.Context, campaignID, systemID, chapterID string, index int) (bool, error)
	// DeleteOwn deletes a page the campaign added; false when there was none.
	DeleteOwn(ctx context.Context, campaignID, systemID, chapterID string, id int64) (bool, error)
	// PromoteCopy turns a copy whose package page is gone into a page the
	// campaign owns, at the end of the chapter, and returns its new row id.
	PromoteCopy(ctx context.Context, campaignID, systemID, chapterID string, index int) (int64, error)
	// AddChapter stores a new house chapter together with its first page, so
	// a chapter is never left without one.
	AddChapter(ctx context.Context, ch HouseChapter, firstPage StoredBookPage) error
	UpdateChapter(ctx context.Context, ch HouseChapter) error
	// DeleteChapter removes a house chapter and its pages; false when absent.
	DeleteChapter(ctx context.Context, campaignID, systemID, chapterID string) (bool, error)
}

type bookEditRepository struct {
	db *sql.DB
}

// NewBookEditRepository creates the MariaDB-backed repository.
func NewBookEditRepository(db *sql.DB) BookEditRepository {
	return &bookEditRepository{db: db}
}

func internalErr(err error) error { return apperror.NewInternal(err) }

func (r *bookEditRepository) Load(ctx context.Context, campaignID, systemID string) (*BookEdits, error) {
	out := &BookEdits{}

	rows, err := r.db.QueryContext(ctx,
		`SELECT chapter_id, title, intro, director, sort_order, COALESCE(created_by, '')
		   FROM campaign_book_chapters
		  WHERE campaign_id = ? AND system_id = ?
		  ORDER BY sort_order, chapter_id`, campaignID, systemID)
	if err != nil {
		return nil, internalErr(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		ch := HouseChapter{CampaignID: campaignID, SystemID: systemID}
		if err := rows.Scan(&ch.ChapterID, &ch.Title, &ch.Intro, &ch.Director, &ch.SortOrder, &ch.CreatedBy); err != nil {
			return nil, internalErr(err)
		}
		out.Chapters = append(out.Chapters, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, internalErr(err)
	}

	prows, err := r.db.QueryContext(ctx,
		`SELECT `+pageColumns+`
		   FROM campaign_book_pages
		  WHERE campaign_id = ? AND system_id = ?
		  ORDER BY chapter_id, package_index, sort_order, id`, campaignID, systemID)
	if err != nil {
		return nil, internalErr(err)
	}
	defer func() { _ = prows.Close() }()
	for prows.Next() {
		p, err := scanPage(prows)
		if err != nil {
			return nil, internalErr(err)
		}
		out.Pages = append(out.Pages, *p)
	}
	if err := prows.Err(); err != nil {
		return nil, internalErr(err)
	}
	return out, nil
}

const pageColumns = `id, campaign_id, system_id, chapter_id, package_index, sort_order,
	page_json, COALESCE(base_hash, ''), COALESCE(updated_by, '')`

type rowScanner interface{ Scan(dest ...any) error }

func scanPage(s rowScanner) (*StoredBookPage, error) {
	var p StoredBookPage
	var idx sql.NullInt64
	if err := s.Scan(&p.ID, &p.CampaignID, &p.SystemID, &p.ChapterID, &idx, &p.SortOrder,
		&p.PageJSON, &p.BaseHash, &p.UpdatedBy); err != nil {
		return nil, err
	}
	if idx.Valid {
		n := int(idx.Int64)
		p.PackageIndex = &n
	}
	return &p, nil
}

func (r *bookEditRepository) SaveCopy(ctx context.Context, p StoredBookPage) error {
	if p.PackageIndex == nil {
		return apperror.NewInternal(errors.New("SaveCopy without a package index"))
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO campaign_book_pages
		   (campaign_id, system_id, chapter_id, package_index, sort_order, page_json, base_hash, updated_by)
		 VALUES (?, ?, ?, ?, 0, ?, NULLIF(?, ''), NULLIF(?, ''))
		 ON DUPLICATE KEY UPDATE page_json = VALUES(page_json), base_hash = VALUES(base_hash),
		                         updated_by = VALUES(updated_by)`,
		p.CampaignID, p.SystemID, p.ChapterID, *p.PackageIndex, p.PageJSON, p.BaseHash, p.UpdatedBy)
	if err != nil {
		return internalErr(err)
	}
	return nil
}

func (r *bookEditRepository) AddOwn(ctx context.Context, p StoredBookPage) (int64, error) {
	var next int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sort_order), 0) + 1 FROM campaign_book_pages
		  WHERE campaign_id = ? AND system_id = ? AND chapter_id = ? AND package_index IS NULL`,
		p.CampaignID, p.SystemID, p.ChapterID).Scan(&next); err != nil {
		return 0, internalErr(err)
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO campaign_book_pages
		   (campaign_id, system_id, chapter_id, package_index, sort_order, page_json, updated_by)
		 VALUES (?, ?, ?, NULL, ?, ?, NULLIF(?, ''))`,
		p.CampaignID, p.SystemID, p.ChapterID, next, p.PageJSON, p.UpdatedBy)
	if err != nil {
		return 0, internalErr(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, internalErr(err)
	}
	return id, nil
}

func (r *bookEditRepository) UpdateOwn(ctx context.Context, campaignID, systemID, chapterID string, id int64, pageJSON, userID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE campaign_book_pages SET page_json = ?, updated_by = NULLIF(?, '')
		  WHERE campaign_id = ? AND system_id = ? AND chapter_id = ? AND id = ? AND package_index IS NULL`,
		pageJSON, userID, campaignID, systemID, chapterID, id)
	if err != nil {
		return internalErr(err)
	}
	return nil
}

func (r *bookEditRepository) DeleteCopy(ctx context.Context, campaignID, systemID, chapterID string, index int) (bool, error) {
	return r.exec(ctx, `DELETE FROM campaign_book_pages
		WHERE campaign_id = ? AND system_id = ? AND chapter_id = ? AND package_index = ?`,
		campaignID, systemID, chapterID, index)
}

func (r *bookEditRepository) DeleteOwn(ctx context.Context, campaignID, systemID, chapterID string, id int64) (bool, error) {
	return r.exec(ctx, `DELETE FROM campaign_book_pages
		WHERE campaign_id = ? AND system_id = ? AND chapter_id = ? AND id = ? AND package_index IS NULL`,
		campaignID, systemID, chapterID, id)
}

// exec runs a write and reports whether it touched a row.
func (r *bookEditRepository) exec(ctx context.Context, query string, args ...any) (bool, error) {
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, internalErr(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, internalErr(err)
	}
	return n > 0, nil
}

func (r *bookEditRepository) PromoteCopy(ctx context.Context, campaignID, systemID, chapterID string, index int) (int64, error) {
	// The row is found by its slot before it leaves it, so a page added at the
	// same moment can't be mistaken for it.
	var id int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT id FROM campaign_book_pages
		  WHERE campaign_id = ? AND system_id = ? AND chapter_id = ? AND package_index = ?`,
		campaignID, systemID, chapterID, index).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, apperror.NewNotFound("page not found")
		}
		return 0, internalErr(err)
	}
	var next int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sort_order), 0) + 1 FROM campaign_book_pages
		  WHERE campaign_id = ? AND system_id = ? AND chapter_id = ? AND package_index IS NULL`,
		campaignID, systemID, chapterID).Scan(&next); err != nil {
		return 0, internalErr(err)
	}
	if _, err := r.db.ExecContext(ctx,
		`UPDATE campaign_book_pages SET package_index = NULL, sort_order = ?, base_hash = NULL
		  WHERE campaign_id = ? AND system_id = ? AND id = ?`,
		next, campaignID, systemID, id); err != nil {
		return 0, internalErr(err)
	}
	return id, nil
}

func (r *bookEditRepository) AddChapter(ctx context.Context, ch HouseChapter, firstPage StoredBookPage) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internalErr(err)
	}
	defer func() { _ = tx.Rollback() }()

	var next int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sort_order), 0) + 1 FROM campaign_book_chapters WHERE campaign_id = ? AND system_id = ?`,
		ch.CampaignID, ch.SystemID).Scan(&next); err != nil {
		return internalErr(err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO campaign_book_chapters
		   (campaign_id, system_id, chapter_id, title, intro, director, sort_order, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''))`,
		ch.CampaignID, ch.SystemID, ch.ChapterID, ch.Title, ch.Intro, ch.Director, next, ch.CreatedBy); err != nil {
		return internalErr(err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO campaign_book_pages
		   (campaign_id, system_id, chapter_id, package_index, sort_order, page_json, updated_by)
		 VALUES (?, ?, ?, NULL, 1, ?, NULLIF(?, ''))`,
		ch.CampaignID, ch.SystemID, ch.ChapterID, firstPage.PageJSON, firstPage.UpdatedBy); err != nil {
		return internalErr(err)
	}
	if err := tx.Commit(); err != nil {
		return internalErr(err)
	}
	return nil
}

func (r *bookEditRepository) UpdateChapter(ctx context.Context, ch HouseChapter) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE campaign_book_chapters SET title = ?, intro = ?, director = ?
		  WHERE campaign_id = ? AND system_id = ? AND chapter_id = ?`,
		ch.Title, ch.Intro, ch.Director, ch.CampaignID, ch.SystemID, ch.ChapterID)
	if err != nil {
		return internalErr(err)
	}
	return nil
}

func (r *bookEditRepository) DeleteChapter(ctx context.Context, campaignID, systemID, chapterID string) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, internalErr(err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`DELETE FROM campaign_book_chapters WHERE campaign_id = ? AND system_id = ? AND chapter_id = ?`,
		campaignID, systemID, chapterID)
	if err != nil {
		return false, internalErr(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, internalErr(err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM campaign_book_pages WHERE campaign_id = ? AND system_id = ? AND chapter_id = ?`,
		campaignID, systemID, chapterID); err != nil {
		return false, internalErr(err)
	}
	if err := tx.Commit(); err != nil {
		return false, internalErr(err)
	}
	return n > 0, nil
}
