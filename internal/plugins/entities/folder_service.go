package entities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// folderNameMax matches the sidebar_nodes.name column.
const folderNameMax = 255

// FolderService owns the rules for sidebar folders and for which folder a
// page sits in, so the sidebar's JSON endpoints and AI Import share one set
// of checks instead of each reaching into the repositories.
type FolderService interface {
	// ListFolders returns a category's folders in sidebar order.
	ListFolders(ctx context.Context, campaignID string, entityTypeID int) ([]SidebarNode, error)
	// ListAllFolders returns every folder in the campaign, grouped by category.
	ListAllFolders(ctx context.Context, campaignID string) ([]SidebarNode, error)
	// FindFolder matches a folder by name within one category, ignoring case.
	// It returns nil when there is none and a bad-request error when two
	// folders share the name, so a caller never has to guess which is meant.
	FindFolder(ctx context.Context, campaignID string, entityTypeID int, name string) (*SidebarNode, error)
	CreateFolder(ctx context.Context, campaignID string, in CreateSidebarNodeInput) (*SidebarNode, error)
	RenameFolder(ctx context.Context, campaignID, folderID, name string) error
	// MoveFolder puts a folder under another folder of the same category, or
	// at the top level when parentID is nil.
	MoveFolder(ctx context.Context, campaignID, folderID string, parentID *string) error
	// DeleteFolder removes the folder only: its sub-folders move up to its
	// parent and its pages return to the top level of the category.
	DeleteFolder(ctx context.Context, campaignID, folderID string) error
	// SetEntityFolder files a page in a folder, or takes it out when
	// folderID is nil. A page sits in a folder or under another page, never
	// both, so filing it clears its parent page.
	SetEntityFolder(ctx context.Context, campaignID, entityID string, folderID *string) error
}

type folderService struct {
	nodes    SidebarNodeRepository
	entities EntityRepository
	types    EntityTypeRepository
}

// NewFolderService builds the folder service over the repositories it checks
// against.
func NewFolderService(nodes SidebarNodeRepository, ents EntityRepository, types EntityTypeRepository) FolderService {
	return &folderService{nodes: nodes, entities: ents, types: types}
}

func (s *folderService) ListFolders(ctx context.Context, campaignID string, entityTypeID int) ([]SidebarNode, error) {
	nodes, err := s.nodes.ListByType(ctx, campaignID, entityTypeID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing folders: %w", err))
	}
	return nodes, nil
}

func (s *folderService) ListAllFolders(ctx context.Context, campaignID string) ([]SidebarNode, error) {
	types, err := s.types.ListByCampaign(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing categories: %w", err))
	}
	var all []SidebarNode
	for _, t := range types {
		nodes, err := s.ListFolders(ctx, campaignID, t.ID)
		if err != nil {
			return nil, err
		}
		all = append(all, nodes...)
	}
	return all, nil
}

func (s *folderService) FindFolder(ctx context.Context, campaignID string, entityTypeID int, name string) (*SidebarNode, error) {
	nodes, err := s.ListFolders(ctx, campaignID, entityTypeID)
	if err != nil {
		return nil, err
	}
	var hit *SidebarNode
	for i := range nodes {
		if !strings.EqualFold(strings.TrimSpace(nodes[i].Name), strings.TrimSpace(name)) {
			continue
		}
		if hit != nil {
			return nil, apperror.NewBadRequest(fmt.Sprintf("more than one folder is called %q in this category", strings.TrimSpace(name)))
		}
		hit = &nodes[i]
	}
	return hit, nil
}

// folder loads a folder and refuses one from another campaign as not found.
func (s *folderService) folder(ctx context.Context, campaignID, id string) (*SidebarNode, error) {
	n, err := s.nodes.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if n.CampaignID != campaignID {
		return nil, apperror.NewNotFound("sidebar node not found")
	}
	return n, nil
}

func cleanFolderName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if err := apperror.ValidateRequired("name", name); err != nil {
		return "", err
	}
	if err := apperror.ValidateStringLength("name", name, folderNameMax); err != nil {
		return "", err
	}
	return name, nil
}

func (s *folderService) CreateFolder(ctx context.Context, campaignID string, in CreateSidebarNodeInput) (*SidebarNode, error) {
	name, err := cleanFolderName(in.Name)
	if err != nil {
		return nil, err
	}
	et, err := s.types.FindByID(ctx, in.EntityTypeID)
	if err != nil || et.CampaignID != campaignID {
		return nil, apperror.NewBadRequest("category not found")
	}
	node := &SidebarNode{
		ID:           generateNodeID(),
		CampaignID:   campaignID,
		EntityTypeID: in.EntityTypeID,
		Name:         name,
		NodeType:     "folder",
		CreatedAt:    time.Now().UTC(),
	}
	if in.ParentID != "" {
		parent, err := s.folder(ctx, campaignID, in.ParentID)
		if err != nil || parent.EntityTypeID != in.EntityTypeID {
			return nil, apperror.NewBadRequest("the parent folder was not found in this category")
		}
		node.ParentID = &in.ParentID
	}
	if err := s.nodes.Create(ctx, node); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating sidebar node: %w", err))
	}
	return node, nil
}

func (s *folderService) RenameFolder(ctx context.Context, campaignID, folderID, name string) error {
	node, err := s.folder(ctx, campaignID, folderID)
	if err != nil {
		return err
	}
	if node.Name, err = cleanFolderName(name); err != nil {
		return err
	}
	if err := s.nodes.Update(ctx, node); err != nil {
		return apperror.NewInternal(fmt.Errorf("renaming sidebar node: %w", err))
	}
	return nil
}

func (s *folderService) MoveFolder(ctx context.Context, campaignID, folderID string, parentID *string) error {
	node, err := s.folder(ctx, campaignID, folderID)
	if err != nil {
		return err
	}
	if parentID != nil {
		// Walk up from the new parent: reaching the folder itself would make
		// a loop that hides both from the sidebar.
		// Stored data can already hold a loop that does not include this
		// folder (the sidebar reorder API can make one), so a visited set
		// stops the walk instead of spinning forever.
		seen := map[string]bool{}
		for cur := *parentID; cur != ""; {
			if seen[cur] {
				return apperror.NewBadRequest("the parent folder sits in a loop of folders; fix the folder tree first")
			}
			seen[cur] = true
			if cur == folderID {
				return apperror.NewBadRequest("a folder cannot be moved into itself or one of its own sub-folders")
			}
			p, err := s.folder(ctx, campaignID, cur)
			if err != nil || p.EntityTypeID != node.EntityTypeID {
				return apperror.NewBadRequest("the parent folder was not found in this category")
			}
			cur = ""
			if p.ParentID != nil {
				cur = *p.ParentID
			}
		}
	}
	if err := s.nodes.UpdateParent(ctx, folderID, campaignID, parentID); err != nil {
		return apperror.NewInternal(fmt.Errorf("updating node parent: %w", err))
	}
	return nil
}

func (s *folderService) DeleteFolder(ctx context.Context, campaignID, folderID string) error {
	if _, err := s.folder(ctx, campaignID, folderID); err != nil {
		return err
	}
	if err := s.nodes.Delete(ctx, folderID); err != nil {
		return apperror.NewInternal(fmt.Errorf("deleting sidebar node: %w", err))
	}
	return nil
}

func (s *folderService) SetEntityFolder(ctx context.Context, campaignID, entityID string, folderID *string) error {
	ent, err := s.entities.FindByID(ctx, entityID)
	if err != nil || ent.CampaignID != campaignID {
		return apperror.NewNotFound("entity not found")
	}
	if folderID != nil {
		node, err := s.folder(ctx, campaignID, *folderID)
		if err != nil {
			return apperror.NewBadRequest("folder not found")
		}
		if node.EntityTypeID != ent.EntityTypeID {
			// The sidebar rolls a sub-category up into its parent, so a folder
			// of the parent category also holds this page.
			et, terr := s.types.FindByID(ctx, ent.EntityTypeID)
			if terr != nil || et.ParentTypeID == nil || *et.ParentTypeID != node.EntityTypeID {
				return apperror.NewBadRequest("that folder belongs to a different category")
			}
		}
		if err := s.entities.UpdateParent(ctx, entityID, campaignID, nil); err != nil {
			return apperror.NewInternal(err)
		}
	}
	if err := s.entities.UpdateParentNode(ctx, entityID, campaignID, folderID); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}
