package core

// HarnessCapabilities describes only protocol abilities; routing policy stays
// shared and adapters merely declare what their protocol can carry.
type HarnessCapabilities struct {
	CanRewriteModel bool
	CanApplyEffort  bool
}

type RewritePlan struct {
	Model        Model
	RewriteModel bool
	ApplyEffort  bool
}

// Plan translates one harness-agnostic decision into protocol actions.
func (d Decision) Plan(c HarnessCapabilities) RewritePlan {
	return RewritePlan{
		Model:        d.Model,
		RewriteModel: c.CanRewriteModel && d.ShouldRewriteModel(),
		ApplyEffort:  c.CanApplyEffort && d.ShouldApplyEffort(),
	}
}
