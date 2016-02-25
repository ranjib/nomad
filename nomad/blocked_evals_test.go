package nomad

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/hashicorp/nomad/nomad/mock"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/testutil"
)

func testBlockedEvals(t *testing.T) (*BlockedEvals, *EvalBroker) {
	broker := testBroker(t, 0)
	broker.SetEnabled(true)
	blocked := NewBlockedEvals(broker)
	blocked.SetEnabled(true)
	return blocked, broker
}

func TestBlockedEvals_Block_Disabled(t *testing.T) {
	blocked, _ := testBlockedEvals(t)
	blocked.SetEnabled(false)

	// Create an escaped eval and add it to the blocked tracker.
	e := mock.Eval()
	e.Status = structs.EvalStatusBlocked
	e.EscapedComputedClass = true
	blocked.Block(e)

	// Verify block did nothing
	bStats := blocked.Stats()
	if bStats.TotalBlocked != 0 || bStats.TotalEscaped != 0 {
		t.Fatalf("bad: %#v", bStats)
	}
}

func TestBlockedEvals_Block_SameJob(t *testing.T) {
	blocked, _ := testBlockedEvals(t)

	// Create two blocked evals and add them to the blocked tracker.
	e := mock.Eval()
	e2 := mock.Eval()
	e2.JobID = e.JobID
	blocked.Block(e)
	blocked.Block(e2)

	// Verify block did track both
	bStats := blocked.Stats()
	if bStats.TotalBlocked != 1 || bStats.TotalEscaped != 0 {
		t.Fatalf("bad: %#v", bStats)
	}
}

func TestBlockedEvals_GetDuplicates(t *testing.T) {
	blocked, _ := testBlockedEvals(t)

	// Create duplicate blocked evals and add them to the blocked tracker.
	e := mock.Eval()
	e2 := mock.Eval()
	e2.JobID = e.JobID
	e3 := mock.Eval()
	e3.JobID = e.JobID
	blocked.Block(e)
	blocked.Block(e2)

	// Verify block did track both
	bStats := blocked.Stats()
	if bStats.TotalBlocked != 1 || bStats.TotalEscaped != 0 {
		t.Fatalf("bad: %#v", bStats)
	}

	// Get the duplicates.
	out := blocked.GetDuplicates(0)
	if len(out) != 1 || !reflect.DeepEqual(out[0], e2) {
		t.Fatalf("bad: %#v %#v", out, e2)
	}

	// Call block again after a small sleep.
	go func() {
		time.Sleep(500 * time.Millisecond)
		blocked.Block(e3)
	}()

	// Get the duplicates.
	out = blocked.GetDuplicates(1 * time.Second)
	if len(out) != 1 || !reflect.DeepEqual(out[0], e3) {
		t.Fatalf("bad: %#v %#v", out, e2)
	}
}

func TestBlockedEvals_UnblockEscaped(t *testing.T) {
	blocked, broker := testBlockedEvals(t)

	// Create an escaped eval and add it to the blocked tracker.
	e := mock.Eval()
	e.Status = structs.EvalStatusBlocked
	e.EscapedComputedClass = true
	blocked.Block(e)

	// Verify block caused the eval to be tracked
	bStats := blocked.Stats()
	if bStats.TotalBlocked != 1 || bStats.TotalEscaped != 1 {
		t.Fatalf("bad: %#v", bStats)
	}

	blocked.Unblock("v1:123")

	testutil.WaitForResult(func() (bool, error) {
		// Verify Unblock caused an enqueue
		brokerStats := broker.Stats()
		if brokerStats.TotalReady != 1 {
			return false, fmt.Errorf("bad: %#v", brokerStats)
		}

		// Verify Unblock updates the stats
		bStats := blocked.Stats()
		if bStats.TotalBlocked != 0 || bStats.TotalEscaped != 0 {
			return false, fmt.Errorf("bad: %#v", bStats)
		}
		return true, nil
	}, func(err error) {
		t.Fatalf("err: %s", err)
	})
}

func TestBlockedEvals_UnblockEligible(t *testing.T) {
	blocked, broker := testBlockedEvals(t)

	// Create a blocked eval that is eligible on a specific node class and add
	// it to the blocked tracker.
	e := mock.Eval()
	e.Status = structs.EvalStatusBlocked
	e.ClassEligibility = map[string]bool{"v1:123": true}
	blocked.Block(e)

	// Verify block caused the eval to be tracked
	blockedStats := blocked.Stats()
	if blockedStats.TotalBlocked != 1 {
		t.Fatalf("bad: %#v", blockedStats)
	}

	blocked.Unblock("v1:123")

	testutil.WaitForResult(func() (bool, error) {
		// Verify Unblock caused an enqueue
		brokerStats := broker.Stats()
		if brokerStats.TotalReady != 1 {
			return false, fmt.Errorf("bad: %#v", brokerStats)
		}

		// Verify Unblock updates the stats
		bStats := blocked.Stats()
		if bStats.TotalBlocked != 0 || bStats.TotalEscaped != 0 {
			return false, fmt.Errorf("bad: %#v", bStats)
		}
		return true, nil
	}, func(err error) {
		t.Fatalf("err: %s", err)
	})
}

func TestBlockedEvals_UnblockIneligible(t *testing.T) {
	blocked, broker := testBlockedEvals(t)

	// Create a blocked eval that is ineligible on a specific node class and add
	// it to the blocked tracker.
	e := mock.Eval()
	e.Status = structs.EvalStatusBlocked
	e.ClassEligibility = map[string]bool{"v1:123": false}
	blocked.Block(e)

	// Verify block caused the eval to be tracked
	blockedStats := blocked.Stats()
	if blockedStats.TotalBlocked != 1 && blockedStats.TotalEscaped != 0 {
		t.Fatalf("bad: %#v", blockedStats)
	}

	// Should do nothing
	blocked.Unblock("v1:123")

	testutil.WaitForResult(func() (bool, error) {
		// Verify Unblock didn't cause an enqueue
		brokerStats := broker.Stats()
		if brokerStats.TotalReady != 0 {
			return false, fmt.Errorf("bad: %#v", brokerStats)
		}

		bStats := blocked.Stats()
		if bStats.TotalBlocked != 1 || bStats.TotalEscaped != 0 {
			return false, fmt.Errorf("bad: %#v", bStats)
		}
		return true, nil
	}, func(err error) {
		t.Fatalf("err: %s", err)
	})
}

func TestBlockedEvals_UnblockUnknown(t *testing.T) {
	blocked, broker := testBlockedEvals(t)

	// Create a blocked eval that is ineligible on a specific node class and add
	// it to the blocked tracker.
	e := mock.Eval()
	e.Status = structs.EvalStatusBlocked
	e.ClassEligibility = map[string]bool{"v1:123": true, "v1:456": false}
	blocked.Block(e)

	// Verify block caused the eval to be tracked
	blockedStats := blocked.Stats()
	if blockedStats.TotalBlocked != 1 && blockedStats.TotalEscaped != 0 {
		t.Fatalf("bad: %#v", blockedStats)
	}

	// Should unblock because the eval hasn't seen this node class.
	blocked.Unblock("v1:789")

	testutil.WaitForResult(func() (bool, error) {
		// Verify Unblock causes an enqueue
		brokerStats := broker.Stats()
		if brokerStats.TotalReady != 1 {
			return false, fmt.Errorf("bad: %#v", brokerStats)
		}

		// Verify Unblock updates the stats
		bStats := blocked.Stats()
		if bStats.TotalBlocked != 0 || bStats.TotalEscaped != 0 {
			return false, fmt.Errorf("bad: %#v", bStats)
		}
		return true, nil
	}, func(err error) {
		t.Fatalf("err: %s", err)
	})
}
