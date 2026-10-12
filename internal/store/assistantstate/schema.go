package assistantstate

import "fmt"

type ColumnTypes struct{ Key, Profile, Text, JSON, Time string }

func SchemaSQL(t ColumnTypes, indexSQL string) []string {
	return []string{fmt.Sprintf(`create table if not exists assistant_sessions (
  id %s primary key,
  owner_id %s not null,
  map_id %s not null,
  profile_id %s not null,
  title %s not null,
  status %s not null,
  revision integer not null,
  state_json %s not null,
  created_at %s not null,
  updated_at %s not null
);`, t.Key, t.Key, t.Key, t.Profile, t.Text, t.Key, t.JSON, t.Time, t.Time), indexSQL}
}
