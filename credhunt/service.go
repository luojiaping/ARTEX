package credhunt

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	pgdb "github.com/Autumn-27/artex/db"
)

// Service is the single entry point used by agent tools and HTTP handlers.
type Service struct {
	Store    *Store
	Fetcher  *Fetcher
	Prober   *Prober
	Verifier *Verifier
}

// NewService wires the ledger, fetcher, verifier and mihomo sidecar.
func NewService(db *pgdb.DB, dataDir string) (*Service, error) {
	store, err := NewStore(db, filepath.Join(dataDir, "credhunt.key"))
	if err != nil {
		return nil, err
	}
	svc := &Service{Store: store, Fetcher: NewFetcher(dataDir), Prober: NewProber(dataDir)}
	svc.Verifier = NewVerifier(store)
	return svc, nil
}

// Submit stores normal credentials.
func (s *Service) Submit(ctx context.Context, inputs []Input) (SubmitResult, error) {
	return s.Store.Submit(ctx, inputs)
}

// Reload lets the fetcher be tuned from settings (not used yet; kept explicit).
func (s *Service) Reload(allowPrivate bool) {
	s.Fetcher.AllowPrivate = allowPrivate
}

// Verify checks explicit UIDs.
func (s *Service) Verify(ctx context.Context, uids []string) []VerifyResult {
	entries, secrets, err := s.Store.ByUIDs(ctx, uids)
	if err != nil || len(entries) == 0 {
		return nil
	}
	return s.Verifier.Verify(ctx, entries, secrets)
}

// VerifyFilter checks up to limit rows matching a filter.
func (s *Service) VerifyFilter(ctx context.Context, f Filter, limit int) []VerifyResult {
	entries, secrets, err := s.Store.SelectForVerify(ctx, f, limit)
	if err != nil || len(entries) == 0 {
		return nil
	}
	return s.Verifier.Verify(ctx, entries, secrets)
}

// SubmitProxies accepts share links, Clash YAML, base64 subscriptions and
// subscription URLs. It stores the parent (if any) and every expanded node.
func (s *Service) SubmitProxies(ctx context.Context, inputs []Input, expand bool) (SubmitResult, error) {
	var total SubmitResult
	for _, in := range inputs {
		value := strings.TrimSpace(in.Value)
		if value == "" {
			total.Skipped++
			continue
		}
		parentInput := in
		parentInput.Kind = KindProxySubscription
		parentInput.Package = nil
		content := value
		evidence := in.EvidencePath
		runID := "proxies"
		if in.TaskID > 0 {
			runID = fmt.Sprintf("t%d", in.TaskID)
		}

		isSub := looksLikeSubscription(value)
		if isSub && (strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")) {
			res := s.Fetcher.Fetch(ctx, value, runID)
			if res.Error != "" {
				total.Skipped++
				total.Errors = append(total.Errors, res.Error)
				continue
			}
			content = string(res.Body)
			if evidence == "" {
				evidence = res.EvidencePath
			}
		}
		parsed := ParseProxyContent(content)
		if len(parsed) == 0 {
			// A single unsupported link is still worth recording as a raw node.
			parentInput.Kind = KindProxyNode
			parentInput.EvidencePath = evidence
			res, _ := s.Store.Submit(ctx, []Input{parentInput})
			mergeSubmit(&total, res)
			continue
		}
		parentUID := ""
		if isSub || len(parsed) > 1 {
			parentInput.EvidencePath = evidence
			parentInput.Value = value
			res, _ := s.Store.Submit(ctx, []Input{parentInput})
			mergeSubmit(&total, res)
			if len(res.Items) > 0 {
				parentUID = res.Items[0].UID
			}
		}
		for _, node := range parsed {
			cfg, _ := json.Marshal(node.Config)
			child := Input{
				Kind:           KindProxyNode,
				Value:          string(cfg),
				Provider:       "proxy",
				Service:        firstNonEmpty(node.Remark, node.Protocol+"://"+node.Server),
				SourceURL:      firstNonEmpty(in.SourceURL, value),
				EvidencePath:   evidence,
				Risk:           "P1",
				Context:        in.Context,
				TaskID:         in.TaskID,
				ConversationID: in.ConversationID,
				ParentUID:      parentUID,
			}
			res, err := s.Store.Submit(ctx, []Input{child})
			if err != nil {
				total.Errors = append(total.Errors, err.Error())
				continue
			}
			mergeSubmit(&total, res)
			for _, item := range res.Items {
				if item.UID != "" {
					_ = s.Store.UpsertProxyNode(ctx, item.UID, parentUID, node.Protocol, node.Server, node.Port, StatusUnverified, "")
				}
			}
		}
	}
	return total, nil
}

func mergeSubmit(dst *SubmitResult, src SubmitResult) {
	dst.Inserted += src.Inserted
	dst.Duplicate += src.Duplicate
	dst.Excluded += src.Excluded
	dst.Skipped += src.Skipped
	dst.Items = append(dst.Items, src.Items...)
	dst.Errors = append(dst.Errors, src.Errors...)
}

func looksLikeSubscription(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	if strings.Contains(v, "proxies:") {
		return true
	}
	if len(v) > 64 && !strings.Contains(v, "://") && !strings.Contains(v, "\n") && isBase64ish(v) {
		if decoded, err := decodeB64(v); err == nil {
			d := strings.ToLower(string(decoded))
			return strings.Contains(d, "://") || strings.Contains(d, "proxies:")
		}
	}
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		if strings.Contains(v, "@") && !strings.Contains(v, "/sub") && !strings.Contains(v, "subscribe") &&
			!strings.Contains(v, "clash") && !strings.Contains(v, "node") && !strings.Contains(v, ".yaml") &&
			!strings.Contains(v, ".yml") && !strings.Contains(v, ".txt") {
			return false // single http proxy node, not a subscription URL
		}
		return strings.Contains(v, "sub") || strings.Contains(v, "clash") || strings.Contains(v, "yaml") ||
			strings.Contains(v, "node") || strings.Contains(v, "proxy") || strings.HasSuffix(v, ".txt") ||
			strings.HasSuffix(v, "/")
	}
	return false
}

// Probe proxies by UID (or latest rows when uids is empty).
func (s *Service) Probe(ctx context.Context, uids []string, limit int) []ProbeResult {
	entries, _, configs, err := s.Store.ProxyRows(ctx, uids, limit)
	if err != nil {
		return nil
	}
	items := make([]ProbeItem, 0, len(entries))
	for i, e := range entries {
		if i >= len(configs) || len(configs[i]) == 0 {
			continue
		}
		items = append(items, ProbeItem{UID: e.UID, Config: configs[i]})
	}
	results := s.Prober.Probe(ctx, items)
	for _, r := range results {
		status := r.Status
		switch status {
		case "alive":
			status = StatusValid
		case "dead":
			status = StatusInvalid
		}
		_ = s.Store.UpdateProxyProbe(ctx, r.UID, status, r.DelayMS, r.Message)
	}
	return results
}

// Fetch is a thin wrapper for tools.
func (s *Service) Fetch(ctx context.Context, rawURL, runID string) FetchResult {
	return s.Fetcher.Fetch(ctx, rawURL, runID)
}

// Batch scans a list of URLs (directory indexes) and ranks candidate files.
func (s *Service) Batch(ctx context.Context, urls []string, maxEntries int, note string, taskID int64) (int64, []BatchTarget) {
	if maxEntries <= 0 || maxEntries > 200 {
		maxEntries = 60
	}
	runID := fmt.Sprintf("%d", time.Now().Unix())
	if taskID > 0 {
		runID = fmt.Sprintf("t%d-%d", taskID, time.Now().Unix())
	}
	var targets []BatchTarget
	for _, raw := range urls {
		res := s.Fetch(ctx, raw, runID)
		t := BatchTarget{URL: raw, Status: res.Status, Title: res.Title, IsListing: res.IsListing,
			EvidencePath: res.EvidencePath, Error: res.Error}
		t.DirCount, t.FileCount = len(res.Dirs), len(res.Files)
		if len(res.Candidates) > maxEntries {
			t.Candidates = res.Candidates[:maxEntries]
		} else {
			t.Candidates = res.Candidates
		}
		targets = append(targets, t)
	}
	stats := map[string]any{"targets": len(urls), "listing": countListings(targets)}
	id, _ := s.Store.RecordRun(ctx, taskID, note, stats)
	return id, targets
}

func countListings(targets []BatchTarget) int {
	n := 0
	for _, t := range targets {
		if t.IsListing {
			n++
		}
	}
	return n
}
