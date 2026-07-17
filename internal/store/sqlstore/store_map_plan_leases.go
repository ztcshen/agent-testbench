package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

func (s *Store) ClaimTestMapPlanLease(ctx context.Context, requested store.TestMapPlanLease, now time.Time, allowUnleasedRunning bool) (_ store.TestMapPlanLease, err error) {
	requested, now, err = prepareTestMapPlanLeaseClaim(requested, now)
	if err != nil {
		return store.TestMapPlanLease{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.TestMapPlanLease{}, fmt.Errorf("begin test map plan lease claim: %w", err)
	}
	defer rollbackTxOnError(tx, &err)

	status, err := s.testMapPlanStatus(ctx, tx, requested.PlanID)
	if err != nil {
		return store.TestMapPlanLease{}, err
	}
	current, err := s.getTestMapPlanLease(ctx, tx, requested.PlanID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		if status == store.StatusRunning && !allowUnleasedRunning {
			return store.TestMapPlanLease{}, &store.TestMapPlanLeaseConflictError{
				PlanID: requested.PlanID,
				Reason: "plan is already running without a lease; verify side effects and use --resume for explicit recovery",
			}
		}
		if insertErr := s.insertTestMapPlanLease(ctx, tx, requested, now); insertErr != nil {
			if rollbackErr := rollbackTxBeforeConflict(tx, "test map plan lease insert conflict"); rollbackErr != nil {
				return store.TestMapPlanLease{}, errors.Join(insertErr, rollbackErr)
			}
			return store.TestMapPlanLease{}, s.testMapPlanLeaseConflict(ctx, requested.PlanID, "another executor claimed the plan")
		}
	case err != nil:
		return store.TestMapPlanLease{}, err
	case current.ExpiresAt.After(now):
		return store.TestMapPlanLease{}, &store.TestMapPlanLeaseConflictError{
			PlanID: requested.PlanID, CurrentOwner: current.OwnerID, CurrentExpiry: current.ExpiresAt,
		}
	default:
		changed, err := s.replaceExpiredTestMapPlanLease(ctx, tx, current, requested, now)
		if err != nil {
			return store.TestMapPlanLease{}, err
		}
		if !changed {
			if rollbackErr := rollbackTxBeforeConflict(tx, "expired test map plan lease conflict"); rollbackErr != nil {
				return store.TestMapPlanLease{}, rollbackErr
			}
			return store.TestMapPlanLease{}, s.testMapPlanLeaseConflict(ctx, requested.PlanID, "another executor renewed or claimed the plan")
		}
	}
	if err := s.markTestMapPlanClaimed(ctx, tx, requested.PlanID, now); err != nil {
		return store.TestMapPlanLease{}, err
	}
	if err := tx.Commit(); err != nil {
		return store.TestMapPlanLease{}, fmt.Errorf("commit test map plan %q lease claim: %w", requested.PlanID, err)
	}
	requested.UpdatedAt = now.UTC()
	return requested, nil
}

func prepareTestMapPlanLeaseClaim(requested store.TestMapPlanLease, now time.Time) (store.TestMapPlanLease, time.Time, error) {
	requested.PlanID = strings.TrimSpace(requested.PlanID)
	requested.OwnerID = strings.TrimSpace(requested.OwnerID)
	requested.Token = strings.TrimSpace(requested.Token)
	if requested.PlanID == "" || requested.OwnerID == "" || requested.Token == "" {
		return store.TestMapPlanLease{}, time.Time{}, errors.New("claim test map plan lease: plan id, owner id, and token are required")
	}
	if now.IsZero() {
		now = utcNow()
	}
	if !requested.ExpiresAt.After(now) {
		return store.TestMapPlanLease{}, time.Time{}, errors.New("claim test map plan lease: expiry must be after claim time")
	}
	return requested, now, nil
}

func (s *Store) RenewTestMapPlanLease(ctx context.Context, lease store.TestMapPlanLease, now time.Time, expiresAt time.Time) (_ store.TestMapPlanLease, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.TestMapPlanLease{}, fmt.Errorf("begin test map plan lease renewal: %w", err)
	}
	defer rollbackTxOnError(tx, &err)
	next, err := s.renewTestMapPlanLease(ctx, tx, lease, now, expiresAt)
	if err != nil {
		return store.TestMapPlanLease{}, err
	}
	if err := tx.Commit(); err != nil {
		return store.TestMapPlanLease{}, fmt.Errorf("commit test map plan %q lease renewal: %w", lease.PlanID, err)
	}
	return next, nil
}

func (s *Store) ResetTestMapPlanWithLease(ctx context.Context, lease store.TestMapPlanLease, record store.TestMapPlanRecord, now time.Time, expiresAt time.Time) (_ store.TestMapPlanLease, err error) {
	if err := validateTestMapPlanLeaseTarget(lease, record.Instance.ID); err != nil {
		return store.TestMapPlanLease{}, err
	}
	for _, task := range record.Tasks {
		if err := validateTestMapPlanLeaseTarget(lease, task.PlanID); err != nil {
			return store.TestMapPlanLease{}, err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.TestMapPlanLease{}, fmt.Errorf("begin claimed test map plan reset: %w", err)
	}
	defer rollbackTxOnError(tx, &err)
	next, err := s.renewTestMapPlanLease(ctx, tx, lease, now, expiresAt)
	if err != nil {
		return store.TestMapPlanLease{}, err
	}
	for _, task := range record.Tasks {
		if err := s.updateTestMapPlanTaskRow(ctx, tx, task); err != nil {
			return store.TestMapPlanLease{}, err
		}
	}
	if err := s.updateTestMapPlanInstanceRow(ctx, tx, record.Instance); err != nil {
		return store.TestMapPlanLease{}, err
	}
	if err := tx.Commit(); err != nil {
		return store.TestMapPlanLease{}, fmt.Errorf("commit claimed test map plan %q reset: %w", record.Instance.ID, err)
	}
	return next, nil
}

func (s *Store) UpdateTestMapPlanTaskWithLease(ctx context.Context, lease store.TestMapPlanLease, item store.TestMapPlanTask, now time.Time, expiresAt time.Time) (store.TestMapPlanLease, error) {
	return s.checkpointTestMapPlanWithLease(ctx, lease, item.PlanID, "task", item.ID, now, expiresAt, func(tx *sql.Tx) error {
		return s.updateTestMapPlanTaskRow(ctx, tx, item)
	})
}

func (s *Store) UpdateTestMapPlanInstanceWithLease(ctx context.Context, lease store.TestMapPlanLease, item store.TestMapPlanInstance, now time.Time, expiresAt time.Time) (store.TestMapPlanLease, error) {
	return s.checkpointTestMapPlanWithLease(ctx, lease, item.ID, "plan", item.ID, now, expiresAt, func(tx *sql.Tx) error {
		return s.updateTestMapPlanInstanceRow(ctx, tx, item)
	})
}

func (s *Store) checkpointTestMapPlanWithLease(ctx context.Context, lease store.TestMapPlanLease, planID string, subject string, subjectID string, now time.Time, expiresAt time.Time, update func(*sql.Tx) error) (_ store.TestMapPlanLease, err error) {
	if err := validateTestMapPlanLeaseTarget(lease, planID); err != nil {
		return store.TestMapPlanLease{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.TestMapPlanLease{}, fmt.Errorf("begin claimed test map %s checkpoint: %w", subject, err)
	}
	defer rollbackTxOnError(tx, &err)
	next, err := s.renewTestMapPlanLease(ctx, tx, lease, now, expiresAt)
	if err != nil {
		return store.TestMapPlanLease{}, err
	}
	if err := update(tx); err != nil {
		return store.TestMapPlanLease{}, err
	}
	if err := tx.Commit(); err != nil {
		return store.TestMapPlanLease{}, fmt.Errorf("commit claimed test map %s %q checkpoint: %w", subject, subjectID, err)
	}
	return next, nil
}

func (s *Store) ReleaseTestMapPlanLease(ctx context.Context, lease store.TestMapPlanLease, final store.TestMapPlanInstance, now time.Time) (err error) {
	if err := validateTestMapPlanLeaseTarget(lease, final.ID); err != nil {
		return err
	}
	if now.IsZero() {
		now = utcNow()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin test map plan lease release: %w", err)
	}
	defer rollbackTxOnError(tx, &err)
	if err := s.requireActiveTestMapPlanLease(ctx, tx, lease, now); err != nil {
		return err
	}
	if err := s.updateTestMapPlanInstanceRow(ctx, tx, final); err != nil {
		return err
	}
	query := fmt.Sprintf(`delete from test_map_plan_leases where plan_id = %s and owner_id = %s and lease_token = %s;`, s.dialect.BindVar(1), s.dialect.BindVar(2), s.dialect.BindVar(3))
	result, err := tx.ExecContext(ctx, query, lease.PlanID, lease.OwnerID, lease.Token)
	if err != nil {
		return fmt.Errorf("release test map plan %q lease: %w", lease.PlanID, err)
	}
	if err := requireUpdatedMapPlannerRow(result, "test map plan lease", lease.PlanID); err != nil {
		return fmt.Errorf("%w: %v", store.ErrTestMapPlanLeaseLost, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit test map plan %q lease release: %w", lease.PlanID, err)
	}
	return nil
}

func validateTestMapPlanLeaseTarget(lease store.TestMapPlanLease, planID string) error {
	if strings.TrimSpace(lease.PlanID) == "" || strings.TrimSpace(lease.OwnerID) == "" || strings.TrimSpace(lease.Token) == "" {
		return errors.New("test map plan lease plan id, owner id, and token are required")
	}
	if strings.TrimSpace(planID) != strings.TrimSpace(lease.PlanID) {
		return fmt.Errorf("%w: lease for plan %q cannot update plan %q", store.ErrTestMapPlanLeaseLost, lease.PlanID, planID)
	}
	return nil
}

func (s *Store) renewTestMapPlanLease(ctx context.Context, tx *sql.Tx, lease store.TestMapPlanLease, now time.Time, expiresAt time.Time) (store.TestMapPlanLease, error) {
	if now.IsZero() {
		now = utcNow()
	}
	if !expiresAt.After(now) {
		return store.TestMapPlanLease{}, errors.New("renew test map plan lease: expiry must be after renewal time")
	}
	query := fmt.Sprintf(`
update test_map_plan_leases
set lease_expires_at = %s, updated_at = %s
where plan_id = %s and owner_id = %s and lease_token = %s and lease_expires_at > %s;`,
		s.dialect.BindVar(1), s.dialect.BindVar(2), s.dialect.BindVar(3), s.dialect.BindVar(4), s.dialect.BindVar(5), s.dialect.BindVar(6))
	result, err := tx.ExecContext(ctx, query,
		dbTimeArg(s.dialect, expiresAt), dbTimeArg(s.dialect, now), lease.PlanID, lease.OwnerID, lease.Token, dbTimeArg(s.dialect, now))
	if err != nil {
		return store.TestMapPlanLease{}, fmt.Errorf("renew test map plan %q lease: %w", lease.PlanID, err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return store.TestMapPlanLease{}, fmt.Errorf("inspect test map plan %q lease renewal: %w", lease.PlanID, err)
	}
	if updated == 0 {
		// MySQL reports changed rows by default. An idempotent renewal can
		// therefore report zero even though the ownership predicate matched.
		if err := s.requireActiveTestMapPlanLease(ctx, tx, lease, now); err != nil {
			return store.TestMapPlanLease{}, err
		}
	} else if updated != 1 {
		return store.TestMapPlanLease{}, fmt.Errorf("%w: plan %q is no longer owned by this executor", store.ErrTestMapPlanLeaseLost, lease.PlanID)
	}
	lease.ExpiresAt = expiresAt.UTC()
	lease.UpdatedAt = now.UTC()
	return lease, nil
}

func (s *Store) requireActiveTestMapPlanLease(ctx context.Context, tx *sql.Tx, lease store.TestMapPlanLease, now time.Time) error {
	query := fmt.Sprintf(`select count(*) from test_map_plan_leases where plan_id = %s and owner_id = %s and lease_token = %s and lease_expires_at > %s;`,
		s.dialect.BindVar(1), s.dialect.BindVar(2), s.dialect.BindVar(3), s.dialect.BindVar(4))
	var count int
	if err := tx.QueryRowContext(ctx, query, lease.PlanID, lease.OwnerID, lease.Token, dbTimeArg(s.dialect, now)).Scan(&count); err != nil {
		return fmt.Errorf("check test map plan %q lease: %w", lease.PlanID, err)
	}
	if count != 1 {
		return fmt.Errorf("%w: plan %q is no longer owned by this executor", store.ErrTestMapPlanLeaseLost, lease.PlanID)
	}
	return nil
}

func (s *Store) testMapPlanStatus(ctx context.Context, tx *sql.Tx, planID string) (string, error) {
	query := fmt.Sprintf(`select status from test_map_plan_instances where plan_id = %s;`, s.dialect.BindVar(1))
	var status string
	if err := tx.QueryRowContext(ctx, query, planID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", store.ErrNotFound
		}
		return "", fmt.Errorf("read test map plan %q status: %w", planID, err)
	}
	return status, nil
}

func (s *Store) getTestMapPlanLease(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, planID string) (store.TestMapPlanLease, error) {
	query := fmt.Sprintf(`select plan_id, owner_id, lease_token, lease_expires_at, updated_at from test_map_plan_leases where plan_id = %s;`, s.dialect.BindVar(1))
	var lease store.TestMapPlanLease
	var expiresAt, updatedAt any
	if err := queryer.QueryRowContext(ctx, query, planID).Scan(&lease.PlanID, &lease.OwnerID, &lease.Token, &expiresAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.TestMapPlanLease{}, store.ErrNotFound
		}
		return store.TestMapPlanLease{}, fmt.Errorf("read test map plan %q lease: %w", planID, err)
	}
	lease.ExpiresAt = decodeDBTime(expiresAt)
	lease.UpdatedAt = decodeDBTime(updatedAt)
	return lease, nil
}

func (s *Store) insertTestMapPlanLease(ctx context.Context, tx *sql.Tx, lease store.TestMapPlanLease, now time.Time) error {
	query := fmt.Sprintf(`insert into test_map_plan_leases (plan_id, owner_id, lease_token, lease_expires_at, updated_at) values (%s);`, s.bindVars(5))
	_, err := tx.ExecContext(ctx, query, lease.PlanID, lease.OwnerID, lease.Token, dbTimeArg(s.dialect, lease.ExpiresAt), dbTimeArg(s.dialect, now))
	return err
}

func (s *Store) replaceExpiredTestMapPlanLease(ctx context.Context, tx *sql.Tx, current store.TestMapPlanLease, requested store.TestMapPlanLease, now time.Time) (bool, error) {
	query := fmt.Sprintf(`
update test_map_plan_leases
set owner_id = %s, lease_token = %s, lease_expires_at = %s, updated_at = %s
where plan_id = %s and lease_token = %s and lease_expires_at <= %s;`,
		s.dialect.BindVar(1), s.dialect.BindVar(2), s.dialect.BindVar(3), s.dialect.BindVar(4), s.dialect.BindVar(5), s.dialect.BindVar(6), s.dialect.BindVar(7))
	result, err := tx.ExecContext(ctx, query,
		requested.OwnerID, requested.Token, dbTimeArg(s.dialect, requested.ExpiresAt), dbTimeArg(s.dialect, now),
		requested.PlanID, current.Token, dbTimeArg(s.dialect, now))
	if err != nil {
		return false, fmt.Errorf("replace expired test map plan %q lease: %w", requested.PlanID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect expired test map plan %q lease claim: %w", requested.PlanID, err)
	}
	return changed == 1, nil
}

func (s *Store) markTestMapPlanClaimed(ctx context.Context, tx *sql.Tx, planID string, now time.Time) error {
	query := fmt.Sprintf(`update test_map_plan_instances set status = %s, started_at = %s, finished_at = %s where plan_id = %s;`,
		s.dialect.BindVar(1), s.dialect.BindVar(2), s.dialect.BindVar(3), s.dialect.BindVar(4))
	result, err := tx.ExecContext(ctx, query, store.StatusRunning, dbTimeArg(s.dialect, now), dbTimeArg(s.dialect, time.Time{}), planID)
	if err != nil {
		return fmt.Errorf("mark test map plan %q claimed: %w", planID, err)
	}
	return requireUpdatedMapPlannerRow(result, "test map plan instance", planID)
}

func (s *Store) testMapPlanLeaseConflict(ctx context.Context, planID string, reason string) error {
	current, err := s.getTestMapPlanLease(ctx, s.db, planID)
	if err != nil {
		return &store.TestMapPlanLeaseConflictError{PlanID: planID, Reason: reason}
	}
	return &store.TestMapPlanLeaseConflictError{
		PlanID: planID, CurrentOwner: current.OwnerID, CurrentExpiry: current.ExpiresAt, Reason: reason,
	}
}
