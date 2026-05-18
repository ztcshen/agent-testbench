import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { exportedValues, parseEvidenceBody } from "./workflowDetailModel.mjs";

describe("workflow detail context exports", () => {
  it("keeps long numeric response identifiers as strings", () => {
    const body = "{\"ext_params\":{\"payout_results\":[{\"payout_id\":9161018236071807}]}}";

    assert.deepEqual(parseEvidenceBody(body), {
      ext_params: {
        payout_results: [{ payout_id: "9161018236071807" }],
      },
    });
  });

  it("does not export scientific notation for long payout ids", () => {
    const values = exportedValues({
      exports: [{ from: "responseBody", name: "payout_id", path: "ext_params.payout_results.0.payout_id" }],
    }, {
      result: {
        response: {
          body: "{\"ext_params\":{\"payout_results\":[{\"payout_id\":9161018236071807}]}}",
        },
      },
    });

    assert.deepEqual(values, { payout_id: "9161018236071807" });
  });
});
