package trackers

import (
	"context"
	"fmt"
	"strings"
)

// OwnedKind is a kind another plugin keeps in step with a record of its own:
// a Homebrew Workshop recipe's tracker or clock part. Owner is unique in the
// campaign and starts with the prefix the caller syncs under.
type OwnedKind struct {
	Owner      string
	Name       string
	Shape      Shape
	Visibility string
}

// MaxOwnerChars bounds an owner key; the column holds this many.
const MaxOwnerChars = 160

// validPrefix says whether prefix can name a set of owned kinds. It must end
// in ':' so one owner's prefix never matches another's longer key.
func validPrefix(prefix string) bool {
	return len(prefix) > 1 && len(prefix) <= MaxOwnerChars && strings.HasSuffix(prefix, ":")
}

// ownedPlan is what a sync would do: kinds to write, and kinds no longer
// wanted, which are deleted when unused and otherwise let go of (they stay,
// with their trackers and logs, as hand-made kinds).
type ownedPlan struct {
	create []Kind
	update []Kind
	drop   []Kind
	letGo  []Kind
}

// planOwned works out a sync and refuses one the kinds' own rules would: a
// bad shape, a name another kind has, a shape change under placed trackers,
// or more kinds than a campaign may hold.
func (s *service) planOwned(ctx context.Context, campaignID, prefix string, want []OwnedKind) (ownedPlan, error) {
	var p ownedPlan
	if prefix == "" {
		return p, fmt.Errorf("trackers: an owned kind needs a prefix")
	}
	all, err := s.repo.ListKinds(ctx, campaignID)
	if err != nil {
		return p, apperrorInternal(err)
	}
	mine := map[string]Kind{}
	for _, k := range all {
		if strings.HasPrefix(k.Owner, prefix) {
			mine[k.Owner] = k
		}
	}
	names := map[string]bool{}
	wanted := map[string]bool{}
	for _, w := range want {
		if !strings.HasPrefix(w.Owner, prefix) || len(w.Owner) > MaxOwnerChars || wanted[w.Owner] {
			return p, fmt.Errorf("trackers: bad owner %q under %q", w.Owner, prefix)
		}
		wanted[w.Owner] = true
		k, err := validateKind(KindInput{Name: w.Name, Shape: w.Shape.Shape, Min: w.Shape.Min, Max: w.Shape.Max,
			Steps: w.Shape.Steps, Bands: w.Shape.Bands, Visibility: w.Visibility})
		if err != nil {
			return p, err
		}
		if names[strings.ToLower(k.Name)] {
			return p, errInvalid("two trackers in this recipe are called " + k.Name + "; give each its own name")
		}
		names[strings.ToLower(k.Name)] = true
		k.CampaignID, k.Owner, k.Managed = campaignID, w.Owner, true
		if old, ok := mine[w.Owner]; ok {
			if old.InUse > 0 && old.Shape != k.Shape {
				return p, errInvalid(old.Name + " is on pages, so it stays a " + old.Shape + "; add a new part instead")
			}
			k.ID, k.Version, k.EveryTypeID, k.InUse = old.ID, old.Version, old.EveryTypeID, old.InUse
			p.update = append(p.update, k)
		} else {
			k.ID = s.newID()
			p.create = append(p.create, k)
		}
	}
	for owner, k := range mine {
		if wanted[owner] {
			continue
		}
		if k.InUse > 0 {
			k.Owner, k.Managed = "", false
			p.letGo = append(p.letGo, k)
		} else {
			p.drop = append(p.drop, k)
		}
	}
	// Every name after the sync must be unique, as the table requires: the
	// wanted kinds against every kind that stays, hand-made or let go.
	kept := 0
	for _, k := range all {
		if strings.HasPrefix(k.Owner, prefix) && (wanted[k.Owner] || k.InUse == 0) {
			continue
		}
		kept++
		if names[strings.ToLower(k.Name)] {
			return p, errInvalid("there is already a tracker kind called " + k.Name + "; rename the part")
		}
	}
	if kept+len(want) > MaxKindsPerCamp {
		return p, errInvalid(fmt.Sprintf("a campaign can have at most %d tracker kinds", MaxKindsPerCamp))
	}
	return p, nil
}

func (s *service) CheckOwned(ctx context.Context, campaignID, prefix string, want []OwnedKind) error {
	_, err := s.planOwned(ctx, campaignID, prefix, want)
	return err
}

func (s *service) SyncOwned(ctx context.Context, campaignID, prefix string, want []OwnedKind) error {
	p, err := s.planOwned(ctx, campaignID, prefix, want)
	if err != nil {
		return err
	}
	// Kinds leave first, so a name a removed part had is free for a new one.
	for _, k := range p.drop {
		if inUse, err := s.repo.DeleteKind(ctx, campaignID, k.ID); err != nil {
			return apperrorInternal(err)
		} else if inUse {
			// Placed since the plan was made: keep it as a hand-made kind.
			k.Owner, k.Managed = "", false
			p.letGo = append(p.letGo, k)
		}
	}
	for _, k := range p.letGo {
		if err := s.write(ctx, k, false); err != nil {
			return err
		}
	}
	for _, k := range p.update {
		if err := s.write(ctx, k, false); err != nil {
			return err
		}
	}
	for _, k := range p.create {
		if err := s.write(ctx, k, true); err != nil {
			return err
		}
	}
	return nil
}

func (s *service) write(ctx context.Context, k Kind, create bool) error {
	if create {
		k.Version = 1
		if err := s.repo.CreateKind(ctx, k); err != nil {
			return dupName(err, k.Name)
		}
		return nil
	}
	ok, err := s.repo.UpdateKind(ctx, k, k.Version)
	if err != nil {
		return dupName(err, k.Name)
	}
	if !ok {
		return errConflict()
	}
	return nil
}
