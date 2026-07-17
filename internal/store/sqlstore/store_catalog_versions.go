package sqlstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

func (s *Store) GetProfileCatalogSnapshot(ctx context.Context, profileID string) (store.ProfileCatalogSnapshot, error) {
	query := fmt.Sprintf(`
select p.catalog_json, h.revision, h.catalog_sha256, h.updated_at
from profile_catalog_heads h
join profile_catalogs p on p.profile_id = h.profile_id
where h.profile_id = %s;`, s.dialect.BindVar(1))
	var payload string
	var snapshot store.ProfileCatalogSnapshot
	var updatedAt any
	if err := s.db.QueryRowContext(ctx, query, profileID).Scan(&payload, &snapshot.Revision, &snapshot.SHA256, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.ProfileCatalogSnapshot{}, store.ErrNotFound
		}
		return store.ProfileCatalogSnapshot{}, fmt.Errorf("get profile catalog snapshot %q: %w", profileID, err)
	}
	if err := json.Unmarshal([]byte(payload), &snapshot.Catalog); err != nil {
		return store.ProfileCatalogSnapshot{}, fmt.Errorf("decode profile catalog snapshot %q: %w", profileID, err)
	}
	if snapshot.SHA256 == "" {
		var err error
		snapshot.SHA256, err = profileCatalogDigest(snapshot.Catalog)
		if err != nil {
			return store.ProfileCatalogSnapshot{}, err
		}
	}
	snapshot.UpdatedAt = decodeDBTime(updatedAt)
	return snapshot, nil
}

func (s *Store) CompareAndSwapProfileCatalog(
	ctx context.Context,
	expectedRevision int64,
	catalog store.ProfileCatalog,
	mutation store.ProfileCatalogMutation,
) (_ store.ProfileCatalogSnapshot, err error) {
	change, err := prepareProfileCatalogChange(expectedRevision, catalog, mutation)
	if err != nil {
		return store.ProfileCatalogSnapshot{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.ProfileCatalogSnapshot{}, fmt.Errorf("begin profile catalog transaction: %w", err)
	}
	defer rollbackTxOnError(tx, &err)

	revision, unchanged, err := s.claimProfileCatalogChange(ctx, tx, change)
	if err != nil {
		return store.ProfileCatalogSnapshot{}, err
	}
	if err := s.replaceProfileCatalogRow(ctx, tx, change.catalog, change.payload); err != nil {
		return store.ProfileCatalogSnapshot{}, err
	}
	if unchanged {
		if err := tx.Commit(); err != nil {
			return store.ProfileCatalogSnapshot{}, fmt.Errorf("commit unchanged profile catalog %q: %w", change.catalog.ProfileID, err)
		}
		return change.snapshot(revision), nil
	}
	if err := s.recordProfileCatalogVersion(ctx, tx, change, revision); err != nil {
		return store.ProfileCatalogSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return store.ProfileCatalogSnapshot{}, fmt.Errorf("commit profile catalog %q revision %d: %w", change.catalog.ProfileID, revision, err)
	}
	return change.snapshot(revision), nil
}

type profileCatalogChange struct {
	catalog          store.ProfileCatalog
	expectedRevision int64
	payload          string
	digest           string
	operation        string
	summaryJSON      string
	now              time.Time
}

func prepareProfileCatalogChange(expectedRevision int64, catalog store.ProfileCatalog, mutation store.ProfileCatalogMutation) (profileCatalogChange, error) {
	if strings.TrimSpace(catalog.ProfileID) == "" {
		return profileCatalogChange{}, errors.New("compare and swap profile catalog: profile id is required")
	}
	if expectedRevision < 0 {
		return profileCatalogChange{}, errors.New("compare and swap profile catalog: expected revision cannot be negative")
	}
	payload, digest, err := prepareProfileCatalogPayload(&catalog)
	if err != nil {
		return profileCatalogChange{}, err
	}
	operation := strings.TrimSpace(mutation.Operation)
	if operation == "" {
		operation = "update"
	}
	summaryJSON := strings.TrimSpace(mutation.SummaryJSON)
	if summaryJSON == "" {
		summaryJSON = "{}"
	}
	if !json.Valid([]byte(summaryJSON)) {
		return profileCatalogChange{}, errors.New("compare and swap profile catalog: mutation summary must be valid JSON")
	}
	return profileCatalogChange{
		catalog:          catalog,
		expectedRevision: expectedRevision,
		payload:          payload,
		digest:           digest,
		operation:        operation,
		summaryJSON:      normalizeJSONText(summaryJSON),
		now:              utcNow(),
	}, nil
}

func (change profileCatalogChange) snapshot(revision int64) store.ProfileCatalogSnapshot {
	return store.ProfileCatalogSnapshot{
		Catalog: change.catalog, Revision: revision, SHA256: change.digest, UpdatedAt: change.now,
	}
}

func (s *Store) claimProfileCatalogChange(ctx context.Context, tx *sql.Tx, change profileCatalogChange) (int64, bool, error) {
	if change.expectedRevision == 0 {
		if err := s.createProfileCatalogHead(ctx, tx, change); err != nil {
			return 0, false, err
		}
		return 1, false, nil
	}
	if err := s.claimProfileCatalogHead(ctx, tx, change); err != nil {
		return 0, false, err
	}
	currentDigest, err := s.claimedProfileCatalogDigest(ctx, tx, change)
	if err != nil {
		return 0, false, err
	}
	if currentDigest == change.digest {
		return change.expectedRevision, true, nil
	}
	revision := change.expectedRevision + 1
	if err := s.advanceProfileCatalogHead(ctx, tx, change, revision); err != nil {
		return 0, false, err
	}
	return revision, false, nil
}

func (s *Store) createProfileCatalogHead(ctx context.Context, tx *sql.Tx, change profileCatalogChange) error {
	query := fmt.Sprintf(`
insert into profile_catalog_heads (profile_id, revision, catalog_sha256, updated_at)
values (%s);`, s.bindVars(4))
	if _, err := tx.ExecContext(ctx, query, change.catalog.ProfileID, 1, change.digest, dbTimeArg(s.dialect, change.now)); err != nil {
		if rollbackErr := rollbackTxBeforeConflict(tx, "profile catalog create conflict"); rollbackErr != nil {
			return errors.Join(fmt.Errorf("create profile catalog head %q: %w", change.catalog.ProfileID, err), rollbackErr)
		}
		return s.profileCatalogConflictOrError(ctx, change.catalog.ProfileID, change.expectedRevision, err)
	}
	return nil
}

func (s *Store) claimProfileCatalogHead(ctx context.Context, tx *sql.Tx, change profileCatalogChange) error {
	query := fmt.Sprintf(`
update profile_catalog_heads
set updated_at = %s
where profile_id = %s and revision = %s;`, s.dialect.BindVar(1), s.dialect.BindVar(2), s.dialect.BindVar(3))
	result, err := tx.ExecContext(ctx, query, dbTimeArg(s.dialect, change.now), change.catalog.ProfileID, change.expectedRevision)
	if err != nil {
		return fmt.Errorf("claim profile catalog %q revision %d: %w", change.catalog.ProfileID, change.expectedRevision, err)
	}
	matched, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect profile catalog %q claim: %w", change.catalog.ProfileID, err)
	}
	if matched == 1 {
		return nil
	}
	if err := rollbackTxBeforeConflict(tx, "profile catalog claim conflict"); err != nil {
		return err
	}
	return s.profileCatalogConflict(ctx, change.catalog.ProfileID, change.expectedRevision)
}

func (s *Store) claimedProfileCatalogDigest(ctx context.Context, tx *sql.Tx, change profileCatalogChange) (string, error) {
	var digest string
	query := fmt.Sprintf(`select catalog_sha256 from profile_catalog_heads where profile_id = %s;`, s.dialect.BindVar(1))
	if err := tx.QueryRowContext(ctx, query, change.catalog.ProfileID).Scan(&digest); err != nil {
		return "", fmt.Errorf("read claimed profile catalog %q: %w", change.catalog.ProfileID, err)
	}
	if digest != "" {
		return digest, nil
	}
	digest, err := s.migratedProfileCatalogDigest(ctx, tx, change.catalog.ProfileID)
	if err != nil {
		return "", err
	}
	query = fmt.Sprintf(`
update profile_catalog_heads
set catalog_sha256 = %s
where profile_id = %s and revision = %s;`, s.dialect.BindVar(1), s.dialect.BindVar(2), s.dialect.BindVar(3))
	if _, err := tx.ExecContext(ctx, query, digest, change.catalog.ProfileID, change.expectedRevision); err != nil {
		return "", fmt.Errorf("repair migrated profile catalog %q digest: %w", change.catalog.ProfileID, err)
	}
	return digest, nil
}

func (s *Store) migratedProfileCatalogDigest(ctx context.Context, tx *sql.Tx, profileID string) (string, error) {
	var payload string
	query := fmt.Sprintf(`select catalog_json from profile_catalogs where profile_id = %s;`, s.dialect.BindVar(1))
	if err := tx.QueryRowContext(ctx, query, profileID).Scan(&payload); err != nil {
		return "", fmt.Errorf("read migrated profile catalog %q: %w", profileID, err)
	}
	var catalog store.ProfileCatalog
	if err := json.Unmarshal([]byte(payload), &catalog); err != nil {
		return "", fmt.Errorf("decode migrated profile catalog %q: %w", profileID, err)
	}
	return profileCatalogDigest(catalog)
}

func (s *Store) advanceProfileCatalogHead(ctx context.Context, tx *sql.Tx, change profileCatalogChange, revision int64) error {
	query := fmt.Sprintf(`
update profile_catalog_heads
set revision = %s, catalog_sha256 = %s, updated_at = %s
where profile_id = %s and revision = %s;`, s.dialect.BindVar(1), s.dialect.BindVar(2), s.dialect.BindVar(3), s.dialect.BindVar(4), s.dialect.BindVar(5))
	if _, err := tx.ExecContext(ctx, query, revision, change.digest, dbTimeArg(s.dialect, change.now), change.catalog.ProfileID, change.expectedRevision); err != nil {
		return fmt.Errorf("advance profile catalog %q revision: %w", change.catalog.ProfileID, err)
	}
	return nil
}

func (s *Store) recordProfileCatalogVersion(ctx context.Context, tx *sql.Tx, change profileCatalogChange, revision int64) error {
	query := fmt.Sprintf(`
insert into profile_catalog_versions (
  profile_id, revision, catalog_sha256, catalog_json, operation, summary_json, created_at
)
values (%s);`, s.bindVars(7))
	if _, err := tx.ExecContext(
		ctx,
		query,
		change.catalog.ProfileID,
		revision,
		change.digest,
		change.payload,
		change.operation,
		change.summaryJSON,
		dbTimeArg(s.dialect, change.now),
	); err != nil {
		return fmt.Errorf("record profile catalog %q revision %d: %w", change.catalog.ProfileID, revision, err)
	}
	return nil
}

func (s *Store) ListProfileCatalogVersions(ctx context.Context, profileID string, limit int) (versions []store.ProfileCatalogVersion, err error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	query := fmt.Sprintf(`
select profile_id, revision, catalog_sha256, catalog_json, operation, summary_json, created_at
from profile_catalog_versions
where profile_id = %s
order by revision desc
limit %s;`, s.dialect.BindVar(1), s.dialect.BindVar(2))
	rows, err := s.db.QueryContext(ctx, query, profileID, limit)
	if err != nil {
		return nil, fmt.Errorf("list profile catalog versions %q: %w", profileID, err)
	}
	defer closeRows(rows, &err)
	for rows.Next() {
		version, err := scanProfileCatalogVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return versions, nil
}

func (s *Store) GetProfileCatalogVersion(ctx context.Context, profileID string, revision int64) (store.ProfileCatalogVersion, error) {
	query := fmt.Sprintf(`
select profile_id, revision, catalog_sha256, catalog_json, operation, summary_json, created_at
from profile_catalog_versions
where profile_id = %s and revision = %s;`, s.dialect.BindVar(1), s.dialect.BindVar(2))
	version, err := scanProfileCatalogVersion(s.db.QueryRowContext(ctx, query, profileID, revision))
	if errors.Is(err, sql.ErrNoRows) {
		return store.ProfileCatalogVersion{}, store.ErrNotFound
	}
	if err != nil {
		return store.ProfileCatalogVersion{}, fmt.Errorf("get profile catalog %q revision %d: %w", profileID, revision, err)
	}
	return version, nil
}

func (s *Store) replaceProfileCatalogRow(ctx context.Context, exec sqlExecer, catalog store.ProfileCatalog, payload string) error {
	counts := catalogCounts(catalog)
	query := fmt.Sprintf(`
insert into profile_catalogs (
  profile_id, indexed_at, catalog_json, services, workflows, interface_nodes, api_cases,
  request_templates, workflow_bindings, case_dependencies, fixtures, templates, template_configs
)
values (%s)
%s;`, s.bindVars(13), s.dialect.UpsertClause("profile_id", []string{
		"indexed_at", "catalog_json", "services", "workflows", "interface_nodes", "api_cases",
		"request_templates", "workflow_bindings", "case_dependencies", "fixtures", "templates", "template_configs",
	}))
	if _, err := exec.ExecContext(
		ctx,
		query,
		catalog.ProfileID,
		dbTimeArg(s.dialect, catalog.IndexedAt),
		payload,
		counts.Services,
		counts.Workflows,
		counts.InterfaceNodes,
		counts.APICases,
		counts.RequestTemplates,
		counts.WorkflowBindings,
		counts.CaseDependencies,
		counts.Fixtures,
		counts.Templates,
		counts.TemplateConfigs,
	); err != nil {
		return fmt.Errorf("replace profile catalog %q: %w", catalog.ProfileID, err)
	}
	return nil
}

func (s *Store) profileCatalogConflictOrError(ctx context.Context, profileID string, expectedRevision int64, cause error) error {
	actual, err := s.profileCatalogHeadRevision(ctx, profileID)
	if err == nil {
		return &store.ProfileCatalogRevisionConflictError{
			ProfileID:        profileID,
			ExpectedRevision: expectedRevision,
			ActualRevision:   actual,
		}
	}
	return fmt.Errorf("create profile catalog %q head: %w", profileID, cause)
}

func (s *Store) profileCatalogConflict(ctx context.Context, profileID string, expectedRevision int64) error {
	actual, err := s.profileCatalogHeadRevision(ctx, profileID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("read profile catalog %q revision after conflict: %w", profileID, err)
	}
	return &store.ProfileCatalogRevisionConflictError{
		ProfileID:        profileID,
		ExpectedRevision: expectedRevision,
		ActualRevision:   actual,
	}
}

func (s *Store) profileCatalogHeadRevision(ctx context.Context, profileID string) (int64, error) {
	query := fmt.Sprintf(`select revision from profile_catalog_heads where profile_id = %s;`, s.dialect.BindVar(1))
	var revision int64
	if err := s.db.QueryRowContext(ctx, query, profileID).Scan(&revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, store.ErrNotFound
		}
		return 0, err
	}
	return revision, nil
}

func prepareProfileCatalogPayload(catalog *store.ProfileCatalog) (string, string, error) {
	if catalog.IndexedAt.IsZero() {
		catalog.IndexedAt = utcNow()
	}
	payload, err := json.Marshal(catalog)
	if err != nil {
		return "", "", fmt.Errorf("encode profile catalog %q: %w", catalog.ProfileID, err)
	}
	digest, err := profileCatalogDigest(*catalog)
	if err != nil {
		return "", "", err
	}
	return string(payload), digest, nil
}

func profileCatalogDigest(catalog store.ProfileCatalog) (string, error) {
	catalog.IndexedAt = time.Time{}
	payload, err := json.Marshal(catalog)
	if err != nil {
		return "", fmt.Errorf("encode profile catalog %q for digest: %w", catalog.ProfileID, err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func scanProfileCatalogVersion(row scanner) (store.ProfileCatalogVersion, error) {
	var version store.ProfileCatalogVersion
	var payload string
	var createdAt any
	if err := row.Scan(
		&version.ProfileID,
		&version.Revision,
		&version.SHA256,
		&payload,
		&version.Operation,
		&version.SummaryJSON,
		&createdAt,
	); err != nil {
		return store.ProfileCatalogVersion{}, err
	}
	if err := json.Unmarshal([]byte(payload), &version.Catalog); err != nil {
		return store.ProfileCatalogVersion{}, fmt.Errorf("decode profile catalog %q revision %d: %w", version.ProfileID, version.Revision, err)
	}
	if version.SHA256 == "" {
		var err error
		version.SHA256, err = profileCatalogDigest(version.Catalog)
		if err != nil {
			return store.ProfileCatalogVersion{}, err
		}
	}
	version.CreatedAt = decodeDBTime(createdAt)
	return version, nil
}
