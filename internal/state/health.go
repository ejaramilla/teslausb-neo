package state

import (
	"fmt"
	"time"
)

// RecordHealth inserts a health metric snapshot.
func (d *DB) RecordHealth(cpuTempMC, storageUsed, storageFree int64) error {
	_, err := d.db.Exec(
		`INSERT INTO health_metrics (timestamp, cpu_temp_mc, storage_used_bytes, storage_free_bytes)
		 VALUES (datetime('now'), ?, ?, ?)`,
		cpuTempMC, storageUsed, storageFree,
	)
	if err != nil {
		return fmt.Errorf("record health: %w", err)
	}
	return nil
}

// RecentHealth returns the most recent health metrics, ordered by timestamp
// descending.
func (d *DB) RecentHealth(limit int) ([]HealthMetric, error) {
	rows, err := d.db.Query(
		`SELECT id, timestamp, cpu_temp_mc, storage_used_bytes, storage_free_bytes
		 FROM health_metrics
		 ORDER BY timestamp DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query recent health: %w", err)
	}
	defer rows.Close()

	var metrics []HealthMetric
	for rows.Next() {
		var m HealthMetric
		var tsStr string
		if err := rows.Scan(&m.ID, &tsStr, &m.CPUTempMC, &m.StorageUsedBytes, &m.StorageFreeBytes); err != nil {
			return nil, fmt.Errorf("scan health row: %w", err)
		}
		m.Timestamp, _ = time.Parse("2006-01-02 15:04:05", tsStr)
		metrics = append(metrics, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate health metrics: %w", err)
	}
	return metrics, nil
}
