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

describe("quick switcher", () => {
  beforeEach(() => {
    document.body.innerHTML = "";
    window.history.replaceState({}, "", "/");
  });

  it("opens with Cmd-K and filters visible results", () => {
    document.body.innerHTML = `
      <button data-quick-open></button>
      <div data-quick-switcher hidden>
        <input data-quick-input />
        <div data-quick-count></div>
        <div data-quick-empty hidden></div>
        <button data-quick-result data-quick-search="pods workloads" aria-selected="false"></button>
        <button data-quick-result data-quick-search="services network" aria-selected="false"></button>
      </div>
    `;

    document.dispatchEvent(new KeyboardEvent("keydown", {
      bubbles: true,
      key: "k",
      metaKey: true,
    }));

    const palette = document.querySelector<HTMLElement>("[data-quick-switcher]");
    expect(palette?.hidden).toBe(false);

    const input = document.querySelector<HTMLInputElement>("[data-quick-input]");
    input!.value = "svc";
    input!.dispatchEvent(new InputEvent("input", { bubbles: true }));

    const results = Array.from(document.querySelectorAll<HTMLElement>("[data-quick-result]"));
    expect(results[0].hidden).toBe(true);
    expect(results[1].hidden).toBe(true);
    expect(document.querySelector<HTMLElement>("[data-quick-empty]")?.hidden).toBe(false);

    input!.value = "network";
    input!.dispatchEvent(new InputEvent("input", { bubbles: true }));

    expect(results[0].hidden).toBe(true);
    expect(results[1].hidden).toBe(false);
    expect(results[1].getAttribute("aria-selected")).toBe("true");
    expect(document.querySelector<HTMLElement>("[data-quick-count]")?.textContent).toBe("1 result");
  });

  it("orders stronger label and object matches before weak context matches", () => {
    document.body.innerHTML = `
      <div data-quick-switcher>
        <input data-quick-input />
        <div data-quick-count></div>
        <div data-quick-empty hidden></div>
        <div data-quick-results>
          <button
            data-quick-result
            data-quick-index="0"
            data-quick-label="Pods"
            data-quick-kind="View"
            data-quick-context="chatto-dev"
            data-quick-search="pods namespaced resource chatto-dev"
            aria-selected="false"
          ></button>
          <button
            data-quick-result
            data-quick-index="1"
            data-quick-label="chatto-api"
            data-quick-kind="Pods"
            data-quick-context="chatto-dev"
            data-quick-selected-name="chatto-api"
            data-quick-search="chatto-api pods default chatto-dev running"
            aria-selected="false"
          ></button>
          <button
            data-quick-result
            data-quick-index="2"
            data-quick-label="chatto"
            data-quick-kind="Namespace"
            data-quick-context="chatto-dev"
            data-quick-namespace="chatto"
            data-quick-search="chatto namespace"
            aria-selected="false"
          ></button>
        </div>
      </div>
    `;

    const input = document.querySelector<HTMLInputElement>("[data-quick-input]");
    input!.value = "chatto";
    input!.dispatchEvent(new InputEvent("input", { bubbles: true }));

    const results = Array.from(document.querySelectorAll<HTMLElement>("[data-quick-result]"));
    expect(results.map((result) => result.dataset.quickLabel)).toEqual(["chatto", "chatto-api", "Pods"]);
    expect(results[0].getAttribute("aria-selected")).toBe("true");
    expect(document.querySelector<HTMLElement>("[data-quick-count]")?.textContent).toBe("3 results");
  });

  it("pushes URL state and closes from selected result", () => {
    document.body.innerHTML = `
      <div data-quick-switcher>
        <input data-quick-input />
        <div data-quick-count></div>
        <div data-quick-empty hidden></div>
        <button
          data-quick-result
          data-quick-search="pods api"
          data-quick-context="prod"
          data-quick-clusters="prod"
          data-quick-resource="pods"
          data-quick-namespace="default"
          data-quick-selected-name="api"
          data-quick-selected-namespace="default"
          data-quick-detail-mode="overview"
          aria-selected="false"
        ></button>
      </div>
    `;

    document.dispatchEvent(new KeyboardEvent("keydown", {
      bubbles: true,
      key: "Enter",
    }));

    const params = new URLSearchParams(window.location.search);
    expect(params.get("context")).toBe("prod");
    expect(params.get("clusters")).toBe("prod");
    expect(params.get("resource")).toBe("pods");
    expect(params.get("namespace")).toBe("default");
    expect(params.get("selectedName")).toBe("api");
    expect(params.get("selectedNamespace")).toBe("default");
    expect(document.querySelector<HTMLElement>("[data-quick-switcher]")?.hidden).toBe(true);
  });
});
