package agent

import (
	"cmp"
	"context"
	"slices"

	acp "github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/mica/pkg/llm"
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
	default:
		return nil, acp.NewRPCError(acp.ErrInvalidParams, "unknown config option")
	}
}

func (a *Agent) setModelConfigOption(ctx context.Context, sess *AgentSession, value string) (*acp.SetSessionConfigOptionResponse, error) {
	mid, err := llm.ParseFullModelID(value)
	if err != nil {
		return nil, acp.NewRPCError(acp.ErrInvalidParams, err.Error())
	}
	if _, ok := a.reg.Config(mid); !ok {
		return nil, acp.NewRPCError(acp.ErrInvalidParams, "unknown model")
	}
	sess.Model = mid
	if err := a.store.Set(ctx, sess.SessionInfo.SessionID, sess); err != nil {
		return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
	}
	return &acp.SetSessionConfigOptionResponse{
		ConfigOptions: a.getSessionConfigOptions(sess),
	}, nil
}

func (a *Agent) setThoughtLevelConfigOption(ctx context.Context, sess *AgentSession, value string) (*acp.SetSessionConfigOptionResponse, error) {
	cfg, _ := a.reg.Config(sess.Model)
	if !cfg.CanReason || !slices.Contains(cfg.ReasoningLevels, value) {
		return nil, acp.NewRPCError(acp.ErrInvalidParams, "invalid reasoning level")
	}
	sess.ThoughtLevel = value
	if err := a.store.Set(ctx, sess.SessionInfo.SessionID, sess); err != nil {
		return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
	}
	return &acp.SetSessionConfigOptionResponse{
		ConfigOptions: a.getSessionConfigOptions(sess),
	}, nil
}

func (a *Agent) getSessionConfigOptions(sess *AgentSession) []acp.SessionConfigOption {
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
				CurrentValue: acp.SessionConfigValueID(sess.Model.String()),
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
				Name:         "Reasoning",
				Category:     acp.ConfigCatThoughtLevel,
				Description:  "Controls how much reasoning the model performs.",
				CurrentValue: acp.SessionConfigValueID(lvl),
				Options:      &acp.SessionConfigSelectOptions{Ungrouped: items},
			},
		})
	}

	return opts
}

var (
	_ acp.AgentSessionConfigSetter = (*Agent)(nil)
)
