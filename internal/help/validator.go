package help

import (
	"strings"

	"github.com/cesareyeserrano/ultron-ap/internal/help/contract"
)

// ValidateLinks walks every rule's links and emits a single structured WARN
// log line for each fragment that does not resolve to a loaded glossary entry.
// External links (http://, https://) are skipped — only fragment-only links
// starting with "#" are checked. Validation never fails, never disables a
// rule, never mutates the input.
//
// @aitri-trace FR-052
func (s *Service) ValidateLinks(rules []contract.RuleLink) {
	for _, r := range rules {
		for _, l := range r.Links {
			if l == "" {
				continue
			}
			if strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") {
				continue
			}
			if !strings.HasPrefix(l, "#") {
				continue
			}
			id := strings.TrimPrefix(l, "#")
			if _, ok := s.byID[id]; ok {
				continue
			}
			s.log("event=insights-link-missing rule_id=%q missing_anchor=%q", r.RuleID, id)
		}
	}
}
