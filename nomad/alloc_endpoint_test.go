package nomad

import (
	"reflect"
	"testing"
	"time"

	"github.com/hashicorp/net-rpc-msgpackrpc"
	"github.com/hashicorp/nomad/nomad/mock"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/testutil"
)

func TestAllocEndpoint_List(t *testing.T) {
	s1 := testServer(t, nil)
	defer s1.Shutdown()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create the register request
	alloc := mock.Alloc()
	state := s1.fsm.State()
	err := state.UpsertAllocs(1000, []*structs.Allocation{alloc})
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Lookup the allocations
	get := &structs.AllocListRequest{
		QueryOptions: structs.QueryOptions{Region: "global"},
	}
	var resp structs.AllocListResponse
	if err := msgpackrpc.CallWithCodec(codec, "Alloc.List", get, &resp); err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Index != 1000 {
		t.Fatalf("Bad index: %d %d", resp.Index, 1000)
	}

	if len(resp.Allocations) != 1 {
		t.Fatalf("bad: %#v", resp.Allocations)
	}
	if resp.Allocations[0].ID != alloc.ID {
		t.Fatalf("bad: %#v", resp.Allocations[0])
	}

	// Lookup the allocations by prefix
	get = &structs.AllocListRequest{
		QueryOptions: structs.QueryOptions{Region: "global", Prefix: alloc.ID[:4]},
	}

	var resp2 structs.AllocListResponse
	if err := msgpackrpc.CallWithCodec(codec, "Alloc.List", get, &resp2); err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp2.Index != 1000 {
		t.Fatalf("Bad index: %d %d", resp2.Index, 1000)
	}

	if len(resp2.Allocations) != 1 {
		t.Fatalf("bad: %#v", resp2.Allocations)
	}
	if resp2.Allocations[0].ID != alloc.ID {
		t.Fatalf("bad: %#v", resp2.Allocations[0])
	}
}

func TestAllocEndpoint_List_Blocking(t *testing.T) {
	s1 := testServer(t, nil)
	defer s1.Shutdown()
	state := s1.fsm.State()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create the alloc
	alloc := mock.Alloc()

	// Upsert alloc triggers watches
	time.AfterFunc(100*time.Millisecond, func() {
		if err := state.UpsertAllocs(2, []*structs.Allocation{alloc}); err != nil {
			t.Fatalf("err: %v", err)
		}
	})

	req := &structs.AllocListRequest{
		QueryOptions: structs.QueryOptions{
			Region:        "global",
			MinQueryIndex: 1,
		},
	}
	start := time.Now()
	var resp structs.AllocListResponse
	if err := msgpackrpc.CallWithCodec(codec, "Alloc.List", req, &resp); err != nil {
		t.Fatalf("err: %v", err)
	}

	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("should block (returned in %s) %#v", elapsed, resp)
	}
	if resp.Index != 2 {
		t.Fatalf("Bad index: %d %d", resp.Index, 2)
	}
	if len(resp.Allocations) != 1 || resp.Allocations[0].ID != alloc.ID {
		t.Fatalf("bad: %#v", resp.Allocations)
	}

	// Client updates trigger watches
	alloc2 := mock.Alloc()
	alloc2.ID = alloc.ID
	alloc2.ClientStatus = structs.AllocClientStatusRunning
	time.AfterFunc(100*time.Millisecond, func() {
		if err := state.UpdateAllocFromClient(3, alloc2); err != nil {
			t.Fatalf("err: %v", err)
		}
	})

	req.MinQueryIndex = 2
	start = time.Now()
	var resp2 structs.AllocListResponse
	if err := msgpackrpc.CallWithCodec(codec, "Alloc.List", req, &resp2); err != nil {
		t.Fatalf("err: %v", err)
	}

	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("should block (returned in %s) %#v", elapsed, resp2)
	}
	if resp2.Index != 3 {
		t.Fatalf("Bad index: %d %d", resp2.Index, 3)
	}
	if len(resp2.Allocations) != 1 || resp.Allocations[0].ID != alloc.ID ||
		resp2.Allocations[0].ClientStatus != structs.AllocClientStatusRunning {
		t.Fatalf("bad: %#v", resp2.Allocations)
	}
}

func TestAllocEndpoint_GetAlloc(t *testing.T) {
	s1 := testServer(t, nil)
	defer s1.Shutdown()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create the register request
	alloc := mock.Alloc()
	state := s1.fsm.State()
	err := state.UpsertAllocs(1000, []*structs.Allocation{alloc})
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Lookup the jobs
	get := &structs.AllocSpecificRequest{
		AllocID:      alloc.ID,
		QueryOptions: structs.QueryOptions{Region: "global"},
	}
	var resp structs.SingleAllocResponse
	if err := msgpackrpc.CallWithCodec(codec, "Alloc.GetAlloc", get, &resp); err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Index != 1000 {
		t.Fatalf("Bad index: %d %d", resp.Index, 1000)
	}

	if !reflect.DeepEqual(alloc, resp.Alloc) {
		t.Fatalf("bad: %#v", resp.Alloc)
	}
}

func TestAllocEndpoint_GetAlloc_Blocking(t *testing.T) {
	s1 := testServer(t, nil)
	defer s1.Shutdown()
	state := s1.fsm.State()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create the allocs
	alloc1 := mock.Alloc()
	alloc2 := mock.Alloc()

	// First create an unrelated alloc
	time.AfterFunc(100*time.Millisecond, func() {
		err := state.UpsertAllocs(100, []*structs.Allocation{alloc1})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
	})

	// Create the alloc we are watching later
	time.AfterFunc(200*time.Millisecond, func() {
		err := state.UpsertAllocs(200, []*structs.Allocation{alloc2})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
	})

	// Lookup the jobs
	get := &structs.AllocSpecificRequest{
		AllocID: alloc2.ID,
		QueryOptions: structs.QueryOptions{
			Region:        "global",
			MinQueryIndex: 50,
		},
	}
	var resp structs.SingleAllocResponse
	start := time.Now()
	if err := msgpackrpc.CallWithCodec(codec, "Alloc.GetAlloc", get, &resp); err != nil {
		t.Fatalf("err: %v", err)
	}

	if elapsed := time.Since(start); elapsed < 200*time.Millisecond {
		t.Fatalf("should block (returned in %s) %#v", elapsed, resp)
	}
	if resp.Index != 200 {
		t.Fatalf("Bad index: %d %d", resp.Index, 200)
	}
	if resp.Alloc == nil || resp.Alloc.ID != alloc2.ID {
		t.Fatalf("bad: %#v", resp.Alloc)
	}
}
