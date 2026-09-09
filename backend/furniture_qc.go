package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type FurnitureQCCheck struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Reason string `json:"reason,omitempty"`
}

type FurnitureQCResult struct {
	Pass         bool               `json:"pass"`
	Score        int                `json:"score"`
	Checks       []FurnitureQCCheck `json:"checks"`
	RepairPrompt string             `json:"repairPrompt,omitempty"`
}

func ParseFurnitureQCJSON(payload []byte) (FurnitureQCResult, error) {
	var result FurnitureQCResult
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("QC JSON 格式不正确: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return result, err
	}
	if result.Score < 0 || result.Score > 100 {
		return result, errors.New("QC score 必须在 0 到 100 之间")
	}
	if len(result.Checks) == 0 {
		return result, errors.New("QC checks 不能为空")
	}
	seen := make(map[string]bool, len(result.Checks))
	for index, check := range result.Checks {
		name := strings.TrimSpace(check.Name)
		if name == "" {
			return result, fmt.Errorf("QC checks[%d].name 不能为空", index)
		}
		if seen[name] {
			return result, fmt.Errorf("QC check %q 重复", name)
		}
		seen[name] = true
		if !check.Pass && strings.TrimSpace(check.Reason) == "" {
			return result, fmt.Errorf("QC check %q 未通过时必须说明原因", name)
		}
	}
	if !result.Pass && strings.TrimSpace(result.RepairPrompt) == "" {
		return result, errors.New("QC 未通过时 repairPrompt 不能为空")
	}
	return result, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("QC JSON 只能包含一个对象")
		}
		return fmt.Errorf("QC JSON 尾部无效: %w", err)
	}
	return nil
}
