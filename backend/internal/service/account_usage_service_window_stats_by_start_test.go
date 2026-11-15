package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

// windowStatsRepoStub answers every account with StandardCost == account ID and
// records how it was queried.
type windowStatsRepoStub struct {
	usageBatchLogRepoStub
	mu          sync.Mutex
	singleCalls []int64
	singleErrID int64
}

func (r *windowStatsRepoStub) GetAccountWindowStats(_ context.Context, accountID int64, _ time.Time) (*usagestats.AccountStats, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.singleCalls = append(r.singleCalls, accountID)
	if accountID == r.singleErrID {
		return nil, errors.New("query failed")
	}
	return &usagestats.AccountStats{StandardCost: float64(accountID)}, nil
}

type windowStatsBatchRepoStub struct {
	windowStatsRepoStub
	batchCalls map[time.Time][]int64
	batchErrAt time.Time
}

func (r *windowStatsBatchRepoStub) GetAccountWindowStatsBatch(_ context.Context, accountIDs []int64, startTime time.Time) (map[int64]*usagestats.AccountStats, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.batchCalls == nil {
		r.batchCalls = make(map[time.Time][]int64)
	}
	r.batchCalls[startTime] = append([]int64(nil), accountIDs...)
	if startTime.Equal(r.batchErrAt) {
		return nil, errors.New("batch failed")
	}
	result := make(map[int64]*usagestats.AccountStats, len(accountIDs))
	for _, accountID := range accountIDs {
		if accountID == 3 {
			continue // 没有用量的账号不在批量结果里
		}
		result[accountID] = &usagestats.AccountStats{StandardCost: float64(accountID)}
	}
	return result, nil
}

func TestGetAccountWindowStatsByStartGroupsByWindowStart(t *testing.T) {
	early := time.Date(2026, 11, 15, 8, 0, 0, 0, time.UTC)
	late := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	repo := &windowStatsBatchRepoStub{}
	svc := &AccountUsageService{usageLogRepo: repo}

	got := svc.GetAccountWindowStatsByStart(context.Background(), map[int64]time.Time{
		1: early,
		2: early.In(time.FixedZone("CST", 8*3600)), // 同一时刻的不同时区表示
		3: early,
		4: late,
	})

	require.Len(t, repo.batchCalls, 2, "one query per distinct window start")
	require.ElementsMatch(t, []int64{1, 2, 3}, repo.batchCalls[early])
	require.ElementsMatch(t, []int64{4}, repo.batchCalls[late])
	require.Empty(t, repo.singleCalls)

	require.Len(t, got, 4)
	require.Equal(t, 1.0, got[1].StandardCost)
	require.Equal(t, 2.0, got[2].StandardCost)
	require.Equal(t, 0.0, got[3].StandardCost, "account without usage gets zero stats")
	require.Equal(t, 4.0, got[4].StandardCost)
}

func TestGetAccountWindowStatsByStartFallsBackPerAccount(t *testing.T) {
	start := time.Date(2026, 11, 15, 8, 0, 0, 0, time.UTC)

	t.Run("batch query fails", func(t *testing.T) {
		repo := &windowStatsBatchRepoStub{batchErrAt: start}
		svc := &AccountUsageService{usageLogRepo: repo}

		got := svc.GetAccountWindowStatsByStart(context.Background(), map[int64]time.Time{1: start, 2: start})

		require.ElementsMatch(t, []int64{1, 2}, repo.singleCalls)
		require.Len(t, got, 2)
		require.Equal(t, 2.0, got[2].StandardCost)
	})

	t.Run("repository has no batch support and one account fails", func(t *testing.T) {
		repo := &windowStatsRepoStub{singleErrID: 2}
		svc := &AccountUsageService{usageLogRepo: repo}

		got := svc.GetAccountWindowStatsByStart(context.Background(), map[int64]time.Time{1: start, 2: start})

		require.ElementsMatch(t, []int64{1, 2}, repo.singleCalls)
		require.Len(t, got, 1, "failed account is left out")
		require.Equal(t, 1.0, got[1].StandardCost)
	})
}

func TestGetAccountWindowStatsByStartEmptyInput(t *testing.T) {
	var nilService *AccountUsageService
	require.Empty(t, nilService.GetAccountWindowStatsByStart(context.Background(), map[int64]time.Time{1: time.Now()}))
	require.Empty(t, (&AccountUsageService{}).GetAccountWindowStatsByStart(context.Background(), map[int64]time.Time{1: time.Now()}))
	require.Empty(t, (&AccountUsageService{usageLogRepo: &windowStatsRepoStub{}}).GetAccountWindowStatsByStart(context.Background(), nil))
}
