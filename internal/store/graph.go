package store

import (
	"fmt"
	"time"
)

// GraphNode is one stored relation graph node (R6). Attrs is a JSON object.
type GraphNode struct {
	ID, Kind, Label, Vendor, Attrs, Source string
	FirstSeen, LastSeen                    time.Time
}

// GraphEdge is one stored relation graph edge (R6).
type GraphEdge struct {
	From, To, Kind, Basis, Source string
	FirstSeen, LastSeen           time.Time
}

// Key matches graph.Edge.Key.
func (e GraphEdge) Key() string { return e.From + "|" + e.Kind + "|" + e.To }

func sec(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// RecordGraph upserts nodes and edges seen at at: a new row gets at as its
// first and last time, a known row only moves its last time. Returns every
// recorded edge's first time, keyed on GraphEdge.Key.
func (s *Store) RecordGraph(nodes []GraphNode, edges []GraphEdge, at time.Time) (map[string]time.Time, error) {
	now := sec(at)
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	nst, err := tx.Prepare(`INSERT INTO graph_nodes (id, kind, label, vendor, attrs, source, first_seen, last_seen)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET kind = excluded.kind, label = excluded.label, vendor = excluded.vendor,
	attrs = excluded.attrs, source = excluded.source, last_seen = excluded.last_seen`)
	if err != nil {
		return nil, err
	}
	defer nst.Close()
	for _, n := range nodes {
		attrs := n.Attrs
		if attrs == "" {
			attrs = "{}"
		}
		if _, err := nst.Exec(n.ID, n.Kind, n.Label, n.Vendor, attrs, n.Source, now, now); err != nil {
			return nil, fmt.Errorf("store: record node %s: %w", n.ID, err)
		}
	}
	est, err := tx.Prepare(`INSERT INTO graph_edges (from_id, to_id, kind, basis, source, first_seen, last_seen)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (from_id, to_id, kind) DO UPDATE SET basis = excluded.basis, source = excluded.source, last_seen = excluded.last_seen
RETURNING first_seen`)
	if err != nil {
		return nil, err
	}
	defer est.Close()
	first := make(map[string]time.Time, len(edges))
	for _, e := range edges {
		var fs string
		if err := est.QueryRow(e.From, e.To, e.Kind, e.Basis, e.Source, now, now).Scan(&fs); err != nil {
			return nil, fmt.Errorf("store: record edge %s: %w", e.Key(), err)
		}
		t, _ := time.Parse(time.RFC3339, fs)
		first[e.Key()] = t
	}
	return first, tx.Commit()
}

// GraphBetween returns the stored edges seen at any time in [from, to] and
// every node they touch.
func (s *Store) GraphBetween(from, to time.Time) ([]GraphNode, []GraphEdge, error) {
	rows, err := s.readDB.Query(`SELECT from_id, to_id, kind, basis, source, first_seen, last_seen
FROM graph_edges WHERE last_seen >= ? AND first_seen <= ?`, sec(from), sec(to))
	if err != nil {
		return nil, nil, err
	}
	edges := []GraphEdge{}
	ids := map[string]bool{}
	for rows.Next() {
		var e GraphEdge
		var fs, ls string
		if err := rows.Scan(&e.From, &e.To, &e.Kind, &e.Basis, &e.Source, &fs, &ls); err != nil {
			rows.Close()
			return nil, nil, err
		}
		e.FirstSeen, _ = time.Parse(time.RFC3339, fs)
		e.LastSeen, _ = time.Parse(time.RFC3339, ls)
		edges = append(edges, e)
		ids[e.From], ids[e.To] = true, true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()

	nodes := []GraphNode{}
	if len(ids) == 0 {
		return nodes, edges, nil
	}
	nrows, err := s.readDB.Query(`SELECT id, kind, label, vendor, attrs, source, first_seen, last_seen FROM graph_nodes WHERE last_seen >= ? AND first_seen <= ?`, sec(from), sec(to))
	if err != nil {
		return nil, nil, err
	}
	defer nrows.Close()
	for nrows.Next() {
		var n GraphNode
		var fs, ls string
		if err := nrows.Scan(&n.ID, &n.Kind, &n.Label, &n.Vendor, &n.Attrs, &n.Source, &fs, &ls); err != nil {
			return nil, nil, err
		}
		if !ids[n.ID] {
			continue
		}
		n.FirstSeen, _ = time.Parse(time.RFC3339, fs)
		n.LastSeen, _ = time.Parse(time.RFC3339, ls)
		nodes = append(nodes, n)
	}
	return nodes, edges, nrows.Err()
}

// HistorySession is one session as the events table remembers it.
type HistorySession struct {
	Vendor, Agent, SessionID, ParentID, Project string
	Last                                        time.Time
}

// SessionsBetween returns one row per session with a turn in [from, to]:
// its parent, project and last turn. It reads the events table, so it
// outlives a deleted transcript.
func (s *Store) SessionsBetween(from, to time.Time) ([]HistorySession, error) {
	rows, err := s.readDB.Query(`SELECT vendor, MAX(agent), session_id, MAX(parent_id), MAX(project), MAX(at)
FROM events WHERE at >= ? AND at <= ? GROUP BY vendor, session_id`,
		from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HistorySession{}
	for rows.Next() {
		var h HistorySession
		var last string
		if err := rows.Scan(&h.Vendor, &h.Agent, &h.SessionID, &h.ParentID, &h.Project, &last); err != nil {
			return nil, err
		}
		h.Last, _ = time.Parse(time.RFC3339Nano, last)
		out = append(out, h)
	}
	return out, rows.Err()
}
