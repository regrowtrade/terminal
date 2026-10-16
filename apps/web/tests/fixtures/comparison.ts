import type { StoredComparison } from "@/lib/observation-schema";

export function syntheticComparison(): StoredComparison {
  return {
    storedAt: "2020-01-01T00:01:01Z",
    report: {
      schemaVersion: 1,
      mode: "rest_observation_comparison",
      status: "compared",
      previousCapturedAt: "2020-01-01T00:00:01Z",
      currentStartedAt: "2020-01-01T00:00:59Z",
      currentCapturedAt: "2020-01-01T00:01:00Z",
      intervalRelation: "disjoint",
      continuity: "not_established",
      executionReady: false,
      totalChanges: 2,
      detailLimit: 200,
      truncated: false,
      changes: [
        {
          entity: "position",
          key: "BTCUSDT:LONG",
          kind: "changed",
          fields: [
            {
              field: "positionAmt",
              before: "1.000",
              after: "1.000000000000000001",
            },
          ],
        },
        {
          entity: "order",
          key: "BTCUSDT:9007199254740993",
          kind: "not_observed",
          fields: [{ field: "status", before: "NEW", after: null }],
        },
      ],
    },
  };
}
