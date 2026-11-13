import { test, expect, type Page } from "@playwright/test";
import { readFile } from "node:fs/promises";
import path from "node:path";

const fixtures = process.env.E2E_FIXTURES!;
async function login(page: Page) {
  await page.goto("/login");
  await page
    .getByLabel("账号", { exact: true })
    .fill(process.env.E2E_USERNAME!);
  await page
    .getByLabel("密码", { exact: true })
    .fill(process.env.E2E_PASSWORD!);
  await page.getByRole("button", { name: "进入文件空间" }).click();
  await expect(page.getByRole("heading", { name: "文件空间" })).toBeVisible();
}
async function upload(page: Page, fixture: string, name: string) {
  await page
    .getByLabel("选择上传文件")
    .setInputFiles({
      name,
      mimeType: "application/octet-stream",
      buffer: await readFile(path.join(fixtures, fixture)),
    });
  const row = page.getByRole("row").filter({ hasText: name });
  await expect(row).toBeVisible();
  await expect(row.locator(".badge")).toHaveText("未分享");
  return row;
}

test("网页上传、PDF 匿名预览下载、撤销、重新分享、删除", async ({
  page,
  context,
  browser,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "登录团队空间" }),
  ).toBeVisible();
  await page
    .getByLabel("账号", { exact: true })
    .fill(process.env.E2E_USERNAME!);
  await page.getByLabel("密码", { exact: true }).fill("wrong-password");
  await page.getByRole("button", { name: "进入文件空间" }).click();
  await expect(page.getByRole("alert")).toHaveText("账号或密码错误");
  await login(page);
  const row = await upload(page, "report.pdf", "web-report.pdf");
  await row.getByRole("button", { name: "开启分享" }).click();
  await expect(row.locator(".badge")).toHaveText("分享中");
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await row.getByRole("button", { name: "复制链接" }).click();
  await expect(page.getByText("分享链接已复制", { exact: true })).toBeVisible();
  const link = await page.evaluate(() => navigator.clipboard.readText());
  expect(link).toMatch(/\/s\/[A-Za-z0-9_-]{43}$/);
  await page.screenshot({
    path: "../work/screenshots/dashboard-desktop.png",
    fullPage: true,
  });
  const receiver = await browser.newContext();
  const publicPage = await receiver.newPage();
  const pdfResponse = publicPage.waitForResponse(
    (r) => r.url().endsWith("/content") && r.status() === 200,
  );
  await publicPage.goto(link);
  await expect(
    publicPage.getByRole("heading", { name: "web-report.pdf" }),
  ).toBeVisible();
  await expect(publicPage.getByTitle("web-report.pdf PDF 预览")).toBeVisible();
  expect((await pdfResponse).headers()["content-type"]).toBe("application/pdf");
  const downloading = publicPage.waitForEvent("download");
  await publicPage.getByRole("link", { name: "下载文件" }).click();
  const download = await downloading;
  expect(download.suggestedFilename()).toBe("web-report.pdf");
  expect(await readFile((await download.path())!)).toEqual(
    await readFile(path.join(fixtures, "report.pdf")),
  );
  await publicPage.screenshot({
    path: "../work/screenshots/share-pdf.png",
    fullPage: true,
  });
  await row.getByRole("button", { name: "撤销", exact: true }).click();
  await expect(row.locator(".badge")).toHaveText("未分享");
  await publicPage.reload();
  await expect(
    publicPage.getByRole("heading", { name: "分享不存在或已失效" }),
  ).toBeVisible();
  await row.getByRole("button", { name: "开启分享" }).click();
  await expect(row.locator(".badge")).toHaveText("分享中");
  await row.getByRole("button", { name: "复制链接" }).click();
  const newLink = await page.evaluate(() => navigator.clipboard.readText());
  expect(newLink).not.toBe(link);
  await publicPage.goto(newLink);
  await expect(
    publicPage.getByRole("heading", { name: "web-report.pdf" }),
  ).toBeVisible();
  await row.getByRole("button", { name: "删除 web-report.pdf" }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(
    page.getByText("正在使用的分享链接也会立即失效。", { exact: false }),
  ).toBeVisible();
  await page.getByRole("button", { name: "取消", exact: true }).click();
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "删除 web-report.pdf" }).click();
  await page.getByRole("button", { name: "确认删除" }).click();
  await expect(row).toHaveCount(0);
  await publicPage.reload();
  await expect(
    publicPage.getByRole("heading", { name: "分享不存在或已失效" }),
  ).toBeVisible();
  expect(errors).toEqual([]);
  await receiver.close();
});

test("图片与普通附件、搜索空态、手机布局、退出登录", async ({
  page,
  context,
  browser,
}) => {
  await login(page);
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  const receiver = await browser.newContext();
  const publicPage = await receiver.newPage();
  for (const [fixture, name, isImage] of [
    ["image.png", "web-image.png", true],
    ["notes.txt", "web-notes.txt", false],
  ] as const) {
    const row = await upload(page, fixture, name);
    await row.getByRole("button", { name: "开启分享" }).click();
    await expect(row.locator(".badge")).toHaveText("分享中");
    await row.getByRole("button", { name: "复制链接" }).click();
    await expect(
      page.getByText("分享链接已复制", { exact: true }),
    ).toBeVisible();
    const link = await page.evaluate(() => navigator.clipboard.readText());
    await publicPage.goto(link);
    await expect(
      publicPage.getByRole("heading", { name, exact: true }),
    ).toBeVisible();
    if (isImage) {
      const image = publicPage.getByRole("img", { name });
      await expect(image).toBeVisible();
      await expect
        .poll(() => image.evaluate((img: HTMLImageElement) => img.naturalWidth))
        .toBeGreaterThan(0);
      await publicPage.setViewportSize({ width: 390, height: 844 });
      await publicPage.screenshot({
        path: "../work/screenshots/share-mobile.png",
        fullPage: true,
      });
    } else
      await expect(
        publicPage.getByText("此类型暂不支持在线预览，请下载后查看。"),
      ).toBeVisible();
    const downloading = publicPage.waitForEvent("download");
    await publicPage.getByRole("link", { name: "下载文件" }).click();
    expect(await readFile((await (await downloading).path())!)).toEqual(
      await readFile(path.join(fixtures, fixture)),
    );
  }
  await page.getByLabel("搜索文件").fill("web-image");
  await expect(
    page.getByRole("row").filter({ hasText: "web-image.png" }),
  ).toBeVisible();
  await expect(
    page.getByRole("row").filter({ hasText: "web-notes.txt" }),
  ).toHaveCount(0);
  await page.getByLabel("搜索文件").fill("definitely-no-such-file");
  await expect(
    page.getByRole("heading", { name: "没有找到匹配的文件" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "清空搜索" }).click();
  await expect(
    page.getByRole("row").filter({ hasText: "web-notes.txt" }),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: "../work/screenshots/dashboard-mobile.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "退出登录" }).click();
  await expect(
    page.getByRole("heading", { name: "登录团队空间" }),
  ).toBeVisible();
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "登录团队空间" }),
  ).toBeVisible();
  await receiver.close();
});
