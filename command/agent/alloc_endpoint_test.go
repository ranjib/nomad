package agent

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/nomad/nomad/mock"
	"github.com/hashicorp/nomad/nomad/structs"
)

func TestHTTP_AllocsList(t *testing.T) {
	httpTest(t, nil, func(s *TestServer) {
		// Directly manipulate the state
		state := s.Agent.server.State()
		alloc1 := mock.Alloc()
		alloc2 := mock.Alloc()
		err := state.UpsertAllocs(1000,
			[]*structs.Allocation{alloc1, alloc2})
		if err != nil {
			t.Fatalf("err: %v", err)
		}

		// Make the HTTP request
		req, err := http.NewRequest("GET", "/v1/allocations", nil)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		respW := httptest.NewRecorder()

		// Make the request
		obj, err := s.Server.AllocsRequest(respW, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}

		// Check for the index
		if respW.HeaderMap.Get("X-Nomad-Index") == "" {
			t.Fatalf("missing index")
		}
		if respW.HeaderMap.Get("X-Nomad-KnownLeader") != "true" {
			t.Fatalf("missing known leader")
		}
		if respW.HeaderMap.Get("X-Nomad-LastContact") == "" {
			t.Fatalf("missing last contact")
		}

		// Check the alloc
		n := obj.([]*structs.AllocListStub)
		if len(n) != 2 {
			t.Fatalf("bad: %#v", n)
		}
	})
}

func TestHTTP_AllocsPrefixList(t *testing.T) {
	httpTest(t, nil, func(s *TestServer) {
		// Directly manipulate the state
		state := s.Agent.server.State()
		alloc1 := mock.Alloc()
		alloc1.ID = "aaaaaaaa-e8f7-fd38-c855-ab94ceb89706"
		alloc2 := mock.Alloc()
		alloc2.ID = "aaabbbbb-e8f7-fd38-c855-ab94ceb89706"
		err := state.UpsertAllocs(1000,
			[]*structs.Allocation{alloc1, alloc2})
		if err != nil {
			t.Fatalf("err: %v", err)
		}

		// Make the HTTP request
		req, err := http.NewRequest("GET", "/v1/allocations?prefix=aaab", nil)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		respW := httptest.NewRecorder()

		// Make the request
		obj, err := s.Server.AllocsRequest(respW, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}

		// Check for the index
		if respW.HeaderMap.Get("X-Nomad-Index") == "" {
			t.Fatalf("missing index")
		}
		if respW.HeaderMap.Get("X-Nomad-KnownLeader") != "true" {
			t.Fatalf("missing known leader")
		}
		if respW.HeaderMap.Get("X-Nomad-LastContact") == "" {
			t.Fatalf("missing last contact")
		}

		// Check the alloc
		n := obj.([]*structs.AllocListStub)
		if len(n) != 1 {
			t.Fatalf("bad: %#v", n)
		}

		// Check the identifier
		if n[0].ID != alloc2.ID {
			t.Fatalf("expected alloc ID: %v, Actual: %v", alloc2.ID, n[0].ID)
		}
	})
}

func TestHTTP_AllocQuery(t *testing.T) {
	httpTest(t, nil, func(s *TestServer) {
		// Directly manipulate the state
		state := s.Agent.server.State()
		alloc := mock.Alloc()
		err := state.UpsertAllocs(1000,
			[]*structs.Allocation{alloc})
		if err != nil {
			t.Fatalf("err: %v", err)
		}

		// Make the HTTP request
		req, err := http.NewRequest("GET", "/v1/allocation/"+alloc.ID, nil)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		respW := httptest.NewRecorder()

		// Make the request
		obj, err := s.Server.AllocSpecificRequest(respW, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}

		// Check for the index
		if respW.HeaderMap.Get("X-Nomad-Index") == "" {
			t.Fatalf("missing index")
		}
		if respW.HeaderMap.Get("X-Nomad-KnownLeader") != "true" {
			t.Fatalf("missing known leader")
		}
		if respW.HeaderMap.Get("X-Nomad-LastContact") == "" {
			t.Fatalf("missing last contact")
		}

		// Check the job
		a := obj.(*structs.Allocation)
		if a.ID != alloc.ID {
			t.Fatalf("bad: %#v", a)
		}
	})
}
