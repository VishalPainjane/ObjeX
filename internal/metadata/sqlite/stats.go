package sqlite

import "context"

// AggregateStats returns total object count and bytes across all buckets.
func (s *Store) AggregateStats(ctx context.Context) (objectCount, totalSize int64, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(object_count), 0), COALESCE(SUM(total_size), 0) FROM buckets`,
	).Scan(&objectCount, &totalSize)
	return objectCount, totalSize, err
}
