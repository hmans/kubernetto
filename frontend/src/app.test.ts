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

describe("search shortcut", () => {
  beforeEach(() => {
    document.body.innerHTML = "";
    window.history.replaceState({}, "", "/");
  });

  it("focuses the table search input when slash is pressed", () => {
    document.body.innerHTML = `<input id="query" type="search" />`;

    const event = new KeyboardEvent("keydown", { key: "/", bubbles: true, cancelable: true });
    const dispatched = document.dispatchEvent(event);

    expect(dispatched).toBe(false);
    expect(document.activeElement).toBe(document.querySelector("#query"));
  });

  it("leaves slash alone while typing in an input", () => {
    document.body.innerHTML = `
      <input id="query" type="search" />
      <input id="other" type="text" />
    `;
    const other = document.querySelector<HTMLInputElement>("#other");
    other?.focus();

    const event = new KeyboardEvent("keydown", { key: "/", bubbles: true, cancelable: true });
    const dispatched = other?.dispatchEvent(event);

    expect(dispatched).toBe(true);
    expect(document.activeElement).toBe(other);
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

describe("map selection history", () => {
  beforeEach(() => {
    document.body.innerHTML = "";
    window.history.replaceState({}, "", "/?resource=map&mapSelected=namespace%3Aold");
  });

  it("handles mapSelected-only popstate without requiring a page reload", () => {
    document.body.innerHTML = `
      <button class="resource-child-button" data-resource-kind="map" aria-pressed="true"></button>
      <div data-cluster-map data-map-selected="namespace:old"></div>
    `;
    let selectedId = "";
    window.addEventListener("kubernetto:map-selection-popstate", ((event: CustomEvent<{ selectedId: string }>) => {
      selectedId = event.detail.selectedId;
    }) as EventListener, { once: true });

    window.history.replaceState({}, "", "/?resource=map&mapSelected=namespace%3Anew");
    window.dispatchEvent(new PopStateEvent("popstate"));

    expect(selectedId).toBe("namespace:new");
  });
});
