package main

import "testing"

func TestParseFurnitureQCJSON(t *testing.T) {
	payload := []byte(`{
		"pass": false,
		"score": 82,
		"checks": [
			{"name":"glass_black_frame","pass":false,"reason":"增加了黑色玻璃边框"},
			{"name":"caster_count","pass":true}
		],
		"repairPrompt":"去除新增黑色玻璃边框"
	}`)
	result, err := ParseFurnitureQCJSON(payload)
	if err != nil {
		t.Fatal(err)
	}
	if result.Pass || result.Score != 82 || len(result.Checks) != 2 {
		t.Fatalf("unexpected QC result: %#v", result)
	}
}

func TestParseFurnitureQCJSONRejectsInvalidOutput(t *testing.T) {
	cases := [][]byte{
		[]byte(`{"pass":true,"score":101,"checks":[{"name":"caster_count","pass":true}]}`),
		[]byte(`{"pass":false,"score":60,"checks":[{"name":"caster_count","pass":false}],"repairPrompt":"修复"}`),
		[]byte(`{"pass":false,"score":60,"checks":[{"name":"caster_count","pass":false,"reason":"数量错误"}]}`),
		[]byte(`{"pass":true,"score":90,"checks":[],"unexpected":true}`),
	}
	for index, payload := range cases {
		if _, err := ParseFurnitureQCJSON(payload); err == nil {
			t.Fatalf("case %d should fail", index)
		}
	}
}
