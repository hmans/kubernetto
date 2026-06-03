import { beforeEach, describe, expect, it } from "vitest";

import "./app";

describe("fleet card URL sync", () => {
  beforeEach(() => {
    document.body.innerHTML = "";
    window.history.replaceState({}, "", "/");
  });

  it("pushes a cluster overview URL from fleet card clicks", () => {
    document.body.innerHTML = `<button data-fleet-context="prod"></button>`;

    document.querySelector<HTMLButtonElement>("[data-fleet-context]")?.click();

    const params = new URLSearchParams(window.location.search);
    expect(params.get("context")).toBe("prod");
    expect(params.get("clusters")).toBe("prod");
    expect(params.get("resource")).toBe("overview");
  });

  it("pushes a scoped issue URL from fleet issue clicks", () => {
    document.body.innerHTML = `
      <button
        data-fleet-context="prod"
        data-fleet-issue="true"
        data-fleet-issue-kind="events"
        data-fleet-issue-query="BackOff"
      ></button>
    `;

    document.querySelector<HTMLButtonElement>("[data-fleet-issue]")?.click();

    const params = new URLSearchParams(window.location.search);
    expect(params.get("context")).toBe("prod");
    expect(params.get("clusters")).toBe("prod");
    expect(params.get("resource")).toBe("events");
    expect(params.get("query")).toBe("BackOff");
  });
});
