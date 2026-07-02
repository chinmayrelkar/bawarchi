package parser

import "testing"

// TestFinalize_DedupesParamNamesAcrossCategories verifies that when a path
// parameter and a body parameter share the same derived Go name (a real
// pattern: an "id" in the URL echoed as an "id" field in the request body,
// as seen in Opsgenie's spec), finalize renames the later one so the
// generated code doesn't declare the same Go variable/flag twice.
func TestFinalize_DedupesParamNamesAcrossCategories(t *testing.T) {
	cli := &CLIData{
		Commands: []CommandData{
			{
				Name: "integration",
				Operations: []OperationData{
					{
						Name: "updateintegration",
						PathParams: []ParamData{
							{Name: "id", GoVarName: "Id", FlagName: "id"},
						},
						BodyParams: []ParamData{
							{Name: "id", GoVarName: "Id", FlagName: "id"},
						},
					},
				},
			},
		},
	}
	cli.finalize()

	op := cli.Commands[0].Operations[0]
	pathVar := op.PathParams[0].GoVarName
	bodyVar := op.BodyParams[0].GoVarName
	if pathVar == bodyVar {
		t.Fatalf("GoVarName collision not resolved: both are %q", pathVar)
	}
	if op.PathParams[0].FlagName == op.BodyParams[0].FlagName {
		t.Fatalf("FlagName collision not resolved: both are %q", op.PathParams[0].FlagName)
	}
	if pathVar != "Id" {
		t.Errorf("first occurrence should keep original GoVarName, got %q", pathVar)
	}
}

// TestFinalize_NoCollision_LeavesNamesUnchanged verifies dedup is a no-op
// when param names don't collide across categories.
func TestFinalize_NoCollision_LeavesNamesUnchanged(t *testing.T) {
	cli := &CLIData{
		Commands: []CommandData{
			{
				Operations: []OperationData{
					{
						PathParams:  []ParamData{{Name: "id", GoVarName: "Id", FlagName: "id"}},
						QueryParams: []ParamData{{Name: "limit", GoVarName: "Limit", FlagName: "limit"}},
					},
				},
			},
		},
	}
	cli.finalize()

	op := cli.Commands[0].Operations[0]
	if op.PathParams[0].GoVarName != "Id" || op.PathParams[0].FlagName != "id" {
		t.Errorf("unrelated path param mutated: %+v", op.PathParams[0])
	}
	if op.QueryParams[0].GoVarName != "Limit" || op.QueryParams[0].FlagName != "limit" {
		t.Errorf("unrelated query param mutated: %+v", op.QueryParams[0])
	}
}
