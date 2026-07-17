package sqlstore

import "fmt"

func coreProfileCatalogVersionSchemaSQL(d Dialect, types coreSchemaTypes) []string {
	return []string{
		fmt.Sprintf(`
create table if not exists profile_catalog_heads (
  profile_id %s primary key,
  revision bigint not null,
  catalog_sha256 %s not null,
  updated_at %s not null
);`, types.profileIDText, types.text, types.timeType),
		fmt.Sprintf(`
create table if not exists profile_catalog_versions (
  profile_id %s not null,
  revision bigint not null,
  catalog_sha256 %s not null,
  catalog_json %s not null,
  operation %s not null,
  summary_json %s not null,
  created_at %s not null,
  primary key (profile_id, revision)
);`, types.profileIDText, types.text, types.jsonType, types.keyText, types.jsonType, types.timeType),
		d.CreateIndexSQL(
			"idx_profile_catalog_versions_created",
			"profile_catalog_versions",
			[]string{"profile_id", "created_at", "revision"},
		),
	}
}

func profileCatalogVersionBackfillSQL(d Dialect) []string {
	headConflict := d.UpsertClause("profile_id", []string{"profile_id"})
	versionConflict := "on conflict(profile_id, revision) do nothing"
	if d.Name() == "mysql" {
		versionConflict = "on duplicate key update profile_id = values(profile_id)"
	}
	return []string{
		fmt.Sprintf(`
insert into profile_catalog_heads (profile_id, revision, catalog_sha256, updated_at)
select profile_id, 1, '', indexed_at
from profile_catalogs
where true
%s;`, headConflict),
		fmt.Sprintf(`
insert into profile_catalog_versions (
  profile_id, revision, catalog_sha256, catalog_json, operation, summary_json, created_at
)
select profile_id, 1, '', catalog_json, 'migration-v19', '{}', indexed_at
from profile_catalogs
where true
%s;`, versionConflict),
	}
}
