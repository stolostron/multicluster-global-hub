// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"encoding/base64"
	"encoding/json"
)

// continueToken is a simple structured object for encoding the state of a continue token
// since the resource name may not be unique, resource uid is combined to check last returned resource
type continueToken struct {
	LastName string `json:"lastName"`
	LastUID  string `json:"lastUID"`
}

// DecodeContinue decodes the continue token and get last resoource name and uid
func DecodeContinue(continueStr string) (string, string, error) {
	lastName, lastUID := "", ""
	decodedContinue, err := base64.RawURLEncoding.DecodeString(continueStr)
	if err != nil {
		return lastName, lastUID, err
	}

	ct := &continueToken{}
	if err := json.Unmarshal(decodedContinue, ct); err != nil {
		return lastName, lastUID, err
	}

	return ct.LastName, ct.LastUID, nil
}

// EncodeContinue encodes the continue token with last resource name and uid
func EncodeContinue(lastName, lastUID string) (string, error) {
	ct, err := json.Marshal(&continueToken{LastName: lastName, LastUID: lastUID})
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(ct), nil
}
