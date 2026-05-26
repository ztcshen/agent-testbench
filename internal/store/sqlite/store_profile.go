package sqlite

import (
	"context"
	"fmt"
	"strings"

	"agent-testbench/internal/store"
)

func (s *Store) UpsertProfileIndex(ctx context.Context, p store.ProfileIndex) (store.ProfileIndex, error) {
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = utcNow()
	}
	if err := s.exec(ctx, fmt.Sprintf(`
insert into profile_indexes (profile_id, bundle_path, bundle_digest, summary_json, imported_at, updated_at)
values (%s, %s, %s, %s, %s, %s)
on conflict(profile_id) do update set
  bundle_path = excluded.bundle_path,
  bundle_digest = excluded.bundle_digest,
  summary_json = excluded.summary_json,
  imported_at = excluded.imported_at,
  updated_at = excluded.updated_at;`,
		sqlString(p.ProfileID), sqlString(p.BundlePath), sqlString(p.BundleDigest), sqlString(p.SummaryJSON),
		sqlString(encodeTime(p.ImportedAt)), sqlString(encodeTime(p.UpdatedAt)))); err != nil {
		return store.ProfileIndex{}, fmt.Errorf("upsert profile index %q: %w", p.ProfileID, err)
	}
	return p, nil
}

func (s *Store) GetProfileIndex(ctx context.Context, profileID string) (store.ProfileIndex, error) {
	var rows []profileIndexRow
	if err := s.query(ctx, fmt.Sprintf(`
select profile_id, bundle_path, bundle_digest, summary_json, imported_at, updated_at
from profile_indexes where profile_id = %s;`, sqlString(profileID)), &rows); err != nil {
		return store.ProfileIndex{}, err
	}
	if len(rows) == 0 {
		return store.ProfileIndex{}, store.ErrNotFound
	}
	return rows[0].toStore(), nil
}

func (s *Store) UpsertConfigVersion(ctx context.Context, v store.ConfigVersion) (store.ConfigVersion, error) {
	if v.CreatedAt.IsZero() {
		v.CreatedAt = utcNow()
	}
	if v.PublishedAt.IsZero() {
		v.PublishedAt = v.CreatedAt
	}
	active := 0
	if v.Active {
		active = 1
	}
	statements := []string{}
	if v.Active {
		statements = append(statements, "update config_versions set active = 0;")
	}
	statements = append(statements, fmt.Sprintf(`
insert into config_versions (id, profile_id, source_path, bundle_digest, summary_json, active, published_at, created_at)
values (%s, %s, %s, %s, %s, %d, %s, %s)
on conflict(id) do update set
  profile_id = excluded.profile_id,
  source_path = excluded.source_path,
  bundle_digest = excluded.bundle_digest,
  summary_json = excluded.summary_json,
  active = excluded.active,
  published_at = excluded.published_at;`,
		sqlString(v.ID), sqlString(v.ProfileID), sqlString(v.SourcePath), sqlString(v.BundleDigest), sqlString(v.SummaryJSON),
		active, sqlString(encodeTime(v.PublishedAt)), sqlString(encodeTime(v.CreatedAt))))
	if err := s.exec(ctx, strings.Join(statements, "\n")); err != nil {
		return store.ConfigVersion{}, fmt.Errorf("upsert config version %q: %w", v.ID, err)
	}
	return v, nil
}

func (s *Store) GetActiveConfigVersion(ctx context.Context) (store.ConfigVersion, error) {
	var rows []configVersionRow
	if err := s.query(ctx, `
select id, profile_id, source_path, bundle_digest, summary_json, active, published_at, created_at
from config_versions
where active = 1
order by published_at desc, id desc
limit 1;`, &rows); err != nil {
		return store.ConfigVersion{}, err
	}
	if len(rows) == 0 {
		return store.ConfigVersion{}, store.ErrNotFound
	}
	return rows[0].toStore(), nil
}

func (s *Store) UpsertReadModel(ctx context.Context, m store.ReadModel) (store.ReadModel, error) {
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = utcNow()
	}
	if m.GeneratedAt.IsZero() {
		m.GeneratedAt = m.UpdatedAt
	}
	if err := s.exec(ctx, fmt.Sprintf(`
insert into config_read_model (profile_id, model_key, config_version_id, payload_json, generated_at, updated_at)
values (%s, %s, %s, %s, %s, %s)
on conflict(profile_id, model_key) do update set
  config_version_id = excluded.config_version_id,
  payload_json = excluded.payload_json,
  generated_at = excluded.generated_at,
  updated_at = excluded.updated_at;`,
		sqlString(m.ProfileID), sqlString(m.Key), sqlString(m.ConfigVersionID), sqlString(m.PayloadJSON),
		sqlString(encodeTime(m.GeneratedAt)), sqlString(encodeTime(m.UpdatedAt)))); err != nil {
		return store.ReadModel{}, fmt.Errorf("upsert read model %q/%q: %w", m.ProfileID, m.Key, err)
	}
	return m, nil
}

func (s *Store) GetReadModel(ctx context.Context, profileID string, key string) (store.ReadModel, error) {
	var rows []readModelRow
	if err := s.query(ctx, fmt.Sprintf(`
select profile_id, model_key, config_version_id, payload_json, generated_at, updated_at
from config_read_model
where profile_id = %s and model_key = %s;`, sqlString(profileID), sqlString(key)), &rows); err != nil {
		return store.ReadModel{}, err
	}
	if len(rows) == 0 {
		return store.ReadModel{}, store.ErrNotFound
	}
	return rows[0].toStore(), nil
}

type profileIndexRow struct {
	ProfileID    string `json:"profile_id"`
	BundlePath   string `json:"bundle_path"`
	BundleDigest string `json:"bundle_digest"`
	SummaryJSON  string `json:"summary_json"`
	ImportedAt   string `json:"imported_at"`
	UpdatedAt    string `json:"updated_at"`
}

func (r profileIndexRow) toStore() store.ProfileIndex {
	return store.ProfileIndex{
		ProfileID:    r.ProfileID,
		BundlePath:   r.BundlePath,
		BundleDigest: r.BundleDigest,
		SummaryJSON:  r.SummaryJSON,
		ImportedAt:   decodeTime(r.ImportedAt),
		UpdatedAt:    decodeTime(r.UpdatedAt),
	}
}

type configVersionRow struct {
	ID           string `json:"id"`
	ProfileID    string `json:"profile_id"`
	SourcePath   string `json:"source_path"`
	BundleDigest string `json:"bundle_digest"`
	SummaryJSON  string `json:"summary_json"`
	Active       int    `json:"active"`
	PublishedAt  string `json:"published_at"`
	CreatedAt    string `json:"created_at"`
}

func (r configVersionRow) toStore() store.ConfigVersion {
	return store.ConfigVersion{
		ID:           r.ID,
		ProfileID:    r.ProfileID,
		SourcePath:   r.SourcePath,
		BundleDigest: r.BundleDigest,
		SummaryJSON:  r.SummaryJSON,
		Active:       r.Active != 0,
		PublishedAt:  decodeTime(r.PublishedAt),
		CreatedAt:    decodeTime(r.CreatedAt),
	}
}

type readModelRow struct {
	ProfileID       string `json:"profile_id"`
	Key             string `json:"model_key"`
	ConfigVersionID string `json:"config_version_id"`
	PayloadJSON     string `json:"payload_json"`
	GeneratedAt     string `json:"generated_at"`
	UpdatedAt       string `json:"updated_at"`
}

func (r readModelRow) toStore() store.ReadModel {
	return store.ReadModel{
		ProfileID:       r.ProfileID,
		Key:             r.Key,
		ConfigVersionID: r.ConfigVersionID,
		PayloadJSON:     r.PayloadJSON,
		GeneratedAt:     decodeTime(r.GeneratedAt),
		UpdatedAt:       decodeTime(r.UpdatedAt),
	}
}
