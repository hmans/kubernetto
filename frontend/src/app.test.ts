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

describe("action item URL sync", () => {
  beforeEach(() => {
    document.body.innerHTML = "";
    window.history.replaceState({}, "", "/?resource=actions");
  });

  it("pushes a selected event URL from issue clicks", () => {
    document.body.innerHTML = `
      <article
        tabindex="0"
        data-action-context="prod"
        data-action-kind="events"
        data-action-query="BackOff"
        data-action-selected-name="worker-event"
        data-action-selected-namespace="default"
      ></article>
    `;

    document.querySelector<HTMLElement>("[data-action-context]")?.click();

    const params = new URLSearchParams(window.location.search);
    expect(params.get("context")).toBe("prod");
    expect(params.get("clusters")).toBe("prod");
    expect(params.get("resource")).toBe("events");
    expect(params.get("query")).toBe("BackOff");
    expect(params.get("selectedName")).toBe("worker-event");
    expect(params.get("selectedNamespace")).toBe("default");
  });

  it("pushes the same selected event URL from Enter", () => {
    document.body.innerHTML = `
      <article
        tabindex="0"
        data-action-context="prod"
        data-action-kind="events"
        data-action-query="BackOff"
        data-action-selected-name="worker-event"
        data-action-selected-namespace="default"
      ></article>
    `;

    document.querySelector<HTMLElement>("[data-action-context]")?.dispatchEvent(new KeyboardEvent("keydown", {
      bubbles: true,
      key: "Enter",
    }));

    const params = new URLSearchParams(window.location.search);
    expect(params.get("resource")).toBe("events");
    expect(params.get("selectedName")).toBe("worker-event");
    expect(params.get("selectedNamespace")).toBe("default");
  });
});
