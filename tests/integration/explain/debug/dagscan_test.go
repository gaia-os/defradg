// Copyright 2023 Democratized Data Foundation
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package test_explain_debug

import (
	"testing"

	testUtils "github.com/sourcenetwork/defradb/tests/integration"
	explainUtils "github.com/sourcenetwork/defradb/tests/integration/explain"
)

var dagScanPattern = dataMap{
	"explain": dataMap{
		"selectTopNode": dataMap{
			"selectNode": dataMap{
				"dagScanNode": dataMap{},
			},
		},
	},
}

func TestDebugExplainCommitsDagScanQueryOp(t *testing.T) {
	test := testUtils.TestCase{

		Description: "Explain (debug) commits query-op.",

		Actions: []any{
			explainUtils.SchemaForExplainTests,

			testUtils.ExplainRequest{

				Request: `query @explain(type: debug) {
					commits (dockey: "bae-41598f0c-19bc-5da6-813b-e80f14a10df3", fieldId: "1") {
						links {
							cid
						}
					}
				}`,

				ExpectedFullGraph: []dataMap{dagScanPattern},
			},
		},
	}

	explainUtils.ExecuteTestCase(t, test)
}

func TestDebugExplainCommitsDagScanQueryOpWithoutField(t *testing.T) {
	test := testUtils.TestCase{

		Description: "Explain (debug) commits query-op with only dockey (no field).",

		Actions: []any{
			explainUtils.SchemaForExplainTests,

			testUtils.ExplainRequest{

				Request: `query @explain(type: debug) {
					commits (dockey: "bae-41598f0c-19bc-5da6-813b-e80f14a10df3") {
						links {
							cid
						}
					}
				}`,

				ExpectedFullGraph: []dataMap{dagScanPattern},
			},
		},
	}

	explainUtils.ExecuteTestCase(t, test)
}

func TestDebugExplainLatestCommitsDagScanQueryOp(t *testing.T) {
	test := testUtils.TestCase{

		Description: "Explain (debug) latestCommits query-op.",

		Actions: []any{
			explainUtils.SchemaForExplainTests,

			testUtils.ExplainRequest{

				Request: `query @explain(type: debug) {
					latestCommits(dockey: "bae-41598f0c-19bc-5da6-813b-e80f14a10df3", fieldId: "1") {
						cid
						links {
							cid
						}
					}
				}`,

				ExpectedFullGraph: []dataMap{dagScanPattern},
			},
		},
	}

	explainUtils.ExecuteTestCase(t, test)
}

func TestDebugExplainLatestCommitsDagScanQueryOpWithoutField(t *testing.T) {
	test := testUtils.TestCase{

		Description: "Explain (debug) latestCommits query-op with only dockey (no field).",

		Actions: []any{
			explainUtils.SchemaForExplainTests,

			testUtils.ExplainRequest{

				Request: `query @explain(type: debug) {
					latestCommits(dockey: "bae-41598f0c-19bc-5da6-813b-e80f14a10df3") {
						cid
						links {
							cid
						}
					}
				}`,

				ExpectedFullGraph: []dataMap{dagScanPattern},
			},
		},
	}

	explainUtils.ExecuteTestCase(t, test)
}

func TestDebugExplainLatestCommitsDagScanWithoutDocKey_Failure(t *testing.T) {
	test := testUtils.TestCase{

		Description: "Explain (debug) latestCommits query without DocKey.",

		Actions: []any{
			explainUtils.SchemaForExplainTests,

			testUtils.ExplainRequest{

				Request: `query @explain(type: debug) {
					latestCommits(fieldId: "1") {
						cid
						links {
							cid
						}
					}
				}`,

				ExpectedError: "Field \"latestCommits\" argument \"dockey\" of type \"ID!\" is required but not provided.",
			},
		},
	}

	explainUtils.ExecuteTestCase(t, test)
}

func TestDebugExplainLatestCommitsDagScanWithoutAnyArguments_Failure(t *testing.T) {
	test := testUtils.TestCase{

		Description: "Explain (debug) latestCommits query without any arguments.",

		Actions: []any{
			explainUtils.SchemaForExplainTests,

			testUtils.ExplainRequest{

				Request: `query @explain(type: debug) {
					latestCommits {
						cid
						links {
							cid
						}
					}
				}`,

				ExpectedError: "Field \"latestCommits\" argument \"dockey\" of type \"ID!\" is required but not provided.",
			},
		},
	}

	explainUtils.ExecuteTestCase(t, test)
}
