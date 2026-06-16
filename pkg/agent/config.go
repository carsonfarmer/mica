package agent

import (
	"cmp"
	"context"
	"slices"

	acp "github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/mica/pkg/core"
)

// SetSessionConfigOption implements acp.AgentSessionConfigSetter.
func (a *Agent) SetSessionConfigOption(ctx context.Context, req *acp.SetSessionConfigOptionRequest, _ acp.Client) (*acp.SetSessionConfigOptionResponse, error) {
	sess, _, err := a.store.Get(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}

	switch req.ConfigID {
	case string(acp.ConfigCatModel):
		return a.setModelConfigOption(ctx, sess, req.Value)
	case string(acp.ConfigCatThoughtLevel):
		return a.setThoughtLevelConfigOption(ctx, sess, req.Value)
	case string(acp.ConfigCatMode):
		return a.setModeConfigOption(ctx, sess, req.Value)
	default:
		return nil, acp.NewRPCError(acp.ErrInvalidParams, "unknown config option")
	}
}

func (a *Agent) setModelConfigOption(ctx context.Context, sess *core.AgentSession, value string) (*acp.SetSessionConfigOptionResponse, error) {
	mid := core.FullModelID(value)
	if _, ok := a.reg.Config(mid); !ok {
		return nil, acp.NewRPCError(acp.ErrInvalidParams, "unknown model")
	}
	return a.setConfigOption(ctx, sess, func() { sess.Model = mid })
}

func (a *Agent) setThoughtLevelConfigOption(ctx context.Context, sess *core.AgentSession, value string) (*acp.SetSessionConfigOptionResponse, error) {
	cfg, _ := a.reg.Config(sess.Model)
	if !cfg.CanReason || !slices.Contains(cfg.ReasoningLevels, value) {
		return nil, acp.NewRPCError(acp.ErrInvalidParams, "invalid reasoning level")
	}
	return a.setConfigOption(ctx, sess, func() { sess.ThoughtLevel = value })
}

func (a *Agent) setModeConfigOption(ctx context.Context, sess *core.AgentSession, value string) (*acp.SetSessionConfigOptionResponse, error) {
	m := core.Mode(value)
	switch m {
	case core.ModeNormal, core.ModeSafe:
	default:
		return nil, acp.NewRPCError(acp.ErrInvalidParams, "invalid mode")
	}
	return a.setConfigOption(ctx, sess, func() { sess.Mode = m }, acp.UpdateCurrentMode(acp.SessionModeID(m)))
}

// setConfigOption applies a change to the session, persists it,
// broadcasts updated config options (plus any extra updates), and
// returns the new option list.
func (a *Agent) setConfigOption(ctx context.Context, sess *core.AgentSession, apply func(), extra ...acp.SessionUpdate) (*acp.SetSessionConfigOptionResponse, error) {
	apply()
	if err := a.store.Set(ctx, sess.SessionInfo.SessionID, sess); err != nil {
		return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
	}
	opts := a.getSessionConfigOptions(sess)
	for _, upd := range extra {
		a.bc.SessionUpdate(ctx, &acp.SessionNotification{SessionID: sess.SessionInfo.SessionID, Update: upd})
	}
	a.bc.SessionUpdate(ctx, &acp.SessionNotification{SessionID: sess.SessionInfo.SessionID, Update: acp.UpdateConfigOptions(opts...)})
	return &acp.SetSessionConfigOptionResponse{ConfigOptions: opts}, nil
}

func (a *Agent) getSessionConfigOptions(sess *core.AgentSession) []acp.SessionConfigOption {
	cfg, _ := a.reg.Config(sess.Model)
	var opts []acp.SessionConfigOption

	// Model selection — grouped by provider.
	var groups []acp.SessionConfigSelectGroup
	for _, pi := range a.reg.Providers() {
		models := a.reg.Models(pi.ID)
		if len(models) == 0 {
			continue
		}
		var items []acp.SessionConfigSelectOption
		for _, mi := range models {
			items = append(items, acp.SessionConfigSelectOption{
				Value:       acp.SessionConfigValueID(string(mi.ModelID)),
				Name:        mi.Name,
				Description: mi.Description,
			})
		}
		groups = append(groups, acp.SessionConfigSelectGroup{
			Group:   acp.SessionConfigGroupID(pi.ID),
			Name:    pi.ID,
			Options: items,
		})
	}
	if len(groups) > 0 {
		opts = append(opts, acp.SessionConfigOption{
			Select: &acp.SessionConfigSelect{
				Type:         acp.ConfigTypeSelect,
				ID:           string(acp.ConfigCatModel),
				Name:         "Model",
				Category:     acp.ConfigCatModel,
				Description:  "Select the LLM model for this session.",
				CurrentValue: acp.SessionConfigValueID(string(sess.Model)),
				Options:      &acp.SessionConfigSelectOptions{Grouped: groups},
			},
		})
	}

	// Reasoning level — ungrouped, only if the model supports it.
	if cfg.CanReason && len(cfg.ReasoningLevels) > 0 {
		lvl := cmp.Or(sess.ThoughtLevel, cfg.DefaultReasoningEffort, "medium")
		var items []acp.SessionConfigSelectOption
		for _, rl := range cfg.ReasoningLevels {
			items = append(items, acp.SessionConfigSelectOption{
				Value: acp.SessionConfigValueID(rl),
				Name:  rl,
			})
		}
		opts = append(opts, acp.SessionConfigOption{
			Select: &acp.SessionConfigSelect{
				Type:         acp.ConfigTypeSelect,
				ID:           string(acp.ConfigCatThoughtLevel),
				Name:         "Thinking",
				Category:     acp.ConfigCatThoughtLevel,
				Description:  "Controls how much reasoning the model performs.",
				CurrentValue: acp.SessionConfigValueID(lvl),
				Options:      &acp.SessionConfigSelectOptions{Ungrouped: items},
			},
		})
	}

	// Mode — ungrouped, always available.
	{
		cur := cmp.Or(string(sess.Mode), string(core.ModeNormal))
		opts = append(opts, acp.SessionConfigOption{
			Select: &acp.SessionConfigSelect{
				Type:         acp.ConfigTypeSelect,
				ID:           string(acp.ConfigCatMode),
				Name:         "Mode",
				Category:     acp.ConfigCatMode,
				Description:  "Permission mode: normal (no prompts) or safe (ask before writes/exec).",
				CurrentValue: acp.SessionConfigValueID(cur),
				Options: &acp.SessionConfigSelectOptions{Ungrouped: []acp.SessionConfigSelectOption{
					{Value: string(core.ModeNormal), Name: "normal"},
					{Value: string(core.ModeSafe), Name: "safe"},
				}},
			},
		})
	}

	return opts
}

var (
	_ acp.AgentSessionConfigSetter = (*Agent)(nil)
)

