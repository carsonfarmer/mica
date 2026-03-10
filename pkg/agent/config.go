package agent

import (
	acp "github.com/coder/acp-go-sdk"

	"github.com/carsonfarmer/mica/internal/app"
	"github.com/carsonfarmer/mica/pkg/session"
)

const (
	responseFormatConfigID acp.SessionConfigId      = "response_format"
	modeConfigID           acp.SessionConfigId      = "mode"
	modelConfigID          acp.SessionConfigId      = "model"
	defaultModeID          acp.SessionModeId        = app.DefaultSessionModeID
	defaultResponseFormat  acp.SessionConfigValueId = app.DefaultResponseFormatID
	rawResponseFormat      acp.SessionConfigValueId = app.RawResponseFormatID
	defaultModelID         acp.ModelId              = app.DefaultModelID
)

type configState struct {
	ResponseFormat acp.SessionConfigValueId
	ModeID         acp.SessionModeId
	ModelID        acp.ModelId
}

func defaultConfigState() configState {
	return configState{
		ResponseFormat: defaultResponseFormat,
		ModeID:         defaultModeID,
		ModelID:        defaultModelID,
	}
}

func configStateFromLog(log *session.Log) configState {
	state := defaultConfigState()
	for _, option := range log.ConfigOptions() {
		if option.Select == nil {
			continue
		}
		switch option.Select.Id {
		case responseFormatConfigID:
			state.ResponseFormat = option.Select.CurrentValue
		case modeConfigID:
			state.ModeID = acp.SessionModeId(option.Select.CurrentValue)
		case modelConfigID:
			state.ModelID = acp.ModelId(option.Select.CurrentValue)
		}
	}
	if modes := log.Modes(); modes != nil && modes.CurrentModeId != "" {
		state.ModeID = modes.CurrentModeId
	}
	return state
}

func applyConfigRequest(state configState, req acp.SetSessionConfigOptionRequest) (configState, bool, error) {
	switch req.ConfigId {
	case responseFormatConfigID:
		if req.Value != defaultResponseFormat && req.Value != rawResponseFormat {
			return configState{}, false, acp.NewInvalidParams(map[string]string{"value": string(req.Value)})
		}
		state.ResponseFormat = req.Value
		return state, false, nil
	case modeConfigID:
		if acp.SessionModeId(req.Value) != defaultModeID {
			return configState{}, false, acp.NewInvalidParams(map[string]string{"value": string(req.Value)})
		}
		state.ModeID = defaultModeID
		return state, true, nil
	case modelConfigID:
		if acp.ModelId(req.Value) != defaultModelID {
			return configState{}, false, acp.NewInvalidParams(map[string]string{"value": string(req.Value)})
		}
		state.ModelID = defaultModelID
		return state, false, nil
	default:
		return configState{}, false, acp.NewInvalidParams(map[string]string{"configId": string(req.ConfigId)})
	}
}

func (s configState) formatResponse(prompt string) string {
	if s.ResponseFormat == rawResponseFormat {
		return prompt
	}
	return "Echo: " + prompt
}

func configOptions(state configState) []acp.SessionConfigOption {
	return []acp.SessionConfigOption{
		selectOption(responseFormatConfigID, "Response Format", state.ResponseFormat,
			optionValue(defaultResponseFormat, "Echo"),
			optionValue(rawResponseFormat, "Raw"),
		),
		selectOption(modeConfigID, "Mode", acp.SessionConfigValueId(state.ModeID),
			optionValue(acp.SessionConfigValueId(defaultModeID), app.DefaultSessionModeName),
		),
		selectOption(modelConfigID, "Model", acp.SessionConfigValueId(state.ModelID),
			optionValue(acp.SessionConfigValueId(defaultModelID), app.DefaultModelName),
		),
	}
}

func selectOption(id acp.SessionConfigId, name string, current acp.SessionConfigValueId, values ...acp.SessionConfigSelectOption) acp.SessionConfigOption {
	ungrouped := acp.SessionConfigSelectOptionsUngrouped(values)
	return acp.SessionConfigOption{
		Select: &acp.SessionConfigOptionSelect{
			Id:           id,
			Name:         name,
			CurrentValue: current,
			Options: acp.SessionConfigSelectOptions{
				Ungrouped: &ungrouped,
			},
			Type: "select",
		},
	}
}

func optionValue(value acp.SessionConfigValueId, name string) acp.SessionConfigSelectOption {
	return acp.SessionConfigSelectOption{
		Value: value,
		Name:  name,
	}
}

func modeState(modeID acp.SessionModeId) *acp.SessionModeState {
	return &acp.SessionModeState{
		AvailableModes: []acp.SessionMode{{
			Id:   defaultModeID,
			Name: app.DefaultSessionModeName,
		}},
		CurrentModeId: modeID,
	}
}

func stableModelState(modelID acp.ModelId) *acp.SessionModelState {
	return &acp.SessionModelState{
		AvailableModels: []acp.ModelInfo{{
			ModelId: defaultModelID,
			Name:    app.DefaultModelName,
		}},
		CurrentModelId: modelID,
	}
}

func unstableModelState(modelID acp.ModelId) *acp.UnstableSessionModelState {
	return &acp.UnstableSessionModelState{
		AvailableModels: []acp.UnstableModelInfo{{
			ModelId: acp.UnstableModelId(defaultModelID),
			Name:    app.DefaultModelName,
		}},
		CurrentModelId: acp.UnstableModelId(modelID),
	}
}
