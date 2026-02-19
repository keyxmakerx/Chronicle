package campaigns

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// UserFinderAdapter wraps auth.UserRepository to satisfy the UserFinder
// interface. This adapter pattern avoids importing auth types throughout
// the campaigns package — only this file references the auth package.
type UserFinderAdapter struct {
	repo auth.UserRepository
}

// NewUserFinderAdapter creates a new adapter around the auth repository.
func NewUserFinderAdapter(repo auth.UserRepository) UserFinder {
	return &UserFinderAdapter{repo: repo}
}

// FindUserByEmail looks up a user by email and maps to MemberUser.
func (a *UserFinderAdapter) FindUserByEmail(ctx context.Context, email string) (*MemberUser, error) {
	user, err := a.repo.FindByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	return &MemberUser{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
	}, nil
}

// FindUserByID looks up a user by ID and maps to MemberUser.
func (a *UserFinderAdapter) FindUserByID(ctx context.Context, id string) (*MemberUser, error) {
	user, err := a.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &MemberUser{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
	}, nil
}
