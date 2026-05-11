package store

import (
	"context"
	"errors"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestPostgresLeaseConcurrency(t *testing.T) {
	dsn := os.Getenv("STACYVM_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("set STACYVM_POSTGRES_TEST_DSN to run Postgres lease concurrency test")
	}

	const contenders = 16
	ctx := context.Background()
	stores := make([]*PostgresStore, 0, contenders)
	for i := 0; i < contenders; i++ {
		st, err := NewPostgresStore(dsn)
		if err != nil {
			t.Fatalf("open postgres store %d: %v", i, err)
		}
		stores = append(stores, st)
		t.Cleanup(func() { _ = st.Close() })
	}
	resetPostgresContractStore(t, stores[0])

	var wg sync.WaitGroup
	winners := make(chan string, contenders)
	failures := make(chan error, contenders)
	for i := 0; i < contenders; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			holderID := "worker-" + strconv.Itoa(i)
			lease, err := stores[i].AcquireLease(ctx, "lease-race", "sandbox", holderID, time.Minute)
			if err == nil {
				winners <- lease.HolderID
				return
			}
			if !errors.Is(err, ErrConflict) {
				failures <- err
			}
		}()
	}
	wg.Wait()
	close(winners)
	close(failures)

	if len(failures) > 0 {
		t.Fatalf("unexpected acquire error: %v", <-failures)
	}
	if len(winners) != 1 {
		t.Fatalf("lease winners = %d, want 1", len(winners))
	}
	winner := <-winners
	lease, err := stores[0].GetLease(ctx, "lease-race")
	if err != nil {
		t.Fatalf("get lease: %v", err)
	}
	if lease.HolderID != winner {
		t.Fatalf("stored holder = %q, want winning holder %q", lease.HolderID, winner)
	}

	renewed, err := stores[0].RenewLease(ctx, "lease-race", winner, time.Nanosecond)
	if err != nil {
		t.Fatalf("renew lease: %v", err)
	}
	if renewed.Generation != lease.Generation+1 {
		t.Fatalf("renewed generation = %d, want %d", renewed.Generation, lease.Generation+1)
	}
	time.Sleep(2 * time.Millisecond)

	var takeoverWG sync.WaitGroup
	takeovers := make(chan string, contenders)
	takeoverFailures := make(chan error, contenders)
	for i := 0; i < contenders; i++ {
		i := i
		takeoverWG.Add(1)
		go func() {
			defer takeoverWG.Done()
			holderID := "takeover-worker-" + strconv.Itoa(i)
			lease, err := stores[i].AcquireLease(ctx, "lease-race", "sandbox", holderID, time.Minute)
			if err == nil {
				takeovers <- lease.HolderID
				return
			}
			if !errors.Is(err, ErrConflict) {
				takeoverFailures <- err
			}
		}()
	}
	takeoverWG.Wait()
	close(takeovers)
	close(takeoverFailures)

	if len(takeoverFailures) > 0 {
		t.Fatalf("unexpected takeover error: %v", <-takeoverFailures)
	}
	if len(takeovers) != 1 {
		t.Fatalf("takeover winners = %d, want 1", len(takeovers))
	}
	takeoverWinner := <-takeovers
	if takeoverWinner == winner {
		t.Fatalf("takeover holder = original holder %q, want a new holder", takeoverWinner)
	}
	lease, err = stores[0].GetLease(ctx, "lease-race")
	if err != nil {
		t.Fatalf("get takeover lease: %v", err)
	}
	if lease.HolderID != takeoverWinner {
		t.Fatalf("stored takeover holder = %q, want %q", lease.HolderID, takeoverWinner)
	}
	if lease.Generation <= renewed.Generation {
		t.Fatalf("takeover generation = %d, want > %d", lease.Generation, renewed.Generation)
	}
}
