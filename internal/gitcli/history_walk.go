package gitcli

import (
	"bufio"
	"bytes"
	"container/heap"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Walk returns the first count commits in the legacy History order. Git
// supplies the reachable graph, including the currently visible shallow
// boundaries; Go orders it so Git CLI timestamp ties cannot move graph rows.
func (r *Repository) Walk(ctx context.Context, tips []string, count int) ([]string, error) {
	if len(tips) == 0 || count <= 0 {
		return nil, nil
	}
	for _, oid := range tips {
		if !validOID(oid) {
			return nil, fmt.Errorf("invalid Git history tip")
		}
	}
	cmd := r.command(ctx, strings.Join(tips, "\n")+"\n", "rev-list", "--timestamp", "--parents", "--stdin", "--")
	output, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr gitDiagnostic
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start Git History walk: %w", err)
	}
	scanner := bufio.NewScanner(output)
	// Allow an octopus commit with up to 65,535 SHA-256 parents.
	scanner.Buffer(make([]byte, 16<<10), 5<<20)
	g := commitGraph{byOID: make(map[string]int)}
	err = g.read(ctx, scanner)
	if err != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	if waitErr != nil {
		return nil, fmt.Errorf("git rev-list: %w: %s", waitErr, strings.TrimSpace(stderr.buffer.String()))
	}
	return g.order(ctx, tips, count)
}

type graphCommit struct {
	oid                     string
	stamp                   int64
	firstParent, lastParent int
	children                int
	loaded                  bool
}

type commitGraph struct {
	byOID   map[string]int
	commits []graphCommit
	parents []int
}

func (g *commitGraph) index(oid []byte) int {
	if i, ok := g.byOID[string(oid)]; ok {
		return i
	}
	key := string(oid)
	i := len(g.commits)
	g.commits = append(g.commits, graphCommit{oid: key})
	g.byOID[key] = i
	return i
}

func (g *commitGraph) read(ctx context.Context, scanner *bufio.Scanner) error {
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		timestamp, rest, ok := bytes.Cut(scanner.Bytes(), []byte{' '})
		oid, parents, _ := bytes.Cut(rest, []byte{' '})
		if !ok || !validOIDBytes(oid) {
			return fmt.Errorf("invalid Git history row")
		}
		stamp, err := strconv.ParseInt(string(timestamp), 10, 64)
		i := g.index(oid)
		if err != nil || g.commits[i].loaded {
			return fmt.Errorf("invalid Git history row")
		}
		first := len(g.parents)
		for oid := range bytes.FieldsSeq(parents) {
			if !validOIDBytes(oid) {
				return fmt.Errorf("invalid Git history parent")
			}
			parent := g.index(oid)
			g.parents = append(g.parents, parent)
			g.commits[parent].children++
		}
		node := &g.commits[i]
		node.loaded, node.stamp = true, stamp
		node.firstParent, node.lastParent = first, len(g.parents)
	}
	return scanner.Err()
}

func (g *commitGraph) order(ctx context.Context, tips []string, count int) ([]string, error) {
	for _, node := range g.commits {
		if !node.loaded {
			return nil, fmt.Errorf("incomplete Git commit graph")
		}
	}
	ready := historyQueue{commits: g.commits}
	seen := make(map[int]bool, len(tips))
	// The legacy revwalk prepends each pushed tip. Reachable refs cannot
	// seed the queue until their last child has been emitted.
	for _, oid := range slices.Backward(tips) {
		i, ok := g.byOID[oid]
		if !ok {
			return nil, fmt.Errorf("missing Git history tip")
		}
		if g.commits[i].children == 0 && !seen[i] {
			heap.Push(&ready, i)
			seen[i] = true
		}
	}
	oids := make([]string, 0, min(count, len(g.commits)))
	for ready.Len() > 0 && len(oids) < count {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		node := g.commits[heap.Pop(&ready).(int)]
		for _, i := range g.parents[node.firstParent:node.lastParent] {
			parent := &g.commits[i]
			parent.children--
			if parent.children == 0 {
				heap.Push(&ready, i)
			}
		}
		oids = append(oids, node.oid)
	}
	if len(oids) < min(count, len(g.commits)) {
		return nil, fmt.Errorf("could not order Git commit graph")
	}
	return oids, nil
}

// Keep ties equivalent in the binary heap. A secondary key, stable queue or
// bulk heap initialization changes the legacy drawing across concurrent tips.
type historyQueue struct {
	indices []int
	commits []graphCommit
}

func (q *historyQueue) Len() int      { return len(q.indices) }
func (q *historyQueue) Swap(i, j int) { q.indices[i], q.indices[j] = q.indices[j], q.indices[i] }
func (q *historyQueue) Less(i, j int) bool {
	return q.commits[q.indices[i]].stamp > q.commits[q.indices[j]].stamp
}
func (q *historyQueue) Push(value any) { q.indices = append(q.indices, value.(int)) }
func (q *historyQueue) Pop() any {
	last := len(q.indices) - 1
	value := q.indices[last]
	q.indices = q.indices[:last]
	return value
}
