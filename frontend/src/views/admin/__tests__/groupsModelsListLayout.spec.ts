import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

import { describe, expect, it } from "vitest";

const currentDir = dirname(fileURLToPath(import.meta.url));
const groupsViewSource = readFileSync(
  resolve(currentDir, "../GroupsView.vue"),
  "utf8",
);

describe("groups models list layout", () => {
  it("keeps the toolbar outside of the scrolling list content", () => {
    expect(groupsViewSource).toContain("overflow-hidden rounded-lg border");
    expect(groupsViewSource).toContain("max-h-64 space-y-2 overflow-y-auto p-2");
    expect(groupsViewSource).not.toContain("sticky top-0");
  });
});

describe("system Agent pricing layout", () => {
  it("uses a table-sized dialog without widening ordinary group forms", () => {
    expect(groupsViewSource).toContain(
      ":width=\"editingGroup?.kind === 'agent' ? 'extra-wide' : 'normal'\"",
    );
  });

  it("mounts the synchronized model catalogue and keeps legacy pricing outside Agent forms", () => {
    expect(groupsViewSource).toContain(
      '<AgentModelCatalogEditor',
    );
    expect(groupsViewSource).toContain(
      "v-if=\"editingGroup.kind === 'agent'\"",
    );
    expect(groupsViewSource).toContain("payload.rate_multiplier = undefined");
    expect(groupsViewSource).toContain("payload.image_price_1k = undefined");
    expect(groupsViewSource).toContain("payload.video_pricing_rules = undefined");
    expect(groupsViewSource).toContain("payload.profit_control_enabled = undefined");
    expect(groupsViewSource).toContain("payload.profit_min_margin = undefined");
    expect(groupsViewSource).toContain("payload.profit_safety_buffer = undefined");
    expect(groupsViewSource).toContain(
      "v-if=\"editingGroup.kind !== 'agent' && isProfitControlPlatform(editForm.platform)\"",
    );
    expect(groupsViewSource).not.toContain('data-testid="agent-image-generation-required"');
  });

  it("labels Agent pricing and visibility according to the model catalogue contract", () => {
    expect(groupsViewSource).toContain(
      '{{ t("admin.groups.agent.catalogPricing") }}',
    );
    expect(groupsViewSource).toContain('{{ t("admin.groups.public") }}');
    expect(groupsViewSource).toContain(
      'v-if="row.kind !== \'agent\'"\n                @click="handleRateMultipliers(row)"',
    );
    expect(groupsViewSource).not.toContain(
      't("admin.groups.agent.systemExclusive")',
    );
    expect(groupsViewSource).toContain(
      'data-testid="agent-platform-summary"',
    );
  });
});
