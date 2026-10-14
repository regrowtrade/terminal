import { expect, it } from "vitest";
import { boundedText } from "@/lib/request";
it("reads a bounded request body", async () => {
  expect(
    await boundedText(
      new Request("http://localhost", { method: "POST", body: "hello" }),
      5,
    ),
  ).toBe("hello");
});
it("rejects an oversized body without trusting Content-Length", async () => {
  await expect(
    boundedText(
      new Request("http://localhost", { method: "POST", body: "too large" }),
      3,
    ),
  ).rejects.toThrow();
});
it("rejects a missing body", async () => {
  await expect(boundedText(new Request("http://localhost"))).rejects.toThrow();
});
