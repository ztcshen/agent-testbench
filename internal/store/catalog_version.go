package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrProfileCatalogRevisionConflict = errors.New("profile catalog revision conflict")

type ProfileCatalogMutation struct {
	Operation   string
	SummaryJSON string
}

type ProfileCatalogSnapshot struct {
	Catalog   ProfileCatalog
	Revision  int64
	SHA256    string
	UpdatedAt time.Time
}

type ProfileCatalogVersion struct {
	ProfileID   string
	Revision    int64
	SHA256      string
	Operation   string
	SummaryJSON string
	CreatedAt   time.Time
	Catalog     ProfileCatalog
}

type ProfileCatalogRevisionConflictError struct {
	ProfileID        string
	ExpectedRevision int64
	ActualRevision   int64
}

func (e *ProfileCatalogRevisionConflictError) Error() string {
	return fmt.Sprintf(
		"profile catalog %q revision conflict: expected %d, current %d",
		e.ProfileID,
		e.ExpectedRevision,
		e.ActualRevision,
	)
}

func (e *ProfileCatalogRevisionConflictError) Unwrap() error {
	return ErrProfileCatalogRevisionConflict
}

type VersionedProfileCatalogStore interface {
	GetProfileCatalogSnapshot(context.Context, string) (ProfileCatalogSnapshot, error)
	CompareAndSwapProfileCatalog(context.Context, int64, ProfileCatalog, ProfileCatalogMutation) (ProfileCatalogSnapshot, error)
	ListProfileCatalogVersions(context.Context, string, int) ([]ProfileCatalogVersion, error)
	GetProfileCatalogVersion(context.Context, string, int64) (ProfileCatalogVersion, error)
}
