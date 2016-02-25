package nomad

import (
	"fmt"
	"math"
	"time"

	"github.com/hashicorp/nomad/nomad/state"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/scheduler"
)

// CoreScheduler is a special "scheduler" that is registered
// as "_core". It is used to run various administrative work
// across the cluster.
type CoreScheduler struct {
	srv  *Server
	snap *state.StateSnapshot
}

// NewCoreScheduler is used to return a new system scheduler instance
func NewCoreScheduler(srv *Server, snap *state.StateSnapshot) scheduler.Scheduler {
	s := &CoreScheduler{
		srv:  srv,
		snap: snap,
	}
	return s
}

// Process is used to implement the scheduler.Scheduler interface
func (s *CoreScheduler) Process(eval *structs.Evaluation) error {
	switch eval.JobID {
	case structs.CoreJobEvalGC:
		return s.evalGC(eval)
	case structs.CoreJobNodeGC:
		return s.nodeGC(eval)
	case structs.CoreJobJobGC:
		return s.jobGC(eval)
	default:
		return fmt.Errorf("core scheduler cannot handle job '%s'", eval.JobID)
	}
}

// jobGC is used to garbage collect eligible jobs.
func (c *CoreScheduler) jobGC(eval *structs.Evaluation) error {
	// Get all the jobs eligible for garbage collection.
	iter, err := c.snap.JobsByGC(true)
	if err != nil {
		return err
	}

	var oldThreshold uint64
	if eval.TriggeredBy == structs.EvalTriggerForceGC {
		// The GC was forced, so set the threshold to its maximum so everything
		// will GC.
		oldThreshold = math.MaxUint64
		c.srv.logger.Println("[DEBUG] sched.core: forced job GC")
	} else {
		// Get the time table to calculate GC cutoffs.
		tt := c.srv.fsm.TimeTable()
		cutoff := time.Now().UTC().Add(-1 * c.srv.config.JobGCThreshold)
		oldThreshold = tt.NearestIndex(cutoff)
	}
	c.srv.logger.Printf("[DEBUG] sched.core: job GC: scanning before index %d (%v)",
		oldThreshold, c.srv.config.JobGCThreshold)

	// Collect the allocations, evaluations and jobs to GC
	var gcAlloc, gcEval, gcJob []string

OUTER:
	for i := iter.Next(); i != nil; i = iter.Next() {
		job := i.(*structs.Job)

		// Ignore new jobs.
		if job.CreateIndex > oldThreshold {
			continue
		}

		evals, err := c.snap.EvalsByJob(job.ID)
		if err != nil {
			c.srv.logger.Printf("[ERR] sched.core: failed to get evals for job %s: %v", job.ID, err)
			continue
		}

		for _, eval := range evals {
			gc, allocs, err := c.gcEval(eval, oldThreshold)
			if err != nil || !gc {
				continue OUTER
			}

			gcEval = append(gcEval, eval.ID)
			gcAlloc = append(gcAlloc, allocs...)
		}

		// Job is eligible for garbage collection
		gcJob = append(gcJob, job.ID)
	}

	// Fast-path the nothing case
	if len(gcEval) == 0 && len(gcAlloc) == 0 && len(gcJob) == 0 {
		return nil
	}
	c.srv.logger.Printf("[DEBUG] sched.core: job GC: %d jobs, %d evaluations, %d allocs eligible",
		len(gcJob), len(gcEval), len(gcAlloc))

	// Reap the evals and allocs
	if err := c.evalReap(gcEval, gcAlloc); err != nil {
		return err
	}

	// Call to the leader to deregister the jobs.
	for _, job := range gcJob {
		req := structs.JobDeregisterRequest{
			JobID: job,
			WriteRequest: structs.WriteRequest{
				Region: c.srv.config.Region,
			},
		}
		var resp structs.JobDeregisterResponse
		if err := c.srv.RPC("Job.Deregister", &req, &resp); err != nil {
			c.srv.logger.Printf("[ERR] sched.core: job deregister failed: %v", err)
			return err
		}
	}

	return nil
}

// evalGC is used to garbage collect old evaluations
func (c *CoreScheduler) evalGC(eval *structs.Evaluation) error {
	// Iterate over the evaluations
	iter, err := c.snap.Evals()
	if err != nil {
		return err
	}

	var oldThreshold uint64
	if eval.TriggeredBy == structs.EvalTriggerForceGC {
		// The GC was forced, so set the threshold to its maximum so everything
		// will GC.
		oldThreshold = math.MaxUint64
		c.srv.logger.Println("[DEBUG] sched.core: forced eval GC")
	} else {
		// Compute the old threshold limit for GC using the FSM
		// time table.  This is a rough mapping of a time to the
		// Raft index it belongs to.
		tt := c.srv.fsm.TimeTable()
		cutoff := time.Now().UTC().Add(-1 * c.srv.config.EvalGCThreshold)
		oldThreshold = tt.NearestIndex(cutoff)
	}
	c.srv.logger.Printf("[DEBUG] sched.core: eval GC: scanning before index %d (%v)",
		oldThreshold, c.srv.config.EvalGCThreshold)

	// Collect the allocations and evaluations to GC
	var gcAlloc, gcEval []string
	for raw := iter.Next(); raw != nil; raw = iter.Next() {
		eval := raw.(*structs.Evaluation)
		gc, allocs, err := c.gcEval(eval, oldThreshold)
		if err != nil {
			return err
		}

		if gc {
			gcEval = append(gcEval, eval.ID)
			gcAlloc = append(gcAlloc, allocs...)
		}
	}

	// Fast-path the nothing case
	if len(gcEval) == 0 && len(gcAlloc) == 0 {
		return nil
	}
	c.srv.logger.Printf("[DEBUG] sched.core: eval GC: %d evaluations, %d allocs eligible",
		len(gcEval), len(gcAlloc))

	return c.evalReap(gcEval, gcAlloc)
}

// gcEval returns whether the eval should be garbage collected given a raft
// threshold index. The eval disqualifies for garbage collection if it or its
// allocs are not older than the threshold. If the eval should be garbage
// collected, the associated alloc ids that should also be removed are also
// returned
func (c *CoreScheduler) gcEval(eval *structs.Evaluation, thresholdIndex uint64) (
	bool, []string, error) {
	// Ignore non-terminal and new evaluations
	if !eval.TerminalStatus() || eval.ModifyIndex > thresholdIndex {
		return false, nil, nil
	}

	// Get the allocations by eval
	allocs, err := c.snap.AllocsByEval(eval.ID)
	if err != nil {
		c.srv.logger.Printf("[ERR] sched.core: failed to get allocs for eval %s: %v",
			eval.ID, err)
		return false, nil, err
	}

	// Scan the allocations to ensure they are terminal and old
	for _, alloc := range allocs {
		if !alloc.TerminalStatus() || alloc.ModifyIndex > thresholdIndex {
			return false, nil, nil
		}
	}

	allocIds := make([]string, len(allocs))
	for i, alloc := range allocs {
		allocIds[i] = alloc.ID
	}

	// Evaluation is eligible for garbage collection
	return true, allocIds, nil
}

// evalReap contacts the leader and issues a reap on the passed evals and
// allocs.
func (c *CoreScheduler) evalReap(evals, allocs []string) error {
	// Call to the leader to issue the reap
	req := structs.EvalDeleteRequest{
		Evals:  evals,
		Allocs: allocs,
		WriteRequest: structs.WriteRequest{
			Region: c.srv.config.Region,
		},
	}
	var resp structs.GenericResponse
	if err := c.srv.RPC("Eval.Reap", &req, &resp); err != nil {
		c.srv.logger.Printf("[ERR] sched.core: eval reap failed: %v", err)
		return err
	}

	return nil
}

// nodeGC is used to garbage collect old nodes
func (c *CoreScheduler) nodeGC(eval *structs.Evaluation) error {
	// Iterate over the evaluations
	iter, err := c.snap.Nodes()
	if err != nil {
		return err
	}

	var oldThreshold uint64
	if eval.TriggeredBy == structs.EvalTriggerForceGC {
		// The GC was forced, so set the threshold to its maximum so everything
		// will GC.
		oldThreshold = math.MaxUint64
		c.srv.logger.Println("[DEBUG] sched.core: forced node GC")
	} else {
		// Compute the old threshold limit for GC using the FSM
		// time table.  This is a rough mapping of a time to the
		// Raft index it belongs to.
		tt := c.srv.fsm.TimeTable()
		cutoff := time.Now().UTC().Add(-1 * c.srv.config.NodeGCThreshold)
		oldThreshold = tt.NearestIndex(cutoff)
	}
	c.srv.logger.Printf("[DEBUG] sched.core: node GC: scanning before index %d (%v)",
		oldThreshold, c.srv.config.NodeGCThreshold)

	// Collect the nodes to GC
	var gcNode []string
	for {
		raw := iter.Next()
		if raw == nil {
			break
		}
		node := raw.(*structs.Node)

		// Ignore non-terminal and new nodes
		if !node.TerminalStatus() || node.ModifyIndex > oldThreshold {
			continue
		}

		// Get the allocations by node
		allocs, err := c.snap.AllocsByNode(node.ID)
		if err != nil {
			c.srv.logger.Printf("[ERR] sched.core: failed to get allocs for node %s: %v",
				eval.ID, err)
			continue
		}

		// If there are any allocations, skip the node
		if len(allocs) > 0 {
			continue
		}

		// Node is eligible for garbage collection
		gcNode = append(gcNode, node.ID)
	}

	// Fast-path the nothing case
	if len(gcNode) == 0 {
		return nil
	}
	c.srv.logger.Printf("[DEBUG] sched.core: node GC: %d nodes eligible", len(gcNode))

	// Call to the leader to issue the reap
	for _, nodeID := range gcNode {
		req := structs.NodeDeregisterRequest{
			NodeID: nodeID,
			WriteRequest: structs.WriteRequest{
				Region: c.srv.config.Region,
			},
		}
		var resp structs.NodeUpdateResponse
		if err := c.srv.RPC("Node.Deregister", &req, &resp); err != nil {
			c.srv.logger.Printf("[ERR] sched.core: node '%s' reap failed: %v", nodeID, err)
			return err
		}
	}
	return nil
}
