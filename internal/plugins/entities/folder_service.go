package entities

import (
	"context"
	"fmt"
	"strconv"
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
	// ChildOrders returns how each sidebar container in the campaign orders
	// its rows, keyed "category:<type id>", "folder:<id>" or "page:<id>". A
	// container that is missing sorts A to Z.
	ChildOrders(ctx context.Context, campaignID string) (map[string]string, error)
	// SetChildOrder sets how one container (a category's top level, a
	// folder, or a page with pages under it) orders its rows.
	SetChildOrder(ctx context.Context, campaignID, kind, id, mode string) error
	// MoveNextTo puts a page or folder just before or after one of its
	// siblings, for a container ordered by hand. The whole sibling set is
	// renumbered, so rows the mover cannot see keep their places among
	// themselves.
	MoveNextTo(ctx context.Context, campaignID, kind, id, siblingID string, after bool) error
}

type folderService struct {
	nodes    SidebarNodeRepository
	entities EntityRepository
	types    EntityTypeRepository
	orders   ChildOrderRepository
}

// NewFolderService builds the folder service over the repositories it checks
// against.
func NewFolderService(nodes SidebarNodeRepository, ents EntityRepository, types EntityTypeRepository, orders ChildOrderRepository) FolderService {
	return &folderService{nodes: nodes, entities: ents, types: types, orders: orders}
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

func (s *folderService) ChildOrders(ctx context.Context, campaignID string) (map[string]string, error) {
	orders, err := s.orders.ListByCampaign(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return orders, nil
}

func (s *folderService) SetChildOrder(ctx context.Context, campaignID, kind, id, mode string) error {
	if !validChildOrder(mode) {
		return apperror.NewBadRequest("order must be az, hand or new")
	}
	switch kind {
	case OrderContainerCategory:
		typeID, err := strconv.Atoi(id)
		if err != nil {
			return apperror.NewNotFound("category not found")
		}
		et, err := s.types.FindByID(ctx, typeID)
		if err != nil || et.CampaignID != campaignID {
			return apperror.NewNotFound("category not found")
		}
	case OrderContainerFolder:
		if _, err := s.folder(ctx, campaignID, id); err != nil {
			return err
		}
	case OrderContainerPage:
		ent, err := s.entities.FindByID(ctx, id)
		if err != nil || ent.CampaignID != campaignID {
			return apperror.NewNotFound("entity not found")
		}
	default:
		return apperror.NewBadRequest("kind must be category, folder or page")
	}
	if err := s.orders.Set(ctx, campaignID, kind, id, mode); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func (s *folderService) MoveNextTo(ctx context.Context, campaignID, kind, id, siblingID string, after bool) error {
	if id == siblingID {
		return apperror.NewBadRequest("a row cannot move next to itself")
	}
	switch kind {
	case OrderContainerPage:
		return s.movePageNextTo(ctx, campaignID, id, siblingID, after)
	case OrderContainerFolder:
		return s.moveFolderNextTo(ctx, campaignID, id, siblingID, after)
	}
	return apperror.NewBadRequest("kind must be page or folder")
}

func (s *folderService) movePageNextTo(ctx context.Context, campaignID, id, siblingID string, after bool) error {
	ent, err := s.entities.FindByID(ctx, id)
	if err != nil || ent.CampaignID != campaignID {
		return apperror.NewNotFound("entity not found")
	}
	sib, err := s.entities.FindByID(ctx, siblingID)
	if err != nil || sib.CampaignID != campaignID {
		return apperror.NewNotFound("entity not found")
	}
	// A sub-category's pages keep their own numbering, so a page of one
	// cannot be placed among the other's.
	if !sameParent(ent.ParentID, sib.ParentID) || !sameParent(ent.ParentNodeID, sib.ParentNodeID) || ent.EntityTypeID != sib.EntityTypeID {
		return apperror.NewBadRequest("those two pages are not in the same list")
	}
	ids, err := s.entities.ListSiblingIDsOrdered(ctx, campaignID, ent.EntityTypeID, ent.ParentID, ent.ParentNodeID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("loading siblings: %w", err))
	}
	ordered, ok := placeNextTo(ids, id, siblingID, after)
	if !ok {
		return apperror.NewBadRequest("those two pages are not in the same list")
	}
	if err := s.entities.ResequenceSiblings(ctx, campaignID, ordered); err != nil {
		return apperror.NewInternal(fmt.Errorf("re-sequencing siblings: %w", err))
	}
	return nil
}

func (s *folderService) moveFolderNextTo(ctx context.Context, campaignID, id, siblingID string, after bool) error {
	node, err := s.folder(ctx, campaignID, id)
	if err != nil {
		return err
	}
	sib, err := s.folder(ctx, campaignID, siblingID)
	if err != nil {
		return err
	}
	if node.EntityTypeID != sib.EntityTypeID || !sameParent(node.ParentID, sib.ParentID) {
		return apperror.NewBadRequest("those two folders are not in the same list")
	}
	all, err := s.nodes.ListByType(ctx, campaignID, node.EntityTypeID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("loading sibling folders: %w", err))
	}
	ids := make([]string, 0, len(all))
	for _, n := range all {
		if sameParent(n.ParentID, node.ParentID) {
			ids = append(ids, n.ID)
		}
	}
	ordered, ok := placeNextTo(ids, id, siblingID, after)
	if !ok {
		return apperror.NewBadRequest("those two folders are not in the same list")
	}
	if err := s.nodes.ResequenceNodes(ctx, campaignID, ordered); err != nil {
		return apperror.NewInternal(fmt.Errorf("re-sequencing sibling folders: %w", err))
	}
	return nil
}
