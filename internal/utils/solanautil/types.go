package solanautil

import (
	"encoding/json"
)

type MetadataPointerState struct {
	Authority       string `json:"authority"`
	MetadataAddress string `json:"metadataAddress"`
}

type TokenMetadataState struct {
	AdditionalMetadata []interface{} `json:"additionalMetadata"`
	Mint               string        `json:"mint"`
	Name               string        `json:"name"`
	Symbol             string        `json:"symbol"`
	UpdateAuthority    string        `json:"updateAuthority"`
	URI                string        `json:"uri"`
}

type Extension struct {
	Extension string          `json:"extension"`
	State     json.RawMessage `json:"state"`
}

type Info struct {
	Decimals        int         `json:"decimals"`
	Extensions      []Extension `json:"extensions"`
	FreezeAuthority *string     `json:"freezeAuthority"`
	IsInitialized   bool        `json:"isInitialized"`
	MintAuthority   *string     `json:"mintAuthority"`
	Supply          string      `json:"supply"`
}

type Parsed struct {
	Info Info   `json:"info"`
	Type string `json:"type"`
}

type TokenData struct {
	Parsed  Parsed `json:"parsed"`
	Program string `json:"program"`
	Space   int    `json:"space"`
}

func parseExtensionState(ext Extension) (interface{}, error) {
	switch ext.Extension {
	case "metadataPointer":
		var state MetadataPointerState
		err := json.Unmarshal(ext.State, &state)
		return state, err
	case "tokenMetadata":
		var state TokenMetadataState
		err := json.Unmarshal(ext.State, &state)
		return state, err
	default:
		return nil, nil
	}
}
