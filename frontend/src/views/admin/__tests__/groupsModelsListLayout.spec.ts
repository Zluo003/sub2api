import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

import { describe, expect, it } from "vitest";

const currentDir = dirname(fileURLToPath(import.meta.url));
const groupsViewSource = readFileSync(
  resolve(currentDir, "../GroupsView.vue"),
  "utf8",
);
const agentPricingSource = readFileSync(
  resolve(currentDir, "../../../components/admin/group/AgentModelPricingEditor.vue"),
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

  it("keeps newly added pricing rules disabled until an admin opts in", () => {
    expect(agentPricingSource).toContain("enabled: false");
    expect(agentPricingSource).not.toContain("enabled: true");
  });

  it("shows the required Agent image-generation permission as locked on", () => {
    expect(groupsViewSource).toContain(
      'data-testid="agent-image-generation-required"',
    );
    expect(groupsViewSource).toContain('group.kind === "agent" ? true');
    expect(groupsViewSource).toContain(
      't("admin.groups.agent.imageGenerationLocked")',
    );
  });
});
