import { expect, it } from "vitest";
import { snapshotSchema } from "@/lib/connections-schema";
import { syntheticSnapshot } from "./fixtures/binance";

it("preserves int64 IDs, decimals and explicit empty order arrays", () => {
  const input = syntheticSnapshot();
  expect(snapshotSchema.parse(input).openOrders[0].orderId).toBe(
    "9007199254740993",
  );
  expect(
    snapshotSchema.parse({ ...input, openOrders: [], algoOrders: [] })
      .openOrders,
  ).toEqual([]);
});
it.each([
  "openOrders",
  "algoOrders",
  "instruments",
  "startedAt",
  "capturedAtMs",
])("rejects missing snapshot component %s", (field) => {
  const input: Record<string, unknown> = { ...syntheticSnapshot() };
  delete input[field];
  expect(snapshotSchema.safeParse(input).success).toBe(false);
});
it.each([9007199254740992, "9223372036854775808", "NaN", "1.5", "-1"])(
  "rejects unsafe order ID %s without throwing",
  (orderId) => {
    const input = syntheticSnapshot();
    expect(
      snapshotSchema.safeParse({
        ...input,
        openOrders: [{ ...input.openOrders[0], orderId }],
      }).success,
    ).toBe(false);
  },
);
it("rejects envelopes, incomplete flags and terminal algo states", () => {
  const input = syntheticSnapshot();
  expect(snapshotSchema.safeParse({ ...input, envelope: {} }).success).toBe(
    false,
  );
  expect(
    snapshotSchema.safeParse({
      ...input,
      algoOrders: [{ ...input.algoOrders[0], closePosition: null }],
    }).success,
  ).toBe(false);
  expect(
    snapshotSchema.safeParse({
      ...input,
      algoOrders: [{ ...input.algoOrders[0], algoStatus: "FINISHED" }],
    }).success,
  ).toBe(false);
  expect(
    snapshotSchema.safeParse({ ...input, startedAt: "2099-01-01T00:00:00Z" })
      .success,
  ).toBe(false);
});
