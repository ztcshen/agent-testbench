import assert from "node:assert/strict";

export async function verifyStepLibrary({
  page,
  call,
  screenshot,
  check,
  summary,
}) {
  await page.getByRole("button", { name: "保存为步骤组", exact: true }).click();
  await page
    .getByLabel("新步骤组名称", { exact: true })
    .fill("Reusable item checks");
  await page.getByLabel("步骤组开始步骤", { exact: true }).selectOption("1");
  await page.getByLabel("步骤组结束步骤", { exact: true }).selectOption("2");
  await page.getByRole("button", { name: "保存到步骤库", exact: true }).click();
  await page.getByRole("dialog").waitFor({ state: "hidden" });
  const library = (await call("/api/step-library")).data.blocks;
  assert.equal(library.length, 1);
  assert.equal(library[0].definition.parameters[0].name, "itemId");
  const originalRevision = library[0].definition.revision;
  await page.getByRole("button", { name: "复用步骤库", exact: true }).click();
  await page.locator(".library-card").waitFor();
  await screenshot("step-library");
  await page
    .getByRole("button", { name: "编辑定义与引用", exact: true })
    .click();
  await page.getByLabel("参数 itemId 默认值", { exact: true }).check();
  await page
    .getByLabel("参数 itemId 默认内容", { exact: true })
    .fill("unused-default");
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await page.locator(".library-card").waitFor();
  const version = (await call("/api/step-library")).data.blocks[0].definition
    .revision;
  assert.notEqual(version, originalRevision);
  await page.getByRole("button", { name: "自动化测试", exact: true }).click();
  await page.getByRole("button", { name: "添加步骤", exact: true }).click();
  await page
    .locator(".add-block-item")
    .filter({ hasText: "Reusable item checks" })
    .click();
  await page.locator(".group-sequence-card").click();
  await page
    .getByLabel("步骤组参数 itemId 来源", { exact: true })
    .selectOption("previous");
  await page
    .getByLabel("步骤组参数 itemId 前序步骤", { exact: true })
    .selectOption("create");
  await page
    .getByLabel("步骤组参数 itemId 字段路径", { exact: true })
    .fill("$.id");
  await screenshot("step-group-parameters");
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await page.locator(".unsaved-label").waitFor({ state: "hidden" });
  await page.getByRole("button", { name: "运行场景", exact: true }).click();
  await page
    .locator(".scenario-inline-result .status.passed")
    .waitFor({ timeout: 30000 });
  const latest = (await call("/api/catalog")).data.workflows[0].latestRun;
  summary.stepGroups = (
    await call(`/api/cases/batch-runs/${encodeURIComponent(latest.id)}`)
  ).data;
  assert.equal(summary.stepGroups.status, "passed");
  assert.equal(summary.stepGroups.total, 5);
  const references = (await call("/api/step-library")).data.blocks[0].usages;
  assert.equal(references.length, 1);
  await page.getByRole("tab", { name: "运行结果", exact: true }).click();
  const groupResult = page
    .locator(".scenario-run-step")
    .filter({ hasText: "Reusable item checks" })
    .first();
  await groupResult.locator(":scope > summary").click();
  await groupResult.locator(".scenario-run-snapshot").waitFor();
  assert(
    (await groupResult.locator(".scenario-run-snapshot").innerText()).includes(
      "item-42",
    ),
  );
  assert(
    (await groupResult.locator(".scenario-run-snapshot").innerText()).includes(
      version.slice(0, 8),
    ),
  );
  await screenshot("step-group-results");
  check(
    "Reusable block definitions, parameter bindings, versioned references and five actual HTTP steps pass",
  );
}
