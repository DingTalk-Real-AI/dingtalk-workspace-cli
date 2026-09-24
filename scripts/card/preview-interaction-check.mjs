#!/usr/bin/env node

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const previewPath = process.argv[2];
if (!previewPath) {
  throw new Error("usage: preview-interaction-check.mjs <preview.html>");
}

const html = readFileSync(previewPath, "utf8");
const scripts = [...html.matchAll(/<script>([\s\S]*?)<\/script>/g)];
assert.ok(scripts.length > 0, "preview must contain an interaction script");

const result = { hidden: true };
const output = { textContent: "" };
let dispatched;

global.document = {
  querySelectorAll() { return []; },
  getElementById(id) {
    if (id === "interaction-json") return output;
    if (id === "interaction-result") return result;
    throw new Error(`unexpected element id: ${id}`);
  },
  dispatchEvent(event) { dispatched = event; },
};
global.CustomEvent = class CustomEvent {
  constructor(type, init) {
    this.type = type;
    this.detail = init.detail;
  }
};

const releaseControls = [
  { type: "text", value: "1.0.0", dataset: { bindingPath: "/form/version" } },
  { type: "textarea", value: "Release notes", dataset: { bindingPath: "/form/notes" } },
  { type: "checkbox", checked: true, value: "web", dataset: { bindingPath: "/form/platforms" } },
  { type: "checkbox", checked: true, value: "ios", dataset: { bindingPath: "/form/platforms" } },
  { tagName: "SELECT", value: "staging", dataset: { bindingPath: "/form/environment" } },
  { type: "radio", checked: false, value: "standard", dataset: { bindingPath: "/form/strategy" } },
  { type: "radio", checked: true, value: "gradual", dataset: { bindingPath: "/form/strategy" } },
];
const releaseButton = {
  dataset: { eventName: "form_submit", surfaceId: "dws-local-form" },
  addEventListener(type, listener) {
    assert.equal(type, "click");
    this.click = listener;
  },
};
global.document.querySelectorAll = (selector) => {
  if (selector === ".Image" || selector === "[data-preview-event]") return [];
  if (selector === "[data-binding-path]") return releaseControls;
  if (selector === "button[data-event-name]") return [releaseButton];
  throw new Error(`unexpected selector: ${selector}`);
};
new Function(scripts.at(-1)[1])();
releaseButton.click();
assert.deepEqual(JSON.parse(output.textContent).event.context.form, {
  version: "1.0.0",
  notes: "Release notes",
  platforms: ["web", "ios"],
  environment: ["staging"],
  strategy: ["gradual"],
});

const payload = JSON.parse(output.textContent);
assert.equal(payload.event.name, "form_submit");
assert.equal(payload.event.context.surfaceId, "dws-local-form");
assert.equal(result.hidden, false);
assert.equal(dispatched.type, "dws-a2ui-preview-action");
assert.deepEqual(dispatched.detail, payload);
process.stdout.write(`${JSON.stringify({ status: "PASS", payload })}\n`);
