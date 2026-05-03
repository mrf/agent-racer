package monitor

import (
	"github.com/agent-racer/backend/internal/session"
)

// parseResultSubagents converts a map of SubagentParseResult (from JSONL
// parsing) into a slice of session.SubagentState suitable for merging into
// the session store. The sessionID is inferred from the parent
// SessionState during mergeSubagents.
func parseResultSubagents(parsed map[string]*SubagentParseResult) []session.SubagentState {
	if len(parsed) == 0 {
		return nil
	}
	result := make([]session.SubagentState, 0, len(parsed))
	for _, sub := range parsed {
		s := session.SubagentState{
			ID:              sub.ID,
			ParentToolUseID: sub.ParentToolUseID,
			Slug:            sub.Slug,
			Model:           sub.Model,
			CurrentTool:     sub.LastTool,
			MessageCount:    sub.MessageCount,
			ToolCallCount:   sub.ToolCalls,
			StartedAt:       sub.FirstTime,
			LastActivityAt:  sub.LastTime,
		}

		// Map activity from JSONL string to session.Activity.
		switch sub.LastActivity {
		case "tool_use":
			s.Activity = session.Thinking
		case "waiting":
			s.Activity = session.Waiting
		default:
			if sub.LastTool != "" {
				s.Activity = session.Thinking
			} else {
				s.Activity = session.Idle
			}
		}

		if sub.Completed {
			s.Activity = session.Complete
			t := sub.LastTime
			s.CompletedAt = &t
		}

		result = append(result, s)
	}
	return result
}

// mergeSubagents merges newly parsed subagent states into the session's
// existing Subagents slice. It:
//   - Appends new subagents (setting SessionID from the parent state)
//   - Updates existing subagents in place (tool, activity, timestamps)
//   - Prunes zero-message phantom entries not present in the new set
//   - Retains completed and real (MessageCount > 0) subagents even if
//     absent from the new parsed set
func mergeSubagents(state *session.SessionState, newSubs []session.SubagentState) {
	newByID := make(map[string]*session.SubagentState, len(newSubs))
	for i := range newSubs {
		newSubs[i].SessionID = state.ID
		newByID[newSubs[i].ID] = &newSubs[i]
	}

	// Update existing subagents in place; mark which new ones were matched.
	matched := make(map[string]bool)
	retained := state.Subagents[:0]
	for i := range state.Subagents {
		existing := &state.Subagents[i]
		if updated, ok := newByID[existing.ID]; ok {
			// Update mutable fields.
			existing.CurrentTool = updated.CurrentTool
			existing.Activity = updated.Activity
			existing.LastActivityAt = updated.LastActivityAt
			if updated.CompletedAt != nil {
				existing.CompletedAt = updated.CompletedAt
			}
			if updated.MessageCount > existing.MessageCount {
				existing.MessageCount = updated.MessageCount
			}
			if updated.ToolCallCount > existing.ToolCallCount {
				existing.ToolCallCount = updated.ToolCallCount
			}
			matched[existing.ID] = true
			retained = append(retained, *existing)
		} else {
			// Not in new set — keep if real or completed; prune zero-message phantoms.
			if existing.MessageCount > 0 || existing.Activity == session.Complete {
				retained = append(retained, *existing)
			}
		}
	}

	// Append truly new subagents.
	for i := range newSubs {
		if !matched[newSubs[i].ID] {
			retained = append(retained, newSubs[i])
		}
	}

	state.Subagents = retained
}

