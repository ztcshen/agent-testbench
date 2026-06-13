// Package sqlite provides the local single-file SQL Store engine.
//
// SQLite shares the sqlstore CoreSchema and Store implementation with the
// PostgreSQL and MySQL engines; this package owns SQLite config, driver setup,
// and small dialect-specific fast paths.
package sqlite
