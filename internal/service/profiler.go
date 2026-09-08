package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ahmed/odoonoir/internal/db"
)

// SlowQuery represents one pg_stat_statements row.
type SlowQuery struct {
	Query    string  `json:"query"`
	Calls    int64   `json:"calls"`
	MeanTime float64 `json:"meanTime"`
	TotalTime float64 `json:"totalTime"`
}

// GetSlowQueries returns top N slow queries for a database (requires pg_stat_statements extension).
func (s *Service) GetSlowQueries(name, dbName string, limit int) ([]SlowQuery, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	if dbName, err = s.ResolveDB(inst, dbName, true); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	// Ensure extension exists (ignore error if already)
	_, _ = s.pg.Query(dbName, "CREATE EXTENSION IF NOT EXISTS pg_stat_statements")
	q := fmt.Sprintf(`SELECT query, calls, mean_exec_time, total_exec_time FROM pg_stat_statements WHERE dbid = (SELECT oid FROM pg_database WHERE datname = '%s') ORDER BY mean_exec_time DESC LIMIT %d`, db.PgEscapeLiteral(dbName), limit)
	out, err := s.pg.Query(dbName, q)
	if err != nil {
		return nil, s.wrapPsqlErr(dbName, err)
	}
	var res []SlowQuery
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}
		var sq SlowQuery
		sq.Query = strings.TrimSpace(parts[0])
		fmt.Sscanf(parts[1], "%d", &sq.Calls)
		fmt.Sscanf(parts[2], "%f", &sq.MeanTime)
		fmt.Sscanf(parts[3], "%f", &sq.TotalTime)
		res = append(res, sq)
	}
	return res, nil
}

// GetFlameGraph runs py-spy for duration seconds and returns svg path.
func (s *Service) GetFlameGraph(name string, seconds int) (string, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return "", err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	mgr := s.procFor(inst)
	_, pid, err := mgr.Status()
	if err != nil || pid == 0 {
		return "", fmt.Errorf("instance not running")
	}
	if seconds <= 0 {
		seconds = 10
	}
	if seconds > 30 {
		seconds = 30
	}
	out := filepath.Join(p.Root, "flame.svg")
	// check py-spy exists
	if _, err := exec.LookPath("py-spy"); err != nil {
		return "", fmt.Errorf("py-spy not found — install with: pip install py-spy")
	}
	cmd := exec.Command("py-spy", "record", "-o", out, "-p", fmt.Sprint(pid), "--duration", fmt.Sprint(seconds), "--rate", "50")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("py-spy: %w", err)
	}
	if _, err := os.Stat(out); err != nil {
		return "", fmt.Errorf("flame graph not created")
	}
	return out, nil
}
